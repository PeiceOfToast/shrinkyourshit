# Task 74: Result policy on the profile, deprecate library rule columns

Part of the v1.11 encode profiles and storage wave. The specification is `docs/design/encode-profiles-and-storage-2026-11-06.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. Owner decisions for this wave are in the spec, section 2.

## Read first
Spec 5.4. Every reader of `libraries.flow`, `libraries.easy`, `libraries.rule_tier` and
`libraries.result_policy`, found by grep before changing anything.

## Scope
`internal/server/*.go` readers, `internal/store/*.go`, `web/src/api.ts`, `ARCHITECTURE.md`,
`API.md`, `ROADMAP.md`, plus the tests that touch those fields.

## Do
1. Serve and consume the graph, tier and result policy from the profile everywhere. The library
   endpoints keep accepting the old fields for one release and ignore them with a deprecation
   note in the response, so an old client does not silently do something different.
2. Add a startup log line when any library row still has a non empty legacy field, so the operator
   knows the drop is safe.
3. Write the follow up removal as its own ROADMAP entry with the exact columns to drop, and do not
   drop them in this task.
4. Update `ARCHITECTURE.md` and `API.md`.

## Acceptance
- [ ] No code path reads the legacy columns as truth any more, proven by grep output in the notes.
- [ ] An old client payload still succeeds and the response marks the deprecated fields.
- [ ] Docs updated and the removal task is in ROADMAP.
- [ ] `make test` and `make lint` green.

## Notes
Write the plan, the grep evidence and the deprecation message in `tasks/74-notes.md`.
