# Roadmap

Honest list — not a promise calendar.

## Done

- Official zip download for 8.7–8.10
- light / full / modeler profiles
- install, switch, doctor, wait, smoke, urls, nuke
- ElasticVue overlay when host Elasticsearch is published
- `camunda ai` — MCP client config + AI Agent connector secrets (8.9+)
- c8ctl + Desktop Modeler helpers
- GitHub Releases + Homebrew tap (`nasraldin/tools`)
- Docs site + CI scaffolding

## Next up

- Optional Cosign verify when `cosign` is installed
- LIVE smoke in CI on a schedule (expensive — maybe nightly, not every PR)
- Optional `--write-cursor` to install MCP JSON into the user’s Cursor config

## Later

- Named labs (`camunda --name upgrade-test`) for side-by-side minors
- Windows support if there’s demand
- Thin Kind/Helm bridge for people who outgrew Compose but like the same CLI verbs
- Sample AI Agent BPMN blueprint deploy helper

Ideas welcome in [issues](https://github.com/nasraldin/camunda-lab/issues).
