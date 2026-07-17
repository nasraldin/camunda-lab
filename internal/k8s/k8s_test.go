package k8s

import (
	"strings"
	"testing"
)

func TestStatusArgs(t *testing.T) {
	var got []string
	r := func(args ...string) (string, error) {
		got = append([]string{}, args...)
		return "ok", nil
	}
	out, err := Status(Options{Namespace: "ns", Release: "camunda", Runner: r})
	if err != nil || out != "ok" {
		t.Fatal(err, out)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "-n ns") || !strings.Contains(joined, "get pods,svc") {
		t.Fatal(got)
	}
}

func TestRestartAndScale(t *testing.T) {
	var last []string
	r := func(args ...string) (string, error) {
		last = append([]string{}, args...)
		return "", nil
	}
	if _, err := Restart(Options{Runner: r}, "connectors"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(last, " "), "rollout restart deploy/camunda-connectors") {
		t.Fatal(last)
	}
	if _, err := Scale(Options{Runner: r}, "workers", 3); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(last, " "), "--replicas=3") {
		t.Fatal(last)
	}
}

func TestUnknownComponent(t *testing.T) {
	_, err := Logs(Options{Runner: func(args ...string) (string, error) { return "", nil }}, "nope", false, 100)
	if err == nil {
		t.Fatal("expected error")
	}
}
