# Phase 0 Follow-ups

Deferred items from the Phase 0 documentation effort; each is out of scope for Phase 0.

- CI job that runs `terraform validate` on every example (via a `dev_overrides` build) to mechanically guarantee example accuracy — out of scope for Phase 0.
- Backfill Go schema `Description` fields (in `internal/provider/resource_*/*_gen.go` sources) so any future doc generation emits clean, user-facing text — the Go sources still contain internal engineering phrasing that must be kept out of docs by hand today — out of scope for Phase 0.
- Add by-name lookup data sources (e.g. custom_property by name) — a provider (Go) enhancement; the cross-project-portability guide currently notes this as a roadmap item — out of scope for Phase 0.
- Automated per-resource maturity badges (currently the maturity table in docs/index.md is maintained by hand from the acceptance-test file list) — out of scope for Phase 0.
- Publishing surfaces: GitHub Pages / GitHub wiki / Notion — the committed markdown is the corpus these will derive from — out of scope for Phase 0.
- CHANGELOG discipline for the provider releases — out of scope for Phase 0.
