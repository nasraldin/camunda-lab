package urls

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ProbeResult is the outcome of checking one component entry.
type ProbeResult struct {
	Name       string `json:"name"`
	OK         bool   `json:"ok"`
	Kind       string `json:"kind"` // http | tcp
	CheckedURL string `json:"checkedURL"`
	Detail     string `json:"detail"`
}

// ProbeEntry verifies that a component is reachable using the official check path.
func ProbeEntry(ctx context.Context, e Entry, timeout time.Duration) ProbeResult {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	kind, target := ProbeTarget(e)
	out := ProbeResult{Name: e.Name, Kind: kind, CheckedURL: target}
	if kind == "tcp" {
		d := net.Dialer{Timeout: timeout}
		c, err := d.DialContext(ctx, "tcp", target)
		if err != nil {
			out.Detail = err.Error()
			return out
		}
		_ = c.Close()
		out.OK = true
		out.Detail = "tcp open — gRPC gateway is listening"
		return out
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		out.Detail = err.Error()
		return out
	}
	resp, err := client.Do(req)
	if err != nil {
		out.Detail = err.Error()
		return out
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 500 {
		out.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return out
	}
	out.OK = true
	out.Detail = fmt.Sprintf("HTTP %d — up and running", resp.StatusCode)
	return out
}
