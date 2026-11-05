# Task 63: Verify the wave on real media and document it

Part of the v1.10 encode settings tiers wave. The specification is `docs/design/encode-settings-tiers-2026-11-04.md`;
this file carries only the scope and the acceptance checklist for this one task. Read the spec
sections named below plus `AGENTS.md`, `DECISIONS.md`, `STACK.md` and `ARCHITECTURE.md`.

`DECISIONS.md` is read only. The owner decisions this wave implements are recorded in the spec,
section 2.

## Read first

Everything above, plus `references`-style verification rules in `AGENTS.md`: verification is
evidence, not a self report.

## Scope

`tasks/63-notes.md`, `ARCHITECTURE.md`, `API.md`, `ROADMAP.md`, `tasks/STATUS.md`.

## Do

1. On real media the worker can encode, run three flows: a Basic library, a Moderate flow with a
   condition and a scale, and an Advanced flow with `raw_args`. For each, capture from the job log
   the actual ffmpeg command line.
2. Prove the outcomes: output height matches the scale target, rate control shows the CRF or
   bitrate argued, a per step hardware choice shows the encoder it selected, and the `raw_args`
   argument appears verbatim in the command line.
3. Prove the downgrade rule end to end through the API: advanced graph, downgrade, read the graph
   back, byte identical.
4. Update `ARCHITECTURE.md` (flow engine vocabulary), `API.md` (the tier endpoint and the flow
   query parameter), `ROADMAP.md` (the wave is delivered), and set every STATUS row done with its
   PR link.

## Acceptance

- [x] The three job logs are quoted in `tasks/63-notes.md` with the relevant line from each.
- [x] Output heights are confirmed with ffprobe, not by eye.
- [x] The downgrade round trip is proven with a diff, not an assertion in prose.
- [x] Docs updated, `make test` and `make lint` green.

## Notes

`tasks/63-notes.md` is the evidence file for the whole wave.
