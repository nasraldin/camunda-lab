package urls_test

import (
	"testing"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/urls"
)

func TestLight88Operate(t *testing.T) {
	entries := urls.List(config.Config{Version: "8.8", Profile: "light", Host: "localhost"})
	if entries[0].Name != "operate" || entries[0].URL != "http://localhost:8080/operate" {
		t.Fatalf("%+v", entries[0])
	}
}

func TestFull88Console(t *testing.T) {
	entries := urls.List(config.Config{Version: "8.8", Profile: "full", Host: "localhost"})
	found := false
	for _, e := range entries {
		if e.Name == "console" && e.URL == "http://localhost:8087" {
			found = true
		}
	}
	if !found {
		t.Fatalf("%+v", entries)
	}
}

func TestLight87Operate(t *testing.T) {
	entries := urls.List(config.Config{Version: "8.7", Profile: "light", Host: "localhost"})
	if entries[0].URL != "http://localhost:8081" {
		t.Fatalf("%+v", entries[0])
	}
}
