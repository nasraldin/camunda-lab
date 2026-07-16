# Lab UI Overview & CLI update (hybrid)

**Date:** 2026-07-17  
**Status:** Approved  
**Scope:** Overview as OSS entry, sidebar project footer, hybrid CLI update from UI

## Goals

- Overview is the professional entry to Camunda Lab (not only a lifecycle panel).
- Sidebar footer shows author + GitHub + Docs + Releases with proper icons.
- CLI card shows installed version vs latest GitHub release; Update uses the channel the user installed with (Homebrew vs release binary).

## Non-goals

- Auto-restart of `camunda ui` after binary replace (user starts a new process).
- Updating Camunda platform minors from this card (that stays on Setup).
- Full changelog rendering inside the UI (link to release notes).

## Sidebar footer

- Author name linking to `https://nasraldin.com`
- Icon links: GitHub (`https://github.com/nasraldin/camunda-lab`), Docs (`https://nasraldin.github.io/camunda-lab/`), Releases (`https://github.com/nasraldin/camunda-lab/releases`)
- Muted installed CLI version under the row

## Overview layout

1. Page head — title + short lead  
2. Lab status — configured state, version/profile/resources, container summary; Setup CTA if empty; Up / Down / Restart when configured  
3. CLI updates — current version, path, channel (`homebrew` | `release` | `dev`); latest tag + date; Update when newer; Release notes + Docs links; post-update restart hint  
4. Project strip — same destinations as footer  
5. Health — Doctor / Smoke (secondary)

## API

### `GET /api/v1/update`

Returns:

- `current` — CLI version string from binary  
- `latest` — latest GitHub release tag (or empty if unreachable)  
- `updateAvailable` — semver/tag compare (ignore leading `v`)  
- `channel` — `homebrew` | `release` | `dev`  
- `executable` — resolved path of running binary  
- `releaseURL` — HTML URL of latest release  
- `publishedAt` — ISO timestamp when available  
- `error` — optional fetch/detection message (non-fatal)

Channel detection:

- `homebrew` if executable path contains `Cellar/camunda-lab` or `brew --prefix`/opt path for the formula  
- `dev` if version contains `dev` / empty / `0.0.0`  
- else `release`

### `POST /api/v1/update`

Runs channel-specific update:

- `homebrew` → `brew upgrade camunda-lab` (or `brew update && brew upgrade camunda-lab`)  
- `release` → `curl -fsSL https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh | bash`  
- `dev` → reject with guidance to rebuild from source / install a release  

Returns `{ ok, output, channel, restartHint }`. Long-running; timeout ~5 minutes.

## Frontend

- Shared project URLs constant  
- Overview cards using existing design tokens (`section-title`, `hint`, `pill`, `btn`, banners)  
- Update button disabled when not available or channel is `dev` (show install/docs instead)

## Risks

- Replacing the running binary may terminate the UI process mid-response; surface restart hint before/after.  
- GitHub API rate limits without token — cache latest check briefly in-process (~5 min).  
- Homebrew may require network; surface stderr in UI.
