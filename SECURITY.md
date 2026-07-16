# Security Policy

## Supported versions

| Version | Supported |
| --- | --- |
| Latest release (`v*`) | Yes |
| `main` | Best-effort |
| Older tags | No |

Camunda Lab is a **local development** CLI. It downloads official Camunda Compose distributions and runs them via Docker. Treat the lab as untrusted local tooling: do not expose it to the public internet.

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.

Email **security@nasraldin.com** (or open a private [GitHub security advisory](https://github.com/nasraldin/camunda-lab/security/advisories/new)) with:

- Description of the issue
- Steps to reproduce
- Affected version / commit
- Impact assessment if known

You should receive an acknowledgement within a few days. We will coordinate a fix and disclosure timeline.

## Scope

In scope examples:

- Supply-chain issues in `install.sh` / release artifacts
- Token handling in CI (Homebrew publish)
- Path traversal or unsafe extraction of release zips / Camunda distributions
- Command injection via untrusted config

Out of scope:

- Vulnerabilities in upstream Camunda, Docker, Keycloak, Elasticsearch
- Misconfiguration of a local lab you intentionally exposed
- Denial of service against your own laptop

## Prefer signed / checksummed installs

- GitHub Releases publish `checksums.txt` — `install.sh` verifies the archive digest
- Prefer Homebrew or a verified release binary over piping unknown scripts to `bash` in production-like environments
