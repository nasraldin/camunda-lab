# Apps auto sign-in opt-out

**Date:** 2026-07-17  
**Status:** Approved  
**Depends on:** [apps-sso-autologin](./2026-07-17-apps-sso-autologin-design.md)

## Goal

Give users a clear way to turn off Lab’s automatic Keycloak warm-up when opening Camunda apps, without removing the default convenience.

## Behavior

- Apps page header includes a labeled **Auto sign-in** switch.
- **On (default):** SSO apps use `/api/v1/sso/open?url=…` (existing flow).
- **Off:** those cards open the app URL directly; user signs in via Keycloak if prompted.
- Preference persisted in `localStorage` (key e.g. `camunda-lab-auto-sso`), same pattern as theme.
- Lead copy and info banner reflect the current mode.
- Keycloak admin and non-SSO links stay direct either way.
- **Turning Auto sign-in off also opens Keycloak logout** — otherwise an existing SSO cookie still skips the login form (opt-out only skips Lab warm-up, not the browser session).

## UI

- Compact switch in the Apps `page-actions` row (with Sign out / Fix broken session / Show all addresses).
- Accessible: real checkbox or `role="switch"` with visible label; `title` tooltip explaining the effect.
- No per-card secondary actions.

## Out of scope

- Server-side / CLI config flag
- Per-app overrides
- Changing default credentials

## Success

- Toggle off → Operate opens without hitting `/api/v1/sso/open`
- Toggle on → warm path restored
- Preference survives page reload
