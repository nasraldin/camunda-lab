# Docs site

This site is MkDocs Material, same general setup as [docker-lab](https://nasraldin.github.io/docker-lab/).

The header uses Camunda’s wordmark from their docs assets (`logo-camunda-black.svg`). Camunda® is a trademark of Camunda GmbH — this project is unofficial.

## Local preview

```bash
cd ~/homelab/camunda-lab
python3 -m venv .venv-docs
source .venv-docs/bin/activate
pip install -r requirements-docs.txt
mkdocs serve
```

Open http://127.0.0.1:8000/

## Deploy

GitHub Actions workflow `.github/workflows/docs.yml` builds on pushes to `main` that touch docs, then publishes to GitHub Pages.

Enable Pages in the repo: **Settings → Pages → Build and deployment → GitHub Actions**.

Public URL: [https://nasraldin.github.io/camunda-lab/](https://nasraldin.github.io/camunda-lab/)
