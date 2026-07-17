package incidents

import (
	"strings"
	"testing"
	"time"
)

func TestListAndRetry(t *testing.T) {
	srv := NewTestServer([]Incident{{
		ID:        "2251799813685249",
		Created:   time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC),
		Error:     "job timeout",
		JobWorker: "payment",
		Process:   "orderProcess",
	}})
	defer srv.Close()
	c := Client{BaseURL: srv.URL}
	items, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatal(items)
	}
	text := FormatTable(items)
	if !strings.Contains(text, "payment") {
		t.Fatal(text)
	}
	if err := c.Retry(items[0].ID); err != nil {
		t.Fatal(err)
	}
}
