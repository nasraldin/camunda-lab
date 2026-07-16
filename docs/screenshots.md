# App screenshots

These shots were taken from a real **camunda-lab** install:

```bash
camunda install --version 8.9 --profile light --resources small --yes
camunda wait
camunda smoke
```

Stack: Camunda **8.9.13** orchestration + connectors (light profile). Default login: **demo** / **demo**.

Open the same UIs on your machine with `camunda urls` and `camunda open operate`.

---

## Operate — login

`http://localhost:8080/operate` (redirects to `/operate/login`)

![Operate login](assets/screenshots/operate-login.png)

Footer shows the patch version from Camunda’s images (here **8.9.13**).

---

## Operate — dashboard

After login you land on the empty Operate dashboard — expected on a fresh lab with no process instances yet.

![Operate dashboard](assets/screenshots/operate-dashboard.png)

---

## Operate — processes

Processes view with Active / Incidents filters. No diagrams until you deploy something.

![Operate processes](assets/screenshots/operate-processes.png)

```bash
camunda open operate
```

---

## Tasklist

`http://localhost:8080/tasklist` — human tasks from your BPMN. Empty until you start a process with user tasks.

![Tasklist](assets/screenshots/tasklist.png)

```bash
camunda open tasklist
```

---

## Admin (orchestration)

`http://localhost:8080/admin` — users, groups, roles, authorizations for the orchestration cluster (light profile uses embedded auth, not Keycloak).

![Admin users](assets/screenshots/admin.png)

The default **demo** user is created by Camunda’s compose initialization.

```bash
camunda open admin
```

---

## Connectors health

Connectors do not ship a full product UI on the light stack. Health is exposed for ops checks:

`http://localhost:8086/actuator/health`

![Connectors health JSON](assets/screenshots/connectors-health.png)

All components report **UP**, including `zeebeClient` with a healthy partition.

---

## What this profile does *not* include

Light profile is intentionally small. You will **not** see these UIs until you switch to **full**:

| App | Typical URL (full) |
| --- | --- |
| Console | http://localhost:8087 |
| Optimize | http://localhost:8083 |
| Identity | http://localhost:8084 |
| Web Modeler | http://localhost:8070 |
| Keycloak | http://localhost:18080/auth/ |

```bash
camunda profile full
camunda wait
camunda urls
```

Ports also differ by minor — especially **8.7** vs **8.8** vs **8.9+**. See [Profiles and versions](profiles.md).

---

## Reproduce these shots

```bash
brew upgrade camunda-lab   # or: make install
camunda version            # expect a recent release
camunda install --version 8.9 --profile light --resources small --yes
camunda wait
camunda smoke
camunda urls
camunda open operate
```
