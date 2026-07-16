# Why Camunda Lab

Short version: Camunda already ships good Compose files. We don’t reinvent them. We make the boring parts (download, version, wait, URLs, wipe) a one-command habit.

## Compared to the official zip

Camunda’s [distributions repo](https://github.com/camunda/camunda-distributions) is the source of truth. You *can* download a zip and run `docker compose up` yourself. Plenty of people do.

What you still end up doing by hand:

- Remember which file is light vs full (and that 8.7 names things differently)
- Wait on Keycloak without a clear “ready” signal
- Hunt for Operate’s port after coffee
- Switch from 8.8 to 8.9 without leaving half-broken volumes behind

`camunda install` / `switch` / `wait` / `urls` are that checklist, automated.

## Compared to Helm on Kind

Helm is the right production story. Locally it means:

1. Install Kind (or k3d, minikube, …)
2. Learn enough kubectl to not hate life
3. Install the chart and values for your minor
4. Port-forward half a dozen services

Fine if you’re a platform engineer testing ingress. Overkill if you only need Operate and a gRPC gateway for a plugin you’re writing.

Camunda Lab stays on Docker Compose on purpose.

## Compared to Camunda 8 Run

[Camunda 8 Run](https://docs.camunda.io/docs/self-managed/quickstart/developer-quickstart/c8run/) is a lightweight local runtime — great for modeling and quick API work. It is **not** the full management stack (Identity, Optimize, Console, Web Modeler as in the full Compose file).

Use 8 Run when you want minimal. Use this lab when you want the Compose profiles Camunda publishes — including full.

## Compared to `c8ctl`

[`c8ctl`](https://docs.camunda.io/docs/apis-tools/c8ctl/getting-started/) talks *to* a cluster: deploy BPMN, watch files, inspect instances.

Camunda Lab gets the cluster **up**. Then:

```bash
camunda tools c8ctl install
```

…and use `c8` / `c8ctl` against the lab. We don’t try to replace it.

## Who this is for

- DevOps / platform folks validating a minor before a Helm upgrade
- Developers who need Operate + connectors locally without k8s
- Anyone who’s lost an afternoon to “which compose file was the full one again?”
