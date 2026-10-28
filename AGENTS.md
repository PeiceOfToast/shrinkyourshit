# Instructions for Coding Agents

You are building ShrinkYourShit. Read this file, then `DECISIONS.md`, `STACK.md`, `ARCHITECTURE.md`, `API.md`, and your assigned file in `tasks/`.

## Kickoff prompt (owner pastes this into OpenCode)

```
Read AGENTS.md and follow it. Open tasks/STATUS.md, take the first task whose status is `todo`,
set it to `in_progress`, and complete it according to its task file. Stop when the task's
acceptance checklist is fully ticked and STATUS.md is updated, or when you are blocked.
```

## Rules

1. One task per run. Finish it or report blocked. Never start the next task.
2. `DECISIONS.md` is read-only. If a decision blocks you, stop and write the question in `tasks/NN-notes.md`, set the task to `blocked` in `tasks/STATUS.md`, and end the run.
3. Use only what `STACK.md` lists. Adding a dependency not listed there requires a one-line justification in the notes file and must not duplicate something already listed.
4. Endpoints come from `API.md`. If a task needs an endpoint that is missing, add it to `API.md` following the conventions, in the same commit.
5. Data shapes and enums come from `ARCHITECTURE.md`. Do not add status values or Target fields without updating that file in the same commit.
6. Stay inside the task's scope. Write out-of-scope ideas in the notes file instead of building them.
7. AGPL-3.0. Every new Go and TypeScript source file starts with `// SPDX-License-Identifier: AGPL-3.0-only`.
8. FFmpeg and ffprobe are external binaries called as subprocesses. Never reimplement them or add Go bindings.
9. Never hardcode video-only assumptions into core types (store, queue, job). Media-specific logic lives in `internal/probe`, `internal/encoders`, `internal/flow`.
10. Never write to, rename, or delete a user's original file until the output has been verified. Never write bookkeeping files into a library folder.
11. Tests: every acceptance item has at least one test. Unit tests must pass without ffmpeg. Tests that need ffmpeg use build tag `integration` and skip with a clear message when ffmpeg is absent. CI installs ffmpeg and runs both.
12. Generate test media with ffmpeg (`-f lavfi -i testsrc`, `-f lavfi -i sine`) in test setup. Never commit media files.
13. Branch `task/NN-short-name` from `main`. Commit small, message `task NN: <what>`. Open a PR when done. CI must be green.
14. Keep the UI plain-language on user-facing screens: no codec names or FFmpeg terms on the easy screen.

## Loop

1. Read the task file and the four reference docs.
2. Write a short plan in `tasks/NN-notes.md` (what files you will touch, in what order).
3. Implement in small commits.
4. Run `make test` (and `make test-integration` if ffmpeg is present) and `make lint`.
5. Tick each acceptance box in the task file only when its test passes.
6. Update `tasks/STATUS.md`: `done` or `blocked`, with the PR link or the blocking question.
7. Write a 5-line summary at the end of the notes file: what was built, what was skipped, what the next task should know.
8. Stop.

## If something in the docs is wrong

Docs can have mistakes. If a doc contradicts another doc or a task is impossible as written, do not guess. Record the contradiction in the notes file, mark the task `blocked`, and stop. The owner resolves it.
