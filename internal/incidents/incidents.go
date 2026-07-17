package incidents

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

// Incident is a cluster incident summary.
type Incident struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	Error     string    `json:"error"`
	Process   string    `json:"process"`
	JobWorker string    `json:"jobWorker"`
	Key       string    `json:"key"`
}

type listResponse struct {
	Items []Incident `json:"items"`
}

// Client talks to a simple incidents API (mockable).
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// List fetches incidents from GET {base}/incidents.
func (c Client) List() ([]Incident, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(strings.TrimRight(c.BaseURL, "/") + "/incidents")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("incidents API HTTP %d", resp.StatusCode)
	}
	var body listResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Items, nil
}

// Retry posts to POST {base}/incidents/{id}/retry.
func (c Client) Retry(id string) error {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Post(strings.TrimRight(c.BaseURL, "/")+"/incidents/"+id+"/retry", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("retry HTTP %d", resp.StatusCode)
	}
	return nil
}

// FormatTable renders a simple table.
func FormatTable(items []Incident) string {
	if len(items) == 0 {
		return "No incidents.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-20s %-20s %-16s %s\n", "ID", "CREATED", "WORKER", "ERROR")
	for _, it := range items {
		fmt.Fprintf(&b, "%-20s %-20s %-16s %s\n",
			trim(it.ID, 20), it.Created.Format(time.RFC3339), trim(it.JobWorker, 16), trim(it.Error, 40))
	}
	return b.String()
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// NewTestServer returns a mock incidents API for tests.
func NewTestServer(items []Incident) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/incidents", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(listResponse{Items: items})
	})
	mux.HandleFunc("/incidents/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/retry") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	return httptest.NewServer(mux)
}
