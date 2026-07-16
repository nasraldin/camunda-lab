// Package update checks GitHub Releases and upgrades the camunda CLI by install channel.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

const (
	RepoOwner  = "nasraldin"
	RepoName   = "camunda-lab"
	RepoURL    = "https://github.com/nasraldin/camunda-lab"
	DocsURL    = "https://nasraldin.github.io/camunda-lab/"
	AuthorURL  = "https://nasraldin.com"
	AuthorName = "Nasr Aldin"
	InstallSH  = "https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh"
)

// Channel is how this binary was likely installed.
type Channel string

const (
	ChannelHomebrew Channel = "homebrew"
	ChannelRelease  Channel = "release"
	ChannelDev      Channel = "dev"
)

// Info is the result of a version check.
type Info struct {
	Current         string  `json:"current"`
	Latest          string  `json:"latest"`
	UpdateAvailable bool    `json:"updateAvailable"`
	Channel         Channel `json:"channel"`
	Executable      string  `json:"executable"`
	ReleaseURL      string  `json:"releaseURL"`
	PublishedAt     string  `json:"publishedAt,omitempty"`
	Error           string  `json:"error,omitempty"`
}

// Result is the outcome of Apply.
type Result struct {
	OK          bool    `json:"ok"`
	Channel     Channel `json:"channel"`
	Output      string  `json:"output"`
	RestartHint string  `json:"restartHint"`
}

type releaseCache struct {
	mu          sync.Mutex
	tag         string
	url         string
	publishedAt string
	fetchedAt   time.Time
	err         string
}

var cache releaseCache

const cacheTTL = 5 * time.Minute

// DetectChannel classifies the running binary.
func DetectChannel(version, exe string) Channel {
	v := strings.ToLower(strings.TrimSpace(version))
	if v == "" || v == "0.0.0" || strings.Contains(v, "dev") {
		return ChannelDev
	}
	p := filepath.ToSlash(exe)
	if strings.Contains(p, "/Cellar/camunda-lab/") ||
		strings.Contains(p, "/opt/homebrew/opt/camunda-lab/") ||
		strings.Contains(p, "/usr/local/opt/camunda-lab/") {
		return ChannelHomebrew
	}
	return ChannelRelease
}

// ExecutablePath returns the resolved path of the current process binary.
func ExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func normalizeTag(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// Newer reports whether latest is a higher semver than current.
func Newer(current, latest string) bool {
	c := normalizeTag(current)
	l := normalizeTag(latest)
	if !semver.IsValid(c) || !semver.IsValid(l) {
		return c != "" && l != "" && c != l && l > c
	}
	return semver.Compare(l, c) > 0
}

type ghRelease struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

func fetchLatest(ctx context.Context) (tag, url, published, errMsg string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if time.Since(cache.fetchedAt) < cacheTTL && (cache.tag != "" || cache.err != "") {
		return cache.tag, cache.url, cache.publishedAt, cache.err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", RepoOwner, RepoName), nil)
	if err != nil {
		cache.err = err.Error()
		cache.fetchedAt = time.Now()
		return "", "", "", cache.err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "camunda-lab")

	client := &http.Client{Timeout: 12 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		cache.err = err.Error()
		cache.fetchedAt = time.Now()
		return "", "", "", cache.err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		cache.err = fmt.Sprintf("GitHub API %s", res.Status)
		cache.fetchedAt = time.Now()
		cache.tag = ""
		return "", "", "", cache.err
	}
	var rel ghRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		cache.err = err.Error()
		cache.fetchedAt = time.Now()
		return "", "", "", cache.err
	}
	cache.tag = rel.TagName
	cache.url = rel.HTMLURL
	cache.publishedAt = rel.PublishedAt
	cache.err = ""
	cache.fetchedAt = time.Now()
	return cache.tag, cache.url, cache.publishedAt, ""
}

// Check returns installed vs latest release info.
func Check(ctx context.Context, currentVersion string) Info {
	exe := ExecutablePath()
	ch := DetectChannel(currentVersion, exe)
	info := Info{
		Current:    currentVersion,
		Channel:    ch,
		Executable: exe,
		ReleaseURL: RepoURL + "/releases",
	}
	tag, url, published, errMsg := fetchLatest(ctx)
	if errMsg != "" {
		info.Error = errMsg
	}
	if tag != "" {
		info.Latest = tag
		info.PublishedAt = published
		if url != "" {
			info.ReleaseURL = url
		}
		info.UpdateAvailable = Newer(currentVersion, tag) && ch != ChannelDev
	}
	return info
}

// Apply upgrades the CLI using the detected channel.
func Apply(ctx context.Context, currentVersion string) (Result, error) {
	exe := ExecutablePath()
	ch := DetectChannel(currentVersion, exe)
	hint := "Close this Camunda Lab window, then open it again so the new version loads."

	switch ch {
	case ChannelDev:
		return Result{}, fmt.Errorf("dev builds cannot self-update — install a release (curl install.sh | bash) or brew install nasraldin/tools/camunda-lab")
	case ChannelHomebrew:
		out, err := runCmd(ctx, "brew", "upgrade", "camunda-lab")
		if err != nil {
			// retry after brew update
			out2, err2 := runCmd(ctx, "bash", "-c", "brew update && brew upgrade camunda-lab")
			out = out + "\n" + out2
			if err2 != nil {
				return Result{OK: false, Channel: ch, Output: strings.TrimSpace(out), RestartHint: hint}, err2
			}
		}
		return Result{OK: true, Channel: ch, Output: strings.TrimSpace(out), RestartHint: hint}, nil
	case ChannelRelease:
		script := fmt.Sprintf("curl -fsSL %s | bash", InstallSH)
		out, err := runCmd(ctx, "bash", "-c", script)
		if err != nil {
			return Result{OK: false, Channel: ch, Output: strings.TrimSpace(out), RestartHint: hint}, err
		}
		return Result{OK: true, Channel: ch, Output: strings.TrimSpace(out), RestartHint: hint}, nil
	default:
		return Result{}, fmt.Errorf("unknown install channel %q", ch)
	}
}

func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}
