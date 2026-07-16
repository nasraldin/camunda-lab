package smoke

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/urls"
)

func Run(ctx context.Context, cfg config.Config) error {
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	entries := urls.List(cfg)
	var last error
	checked := 0
	for _, e := range entries {
		if !stringsHasHTTP(e.URL) {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			last = fmt.Errorf("%s: %w", e.Name, err)
			continue
		}
		_ = resp.Body.Close()
		// 2xx, 3xx, 401, 403 count as "up"
		if resp.StatusCode < 500 {
			checked++
			continue
		}
		last = fmt.Errorf("%s: HTTP %d", e.Name, resp.StatusCode)
	}
	if checked == 0 {
		if last != nil {
			return last
		}
		return fmt.Errorf("no HTTP endpoints responded successfully")
	}
	return nil
}

func stringsHasHTTP(u string) bool {
	return len(u) >= 4 && (u[:4] == "http")
}

func Wait(ctx context.Context, cfg config.Config, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		last = Run(ctx, cfg)
		if last == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	if last == nil {
		last = fmt.Errorf("timeout waiting for healthy lab")
	}
	return fmt.Errorf("wait timed out after %s: %w", timeout, last)
}
