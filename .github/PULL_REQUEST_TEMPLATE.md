## Summary

<!-- What and why (1–3 bullets). -->

-

## Test plan

- [ ] `make check` (fmt, tidy, vet, test)
- [ ] `make ui-check` if UI source changed (`internal/ui/web/src`, `package.json`, etc.)
- [ ] CI green (`Test & build`, `Docs` when `docs/` changed)
- [ ] Docs updated if user-facing behavior changed
- [ ] If CLI UX changed: `camunda about` / `camunda urls` still make sense for 8.7 and 8.9
- [ ] If release/Homebrew/install paths changed: verify `docs/releasing.md` and `Formula/camunda-lab.rb`

## Release / install impact

<!-- Delete section if N/A. -->

- [ ] No change to install channels
- [ ] Homebrew tap / `install.sh` / release workflow considered
