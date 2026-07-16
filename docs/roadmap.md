# Roadmap

Honest list — not a promise calendar.

## Done in v1

- Official zip download for 8.7–8.10
- light / full / modeler profiles
- install, switch, doctor, wait, smoke, urls, nuke
- c8ctl + Desktop Modeler helpers
- Docs site + CI scaffolding

## Next up

- First GitHub Release + real Homebrew sha256s
- Harden URL maps against each minor’s compose ports (especially 8.7)
- Optional Cosign verify when `cosign` is installed
- LIVE smoke in CI on a schedule (expensive — maybe nightly, not every PR)

## Later

- Named labs (`camunda --name upgrade-test`) for side-by-side minors
- Windows support if there’s demand
- Thin Kind/Helm bridge for people who outgrew Compose but like the same CLI verbs

Ideas welcome in [issues](https://github.com/nasraldin/camunda-lab/issues).
