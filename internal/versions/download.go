package versions

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

type DownloadOptions struct {
	URL           string
	HTTPClient    *http.Client
	SkipIfPresent bool
}

func Ensure(minor string, opts DownloadOptions) (string, error) {
	if err := ValidateMinor(minor); err != nil {
		return "", err
	}
	dir := paths.VersionDir(minor)
	marker := filepath.Join(dir, "docker-compose.yaml")
	coreMarker := filepath.Join(dir, "docker-compose-core.yaml")
	if opts.SkipIfPresent {
		if _, err := os.Stat(marker); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(coreMarker); err == nil {
			return dir, nil
		}
	}

	url := opts.URL
	if url == "" {
		url = ZipURL(minor)
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	if err := os.MkdirAll(paths.VersionsDir(), 0o755); err != nil {
		return "", err
	}

	tmpZip, err := os.CreateTemp("", "camunda-compose-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmpZip.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	resp, err := client.Get(url)
	if err != nil {
		_ = tmpZip.Close()
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_ = tmpZip.Close()
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	if _, err := io.Copy(tmpZip, resp.Body); err != nil {
		_ = tmpZip.Close()
		return "", err
	}
	if err := tmpZip.Close(); err != nil {
		return "", err
	}

	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := unzipTo(tmpPath, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func unzipTo(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	stripPrefix := commonRootPrefix(r.File)

	for _, f := range r.File {
		name := f.Name
		if stripPrefix != "" {
			name = strings.TrimPrefix(name, stripPrefix)
		}
		if name == "" || name == "/" {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), filepath.Clean(dest)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("zip entry escapes dest: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func commonRootPrefix(files []*zip.File) string {
	if len(files) == 0 {
		return ""
	}
	first := files[0].Name
	slash := strings.Index(first, "/")
	if slash <= 0 {
		return ""
	}
	prefix := first[:slash+1]
	for _, f := range files {
		if !strings.HasPrefix(f.Name, prefix) {
			return ""
		}
	}
	return prefix
}
