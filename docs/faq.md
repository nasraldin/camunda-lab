# FAQ

## Is this an official Camunda product?

No. Unofficial community project. Stack bits come from Camunda’s published Compose zips; the CLI and docs are ours.

## Can I use this in production?

No. Same guidance as Camunda: Compose files are for local development. Production → [Helm](https://docs.camunda.io/docs/self-managed/setup/install/).

## Which versions work?

**8.7, 8.8, 8.9, and 8.10.**  
8.10 is labeled preview here because Elasticsearch isn’t bundled the same way — we add a helper overlay for the full profile.

## Light vs full — what do I actually get?

**Light:** orchestration (Zeebe + Operate + Tasklist on newer minors), connectors, Elasticsearch (on ≤8.9).

**Full:** that plus Optimize, Console, Identity, Keycloak, Postgres, Web Modeler.

**Modeler:** Web Modeler and its dependencies only.

Details change slightly by minor; we map profiles to the files Camunda ships. See [profiles and versions](profiles.md).

## Default password?

`demo` / `demo` for the apps. Keycloak admin on full is `admin` / `admin`. Change those if you expose anything beyond localhost.

## Why is my Mac melting?

Full profile + Elasticsearch is hungry. Start with:

```bash
camunda install --profile light --resources small --yes
```

## Does it work on Windows?

Not in v1. macOS and Linux only.

## Where does data live?

`~/.camunda-lab/` (or `CAMUNDA_LAB_HOME`). `camunda nuke` removes it after a confirm.

## Trademark?

“Camunda” is Camunda’s mark. We use the CLI name for clarity; the repo and Homebrew formula are `camunda-lab`. If that ever becomes a problem, we’ll rename the binary — say something in an issue.
