# FAQ

## Is this an official Camunda product?

No. Unofficial community project. The stack comes from Camunda’s published Compose zips; the CLI and docs are ours.

## Can I use this in production?

No. Same guidance as Camunda: Compose files are for local development. Production → [Helm](https://docs.camunda.io/docs/self-managed/setup/install/).

## Which versions work?

**8.7, 8.8, 8.9, and 8.10.**

8.10 is labeled preview here because Elasticsearch isn’t bundled the same way — we add a helper overlay for the full profile.

## Light vs full — what do I get?

**Light:** orchestration (Zeebe + Operate + Tasklist on newer minors), connectors, Elasticsearch on ≤8.9.

**Full:** that plus Optimize, Console, Identity, Keycloak, Postgres, Web Modeler.

**Modeler:** Web Modeler and its dependencies only.

Details shift a bit by minor; we map profiles to the files Camunda ships. See [profiles and versions](profiles.md).

## Default password?

`demo` / `demo` for the apps. Keycloak admin on full is `admin` / `admin`. Change those if you expose anything beyond localhost.

## Why is my laptop struggling?

Full profile + Elasticsearch is heavy. Start with:

```bash
camunda install --profile light --resources small --yes
```

## Do I need a local LLM for AI / MCP?

No. MCP talks to Cursor/Claude (their models). The AI Agent connector uses a cloud API key (OpenAI/Anthropic) or an optional OpenAI-compatible URL (Ollama, etc.). See [AI and MCP](ai-mcp.md).

```bash
camunda ai enable --openai-key "$OPENAI_API_KEY"
camunda ai config
```

Needs Camunda **8.9+** and profile **light** or **full**.

## MCP returns 401 on full profile — is it broken?

Expected. Full compose uses OIDC. Use `camunda ai config` for the `mcp-proxy` snippet (and `camunda tools c8ctl install` if needed). Light profile allows direct HTTP to `/mcp/cluster`.

## When does ElasticVue show up?

When the active profile publishes Elasticsearch on the host (`:9200`). That’s light on ≤8.8 and full on supported minors — not modeler, and not 8.9+ light. Open with `camunda open elasticvue`.

## Does it work on Windows?

Not yet. macOS and Linux only.

## Where does data live?

`~/.camunda-lab/` (or `CAMUNDA_LAB_HOME`). `camunda nuke` removes it after a confirm.

## Trademark?

“Camunda” is Camunda’s mark. We use the CLI name for clarity; the repo and Homebrew formula are `camunda-lab`. If that becomes a problem, we’ll rename the binary — open an issue.
