# Agent guide

This repository is the Go SDK. Its public peers are Rust, Python, TypeScript and
Kotlin in `ekoDB/ekodb-client`.

## Required SDK parity audit

Before implementing or reporting any SDK contract change complete, compare all
five actual implementations, exports, tests, examples and docs. Read the
canonical
[sdk-parity skill](https://github.com/ekoDB/ekodb-client/blob/main/.agents/skills/sdk-parity/SKILL.md),
or its local file in the paired client checkout. Codex, Claude and Copilot use
that same source. An unreadable skill is not a successful audit. The public
[parity contribution rules](https://github.com/ekoDB/ekodb-client/blob/main/PARITY_MATRIX.md#contributing)
apply.

Verify suspected gaps in source and search existing issues before ticketing. Put
confirmed work on the affected repository board and take it In Progress. Record
per-SDK source and transport-behavior evidence in the tracking issue and draft
PR. A passing Go suite does not demonstrate peer implementation parity. A
missing or unassessed required SDK prevents a full-parity completion claim;
explicit user scope exclusions must remain visible.

Preserve omitted/null/empty-array request states and response metadata. Keep
admin facades separate from data CRUD, sanitize errors and avoid replaying
ambiguous creation failures. Update examples and docs with the implementation.
Public artifacts must not name private repositories, internal tickets or paths.

## Verification

From the branch checkout run `make lint-fix && make fmt`, inspect all changes,
then `make lint && make vet && make test && make build`. `make test` includes
race detection. Live writes require applicable explicit authorization; this
guide grants none. No version bump or publication without authorization.

The recorded admin fixture is vendored from the public client contract so this
Go module can test offline as a standalone clone. The parity audit compares it
with the canonical fixture; do not independently change its contract.
