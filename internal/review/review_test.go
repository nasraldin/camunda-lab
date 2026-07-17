package review

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

func TestOfflineReview(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "lint", "broken.bpmn")
	m, err := bpmn.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(m, Options{File: "broken.bpmn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected lint findings")
	}
	if res.AIText != "" {
		t.Fatal("offline should not include AI")
	}
	if !lint.ShouldFail(res.Findings, "error") {
		t.Fatal("should fail")
	}
}

func TestAIReviewMocked(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "order-v1.bpmn")
	m, err := bpmn.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(m, Options{
		AI:       true,
		AIClient: StubClient{},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := FormatText(res)
	if !strings.Contains(text, "Review (lint)") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "AI suggestions") {
		t.Fatal(text)
	}
	if !strings.Contains(res.AIText, "orderProcess") {
		t.Fatal(res.AIText)
	}
}
