# Architecture

```text
camunda (Go CLI)
  └── ~/.camunda-lab/
        ├── config.yaml
        ├── versions/<minor>/     ← official camunda-distributions zip
        ├── overlays/             ← 8.10 ES helper
        └── resources.env         ← JAVA_TOOL_OPTIONS from resource profile
  └── docker compose -p camunda-lab -f …
```

Stack source of truth: [camunda/camunda-distributions](https://github.com/camunda/camunda-distributions).
We do not rewrite OIDC/Keycloak wiring — only thin overlays.
