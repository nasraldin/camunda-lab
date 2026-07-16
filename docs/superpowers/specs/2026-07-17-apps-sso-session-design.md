# Apps SSO session tools design

**Date:** 2026-07-17  
**Status:** Approved (approach 1)  
**Scope:** Lab UI Apps (+ related Logins copy). No cookie injection into Camunda/Keycloak.

## Findings (lab 8.9 full)

- Camunda apps use Keycloak OIDC (PKCE). Login is on Keycloak, not on each app form.
- After one successful login, other apps SSO without re-prompt (Keycloak session on `localhost:18080`).
- Session cookies are mostly HttpOnly. Lab UI (`127.0.0.1:9090`) cannot read/set them for `localhost` app origins.
- App URLs already use `localhost` (required for consistent cookies). Mixing `127.0.0.1` causes login loops / odd 404s.

## Goals

1. Set expectation: log in once → other Camunda apps stay signed in.
2. Let the user **sign out** of Camunda SSO from the Lab UI.
3. Offer **fix broken session** guidance (logout + reopen via Lab links only).
4. Optional: copy `demo` / `demo` (or Keycloak `admin`) before first open.

## Non-goals

- Presetting or forging Camunda/Keycloak cookies from Lab UI.
- Automating the Keycloak login form (extension/CDP).
- Changing Camunda compose auth configuration.

## UX

### Apps page toolbar

- Tip (banner or hint): “Sign in once in the browser. Other Camunda apps usually stay open.”
- Actions:
  - **Sign out of Camunda** — opens Keycloak logout URL in a new tab  
    `http://localhost:<keycloak-port>/auth/realms/camunda-platform/protocol/openid-connect/logout`  
    (derive host/port from the `keycloak` entry in `/api/v1/urls`, fallback `http://localhost:18080/auth/`).
  - **Fix broken session** — same logout URL + toast/banner: use only Lab “Apps” links (`localhost`); avoid mixing `127.0.0.1`.
  - Keep **Show all addresses**.

### Opening an app

- Unchanged: whole card opens the app URL in a new tab.
- Optional soft path — small secondary control “Copy login” (username then password, or combined `user:pass`) without blocking open.
- Do not force a modal on every click (annoying after SSO is warm).

Cards stay clean: open-only; credentials live on the Logins page.

### Logins page

- Short cross-link: “Apps stay signed in after one Keycloak login. Sign out from Apps.”

## Implementation notes

- Frontend-only for logout (open URL). No backend cookie API.
- Parse Keycloak base from `urls` entry named `keycloak`; append `/realms/camunda-platform/protocol/openid-connect/logout` if path is `/auth/` or `/auth`.
- Light profile without Keycloak: hide Sign out / Fix session, or show “No Keycloak in this profile.”

## Success

- User understands SSO.
- Can clear SSO from Lab UI and get a fresh login page on next app open.
- No attempt to inject cookies; docs/spec state why.
