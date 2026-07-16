# Apps auto sign-in via Keycloak SSO cookies

**Date:** 2026-07-17  
**Status:** Approved  
**Approach:** Lab UI on `localhost` + server-side Keycloak login + `Set-Cookie` + redirect

## Goal

Clicking an app in Lab UI opens it already signed in (no Keycloak form), by presenting real Keycloak SSO cookies to the browser.

## Mechanism

1. Default Lab UI bind: **`http://localhost:9090`** (same cookie host as Camunda; ports differ).
2. `GET /api/v1/sso/open?url=<app-url>` (loopback only):
   - Lab backend performs Keycloak username/password login (`demo`/`demo`) with a cookie jar.
   - Response copies Keycloak session cookies onto the browser (`Host=localhost`, HttpOnly as appropriate).
   - `302` redirect to the app URL.
3. Browser visits Operate → Keycloak sees existing SSO session → no login form.

## Non-goals

- Setting cookies from `127.0.0.1` (wrong host).
- Forging cookies without Keycloak.
- Production IdP support.

## UX

- Apps cards link to `/api/v1/sso/open?url=…` for HTTP app URLs that use Keycloak (credential-backed / Camunda UIs). Non-auth links (Elasticsearch, raw APIs) stay direct.
- Keep Sign out / Fix broken session (Keycloak logout).
- Banner: auto sign-in needs Lab UI on `localhost`.

## Credentials

Default `demo`/`demo`. Keycloak admin console may use `admin`/`admin` — app SSO uses realm user `demo`.
