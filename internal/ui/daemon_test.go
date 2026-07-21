package ui

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestBaseURL(t *testing.T) {
	got := BaseURL(Options{Host: "localhost", Port: 9090})
	if got != "http://localhost:9090/" {
		t.Fatalf("got %q", got)
	}
}

func TestIsRunning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/overview" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Host: host, Port: port}
	if !IsRunning(opts) {
		t.Fatal("expected running")
	}
	opts.Port = 1
	if IsRunning(opts) {
		t.Fatal("expected not running on bad port")
	}
}

func TestPIDFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	paths.Reset()
	t.Setenv("CAMUNDA_LAB_HOME", dir)
	if err := writePIDFile(4242, 9090); err != nil {
		t.Fatal(err)
	}
	pid, port, err := readPIDFile()
	if err != nil {
		t.Fatal(err)
	}
	if pid != 4242 || port != 9090 {
		t.Fatalf("got pid=%d port=%d", pid, port)
	}
	if _, err := os.Stat(filepath.Join(dir, "ui.pid")); err != nil {
		t.Fatal(err)
	}
}
