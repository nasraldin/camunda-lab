package versions_test

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

func TestEnsureExtractsZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"docker-compose.yaml", ".env"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte("content-" + name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()

	dir, err := versions.Ensure("8.8", versions.DownloadOptions{
		URL:        srv.URL + "/docker-compose-8.8.zip",
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "versions", "8.8", "docker-compose.yaml")
	if dir != filepath.Join(home, "versions", "8.8") {
		t.Fatalf("dir=%q", dir)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content-docker-compose.yaml" {
		t.Fatalf("content=%q", data)
	}
}

func TestEnsureSkipIfPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	dir := filepath.Join(home, "versions", "8.8")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "docker-compose.yaml")
	if err := os.WriteFile(marker, []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := versions.Ensure("8.8", versions.DownloadOptions{
		URL:           "http://127.0.0.1:1/should-not-hit",
		SkipIfPresent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("got=%q", got)
	}
}
