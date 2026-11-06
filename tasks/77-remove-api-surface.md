# Task 77: Remove the deprecated library API surface

Part of the v1.12 wave, drop the library rule columns. Task 76 removed the code that wrote them.
This task removes the API shapes task 74 deprecated. `DECISIONS.md` is read only.

## Read first

`ROADMAP.md` v1.12, the `API.md` tasks 73 and 74 sections, and `tasks/74-notes.md`.

## Scope

`internal/api/handlers.go`, `internal/api/api.go` (routes), `internal/api/openapi.go` and the
generated spec if there is one, `web/src/api.ts` and the screens that send or read the removed
fields, `API.md`, `ARCHITECTURE.md`, plus the tests.

## Do

1. Remove the deprecated request body fields `target`, `easy` and `result_policy` from
   `POST /api/v1/libraries` and `PUT /api/v1/libraries/{id}`, and the `deprecated` array (type,
   population and reply field) from the save replies. The replies keep `profile_id` and keep the
   derived `target`, `easy`, `flow`, `flow_source`, `rule_tier`, `result_policy` read fields.
2. Remove `PUT /api/v1/libraries/{id}/tier` (route, handler, its tests). It has answered
   `409 follows_profile` since task 74. `PUT /api/v1/libraries/{id}/flow` stays as it is.
3. Decide and document the behaviour for a request that still sends a removed field, using what the
   decoder already does elsewhere in the package (strict `DisallowUnknownFields` or silent ignore).
   Do not invent a new convention. State the behaviour and the evidence in the notes, and cover it
   with a test.
4. Follow the change through the web client: no screen may send `target`, `easy` or
   `result_policy` on a library save, none may read `deprecated`, none may call the tier route.
   Update `web/src/api.ts` and every caller.
5. Update `API.md` (add a v1.12 section; leave the older sections as history with a superseded
   note), the OpenAPI document, and `ARCHITECTURE.md` where it names the deprecated fields.
6. No schema change in this task. Task 78 drops the columns.

## Acceptance

- [x] A library save that sends `target`, `easy` or `result_policy` behaves exactly as the notes
      say (one test), and no reply carries `deprecated`.
- [x] `PUT /api/v1/libraries/{id}/tier` is gone (route table test and API.md).
- [x] The web client sends and reads neither the removed fields nor the removed route (grep output
      in the notes).
- [x] OpenAPI and API.md match the code.
- [x] `make test` and `make lint` green.

## Notes

Plan, the decoder decision, grep evidence and the 5 line summary go in `tasks/77-notes.md`.
