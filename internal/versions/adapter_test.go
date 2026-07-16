package versions_test

import (
	"testing"

	"github.com/nasraldin/camunda-lab/internal/versions"
)

func TestComposeFiles88Light(t *testing.T) {
	files, err := versions.ComposeFiles("8.8", "light")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "docker-compose.yaml" {
		t.Fatalf("%v", files)
	}
}

func TestComposeFiles88Full(t *testing.T) {
	files, err := versions.ComposeFiles("8.8", "full")
	if err != nil {
		t.Fatal(err)
	}
	if files[0] != "docker-compose-full.yaml" {
		t.Fatalf("%v", files)
	}
}

func TestComposeFiles87(t *testing.T) {
	light, err := versions.ComposeFiles("8.7", "light")
	if err != nil {
		t.Fatal(err)
	}
	full, err := versions.ComposeFiles("8.7", "full")
	if err != nil {
		t.Fatal(err)
	}
	if light[0] != "docker-compose-core.yaml" {
		t.Fatalf("light=%v", light)
	}
	if full[0] != "docker-compose.yaml" {
		t.Fatalf("full=%v", full)
	}
}

func Test810NeedsES(t *testing.T) {
	if !versions.NeedsElasticsearchOverlay("8.10", "full") {
		t.Fatal("expected ES overlay")
	}
	if versions.NeedsElasticsearchOverlay("8.10", "light") {
		t.Fatal("light should not force ES overlay")
	}
	if versions.NeedsElasticsearchOverlay("8.9", "full") {
		t.Fatal("8.9 bundles ES")
	}
}

func TestPreview(t *testing.T) {
	if !versions.IsPreview("8.10") {
		t.Fatal("8.10 should be preview")
	}
	if versions.IsPreview("8.8") {
		t.Fatal("8.8 should not be preview")
	}
}

func TestHasHostElasticsearch(t *testing.T) {
	cases := []struct {
		minor, profile string
		want           bool
	}{
		{"8.7", "light", true},
		{"8.7", "full", true},
		{"8.7", "modeler", false},
		{"8.8", "light", true},
		{"8.8", "full", true},
		{"8.9", "light", false},
		{"8.9", "full", true},
		{"8.10", "light", false},
		{"8.10", "full", true},
		{"8.10", "modeler", false},
	}
	for _, tc := range cases {
		got := versions.HasHostElasticsearch(tc.minor, tc.profile)
		if got != tc.want {
			t.Fatalf("%s/%s: got %v want %v", tc.minor, tc.profile, got, tc.want)
		}
	}
}
