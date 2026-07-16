# AI and MCP

Turn on Camunda’s **Orchestration Cluster MCP**, **Processes MCP** (8.10+), and **AI Agent connector** secrets from the CLI. You don’t need a local LLM.

Camunda’s Compose already enables MCP on 8.9+. camunda-lab makes the endpoints easy to find, injects connector secrets, and prints client config (including the full-profile OIDC bridge).

## Prerequisites

- Camunda **8.9+** (`light` or `full` — not `modeler`)
- A running lab (`camunda install` / `camunda up`)
- At least one provider for the AI Agent connector:
  - OpenAI API key, and/or
  - Anthropic API key, and/or
  - OpenAI-compatible base URL (optional Ollama/LM Studio — not required)

## Quick start

```bash
# On a running lab
camunda ai enable --openai-key "$OPENAI_API_KEY"
camunda ai status
camunda ai config
camunda urls   # lists mcp-cluster (and mcp-processes on 8.10+)

# Or at install time
camunda install --version 8.9 --profile light --resources small --yes \
  --ai --openai-key "$OPENAI_API_KEY"
```

Secrets land in `~/.camunda-lab/ai.env` (mode `0600`):

```bash
SECRET_OPENAI_API_KEY=…
SECRET_ANTHROPIC_API_KEY=…
SECRET_OPENAI_BASE_URL=…   # optional
```

Connectors pick these up via a lab overlay. `camunda ai enable` recreates the **connectors** service only.

## MCP clients (Cursor / Claude)

```bash
camunda ai config
```

**Light profile** — direct Streamable HTTP (no auth):

- Cluster: `http://localhost:8080/mcp/cluster`
- Processes (8.10+): `http://localhost:8080/mcp/processes`

**Full profile** — MCP returns **401** (OIDC). `camunda ai config` prints a `c8ctl mcp-proxy` STDIO snippet. Install the helper with `camunda tools c8ctl install` if needed.

## Disable

```bash
camunda ai disable
camunda ai disable --wipe-secrets   # also delete ai.env
```

## Official Camunda docs

- [Orchestration Cluster MCP](https://docs.camunda.io/docs/apis-tools/orchestration-cluster-api-mcp/orchestration-cluster-api-mcp-overview/)
- [Enable and connect](https://docs.camunda.io/docs/apis-tools/orchestration-cluster-api-mcp/orchestration-cluster-api-mcp-setup/)
- [Processes MCP](https://docs.camunda.io/docs/apis-tools/processes-mcp/processes-mcp-overview/)
- [AI agents](https://docs.camunda.io/docs/components/agentic-orchestration/ai-agents/)
- [Connectors secrets](https://docs.camunda.io/docs/self-managed/components/connectors/connectors-configuration/)
