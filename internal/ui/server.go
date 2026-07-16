package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/ui/api"
)

//go:embed all:web/dist
var distRoot embed.FS

// Options configures the Lab UI server.
type Options struct {
	Host    string
	Port    int
	Open    bool
	Version string
}

func DefaultOptions() Options {
	port := 9090
	if v := os.Getenv("CAMUNDA_LAB_UI_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			port = n
		}
	}
	return Options{
		Host: "127.0.0.1",
		Port: port,
		Open: true,
	}
}

// Run starts the Lab UI HTTP server (blocking).
func Run(opts Options) error {
	if opts.Host == "" {
		opts.Host = "127.0.0.1"
	}
	if opts.Port == 0 {
		opts.Port = 9090
	}
	if err := assertLoopback(opts.Host); err != nil {
		return err
	}

	static, err := fs.Sub(distRoot, "web/dist")
	if err != nil {
		return fmt.Errorf("ui assets: %w", err)
	}

	mux := http.NewServeMux()
	api.Register(mux, opts.Version)
	mux.Handle("/", spaHandler(http.FS(static)))

	addr := net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port))
	url := fmt.Sprintf("http://%s/", addr)
	fmt.Fprintf(os.Stderr, "Camunda Lab UI listening on %s\n", url)
	fmt.Fprintf(os.Stderr, "No auth — loopback only. Ctrl+C to stop.\n")

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	if opts.Open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			_ = openBrowser(url)
		}()
	}

	return srv.ListenAndServe()
}

func assertLoopback(host string) error {
	h := strings.TrimSpace(strings.ToLower(host))
	switch h {
	case "127.0.0.1", "localhost", "::1":
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("lab UI refuses non-loopback host %q (no auth); use 127.0.0.1", host)
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("open browser unsupported on %s", runtime.GOOS)
	}
	return cmd.Start()
}

func spaHandler(static http.FileSystem) http.Handler {
	fileServer := http.FileServer(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := r.URL.Path
		if path != "/" {
			if f, err := static.Open(strings.TrimPrefix(path, "/")); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			r2 := *r
			u := *r.URL
			u.Path = "/"
			r2.URL = &u
			fileServer.ServeHTTP(w, &r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
