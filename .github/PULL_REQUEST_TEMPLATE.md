## What & why

<One-line summary. Ticket refs: `Refs #…` (the maintainer closes tickets after
review).>

## Reviewer checklist (before merge)

- [ ] **Data contract traced end to end:** every field/value flows intact
      through every layer that transforms it (e.g. handler, serializer,
      transport, client mapping, UI); no layer silently drops or renames it.
- [ ] Every layer that reshapes a payload has a test asserting the **whole**
      contract (all fields), not a subset. Prefer one shared builder over
      duplicated hand-built objects so they cannot drift.
- [ ] **Real path verified, not just unit tests:** drove the running code,
      captured the real payload at the boundary, confirmed the observable
      output. Green units plus approved reviews are not proof it works.
- [ ] No dead, speculative, or unwired code; no "known gaps".
- [ ] Tests cover happy, error, and boundary paths.
- [ ] Docs updated; CHANGELOG under `[Unreleased]` (committed at release).
- [ ] Cross-surface parity where relevant (server and all clients, CLI and GUI,
      code and docs).

## Verification evidence

<How the real path was verified: payloads, screenshots, or test output.>
