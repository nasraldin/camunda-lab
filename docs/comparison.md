# Why Camunda Lab

Camunda already ships good Compose files. We don’t reinvent them. We automate the boring parts: download, pick a profile, wait for healthy, print URLs, switch minors, wipe clean.

## Compared to the official zip

Camunda’s [distributions repo](https://github.com/camunda/camunda-distributions) is the source of truth. You can unzip and run `docker compose up` yourself.

What you still do by hand:

- Remember which file is light vs full (and that 8.7 names things differently)
- Wait on Keycloak without a clear “ready” signal
- Hunt for Operate’s port after a coffee break
- Move from 8.8 to 8.9 without leaving half-broken volumes behind
- Wire MCP client config and AI Agent connector secrets yourself

`camunda install` / `switch` / `wait` / `urls` / `ai` cover that checklist.

## Compared to Helm on Kind

Helm is the right production story. Locally it usually means:

1. Install Kind (or k3d, minikube, …)
2. Learn enough kubectl to stay sane
3. Install the chart and values for your minor
4. Port-forward half a dozen services

Fine if you’re testing ingress. Overkill if you only need Operate and a gRPC gateway for a plugin you’re writing.

Camunda Lab stays on Docker Compose on purpose.

## Compared to Camunda 8 Run

[Camunda 8 Run](https://docs.camunda.io/docs/self-managed/quickstart/developer-quickstart/c8run/) is a lightweight local runtime — great for modeling and quick API work. It is **not** the full management stack (Identity, Optimize, Console, Web Modeler as in the full Compose file).

Use 8 Run when you want minimal. Use this lab when you want the Compose profiles Camunda publishes — including full.

## Compared to official cluster CLIs

Official tools such as [`c8ctl`](https://docs.camunda.io/docs/apis-tools/c8ctl/getting-started/) talk _to_ a cluster: deploy BPMN, watch files, inspect instances.

Camunda Lab gets the cluster **up**. Then:

```bash
camunda tools c8ctl install
```

…and use the official CLI against the lab. We don’t try to replace resource management.

Over time Lab also grows into a **developer and platform toolkit** (project scaffold, semantic BPMN diff/lint/review, deeper doctor, env profiles, deploy _preview_, drift, incident/trace helpers, thin kubectl wrappers). Those features fill gaps official CLIs intentionally leave — analysis, diagnostics, and GitOps-style preview — while deploy/start-instance stay with official tooling. See the [roadmap](roadmap.md) and [platform toolkit vision](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-platform-toolkit-vision.md).

## Who this is for

- Platform folks validating a minor before a Helm upgrade
- Developers who need Operate + connectors locally without Kubernetes
- Anyone wiring Cursor/Claude to Camunda MCP without hand-editing compose
- Anyone who’s lost an afternoon to “which compose file was the full one again?”
- (Upcoming) Teams who want BPMN review, secrets scan, and ops helpers next to their local lab
