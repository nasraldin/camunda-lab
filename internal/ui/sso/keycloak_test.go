package sso

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFormAction(t *testing.T) {
	html := `<form id="kc-form-login" action="http://localhost:18080/auth/realms/camunda-platform/login-actions/authenticate?session_code=abc&amp;client_id=orchestration" method="post">`
	got := formAction(html)
	if !strings.Contains(got, "login-actions/authenticate") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "&amp;") {
		t.Fatalf("amp not expected before unescape in matcher; got %q", got)
	}
}

func TestSessionCookiesLive(t *testing.T) {
	if testing.Short() {
		t.Skip("live")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	base := "http://localhost:18080/auth/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("Keycloak not reachable: %v", err)
	}
	_ = res.Body.Close()

	cookies, err := SessionCookies(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range cookies {
		t.Logf("%s path=%s secure=%v valueLen=%d", c.Name, c.Path, c.Secure, len(c.Value))
		names[c.Name] = true
	}
	if !names["KEYCLOAK_SESSION"] || !names["KEYCLOAK_IDENTITY"] {
		t.Fatalf("missing cookies: %#v", names)
	}
}
