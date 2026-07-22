package testgen

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPackZIPHasSignatureAndRestrictiveModes(t *testing.T) {
	artifacts := []Artifact{
		{Path: "js/b.spec.js", MediaType: "text/javascript", Content: []byte("b")},
		{Path: "js/a.spec.js", MediaType: "text/javascript", Content: []byte("a")},
	}
	payload, err := PackZIP(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) < 4 || !bytes.Equal(payload[:4], []byte("PK\x03\x04")) {
		t.Fatalf("missing ZIP local-file signature: %x", payload[:min(4, len(payload))])
	}

	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 2 {
		t.Fatalf("entries = %d, want 2", len(reader.File))
	}
	if reader.File[0].Name != "js/a.spec.js" || reader.File[1].Name != "js/b.spec.js" {
		t.Fatalf("entry order = %q, %q", reader.File[0].Name, reader.File[1].Name)
	}
	for _, file := range reader.File {
		if got := file.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 0600", file.Name, got)
		}
		if !file.Modified.Equal(time.Unix(0, 0).UTC()) {
			t.Fatalf("%s Modified = %v, want unix epoch UTC", file.Name, file.Modified)
		}
		if filepath.IsAbs(file.Name) || file.Name != filepath.ToSlash(file.Name) {
			t.Fatalf("unsafe entry name %q", file.Name)
		}
	}
}

func TestPackZIPIsDeterministic(t *testing.T) {
	artifacts := []Artifact{
		{Path: "java/TwoTest.java", MediaType: "text/x-java-source", Content: []byte("two")},
		{Path: "java/OneTest.java", MediaType: "text/x-java-source", Content: []byte("one")},
	}
	first, err := PackZIP(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := PackZIP(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("PackZIP is not byte-deterministic")
	}
}

func TestPackZIPRejectsTraversalAndAbsolutePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "parent", path: "../escape.js"},
		{name: "nested parent", path: "js/../../escape.js"},
		{name: "absolute", path: "/tmp/escape.js"},
		{name: "windows abs", path: `C:\tmp\escape.js`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := PackZIP([]Artifact{{Path: test.path, Content: []byte("x")}})
			if err == nil {
				t.Fatal("PackZIP accepted unsafe path")
			}
		})
	}
}

func TestPackZIPDoesNotWriteServerFiles(t *testing.T) {
	root := t.TempDir()
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := PackZIP([]Artifact{{Path: "js/one.spec.js", Content: []byte("ok")}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("PackZIP wrote files under %s", root)
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := reader.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
}
