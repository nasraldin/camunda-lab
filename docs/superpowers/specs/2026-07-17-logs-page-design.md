# Logs page enhancement design

**Date:** 2026-07-17  
**Status:** Approved  
**Scope:** Lab UI `/logs` page only (client-side). No API changes.

## Goal

Make Logs match the Services page language (friendly names, clearer controls) and add find-in-logs with **Filter** (default) and **Highlight** modes.

## Behavior

### Layout

1. Page head: title, lead, link to Services.
2. Toolbar card:
   - Service `<select>` with friendly labels (shared name map with Services).
   - Actions: Show recent · Follow live · Stop following · Clear screen.
   - Search: “Find in logs…” (case-insensitive substring on loaded lines).
   - Mode chips: **Filter** | **Highlight** (Filter default).
   - Meta: line counts / match counts; live pill when following.
3. Log viewer (`<pre>` / line list) below.

### Search

| Mode | Behavior |
|------|----------|
| Filter | Show only matching lines. Meta: `Showing N of M lines`. Empty hint if no matches. |
| Highlight | Show all lines; wrap match substrings. Meta: `N matches highlighted`. |

- Search applies to the in-memory buffer (max ~2000 lines), including while following.
- Empty search = show all lines (no filter / no highlights).

### Streaming

- Unchanged: EventSource `GET /api/v1/logs/{service}?follow=0|1`.
- Changing service: stop stream, clear buffer, update `?service=` query.
- Auto-scroll while following only if the user is near the bottom of the viewer.

### Out of scope

Regex, download, multi-service merge, server-side grep, backend changes.

## Files

- `internal/ui/web/src/pages/Logs.tsx` — rewrite UI + search
- `internal/ui/web/src/serviceNames.ts` — shared friendly names (extract from Containers)
- `internal/ui/web/src/pages/Containers.tsx` — import shared names
- `internal/ui/web/src/styles.css` — logs toolbar / mark styles
