# Task 50 notes: Integrations and Plugins (spec 5.12)

## Plan

1. `CodeBlock.tsx`: add an optional `copyTestId` prop so the webhook secret keeps `webhook-secret-copy`. Default unchanged.
2. `IntegrationsPage.tsx`: PageHeader (title, subtitle from 5.12), `Tabs` under it (wrapper keeps `integrations-tabs`),
   one Card per tab (`padding="none"`, primary action in the header, DataList or EmptyState in the body), add/edit
   forms and the secret as ModalDialog with DialogHeader/Body/Footer and `Field`.
3. `PluginsPage.tsx`: same pattern for the Plugins tab (Card header with the existing paragraph, "Install plugin"
   primary, install form and settings as dialogs, Test result as a Banner with a CodeBlock for output).
4. Tests: added tests only (see below). No `api.ts`, no Go, no DECISIONS.md, no new dependency.

## Changed assertions

None. No existing assertion was edited or removed. The diff to both test files is additions only.

Added tests (all in the existing test files, appended at the end):
- `IntegrationsPage.test.tsx` "IntegrationsPage layout (task 50)":
  - Tabs under the header; `?tab=plugins` selects the plugins tab (Tabs, `integrations-tabs`, tab test ids).
  - Primary action sits in the Card header (`integrations-add`, Card h2 "Media servers", one accent button).
  - EmptyState on the servers tab (`integrations-empty`).
  - EmptyState on the webhooks tab (`webhooks-empty`).
  - State badges: "Not tested" when on and untested, "Disabled" when off, "Connected" after a good Test.
  - Webhook rows carry a state badge and the add form is a DIALOG.
  - Secret dialog: DIALOG, warning text, CodeBlock `pre` with `whitespace-pre-wrap`, `webhook-secret-copy`.
- `PluginsPage.test.tsx` "the plugins tab layout (task 50)":
  - EmptyState on the plugins tab (`plugins-empty`).
  - State badge flips Disabled to Enabled after the switch PUT (the GET mock is unshifted so the refetch returns the enabled row).
  - Install plugin in the Card header.

## Test ids

All old `data-testid` values survive. Checked by comparing the old and new page files. `integrations-tab-*`,
`plugin-install-mode-*` and `webhook-event-*` moved into `TABS` / `testId` props and are still present.

## Judgement calls (not in the spec)

1. **"Connected" and "Not connected" come from the Test button, not the API.** The API has no connection state, and
   spec 5.12 says no API change. So Connected or Not connected appears only after a Test in this page visit. An enabled
   row with no Test yet says **"Not tested"**. That is a fourth label the spec does not name. Disabled rows say
   "Disabled". Owner: pick a different wording, or add a field in a later task.
2. **Plugin badges** (the spec names none): "Enabled" (success), "Disabled" (neutral), "Manifest error" (danger).
3. **Failed Test uses `Banner tone="warning"`, not `danger`.** The existing test expects `role="status"` on the failed
   result. `danger` would give `role="alert"`. Warning reads as "needs a look", which fits.
4. **Dialogs.** Server add/edit, webhook add/edit, deliveries, settings, the secret, and plugin install are all
   ModalDialog. The spec's list does not name plugin install; I made it a dialog so every "add" works the same way.
5. **The secret is a dialog** (the spec's list names it). It has a Done button and no backdrop dismiss. Escape or the
   close button also dismisses it, and the banner says the secret is not shown again.
6. **Labels.** Row action "Remove" (spec 5.12) instead of "Delete". Confirm dialogs read "Remove this server?" and
   "Remove". Test ids unchanged. Save buttons say "Save changes" when editing and "Add server" / "Add webhook" when
   creating (spec 3.6). In-flight text is "Saving…".
7. **Kind column removed from the servers table.** Kind and URL are the mono sub-line under the name (spec 5.12 layout).
   The text is split into separate spans, so `getAllByText('Jellyfin')` still matches.
8. **Card wraps DataList and SkeletonRows.** Both draw their own surface. Spec 3.4 says "never nest a Card in a Card",
   but spec 3.5 gives the DataList wrapper a `card` class. Visually the list reads as one box, so I did not change
   DataList. Owner: decide whether DataList needs a "bare" mode for use inside a Card.
9. **Empty-state action duplicates the header primary.** Spec 5.12's EmptyState example names an action, so on an empty
   tab there are two green "Add …" buttons. Spec 2.1 says one emerald fill per view. Owner: keep the empty-state action
   or drop it.
10. **`CodeBlock` gained `copyTestId`.** It is a shared primitive. The change is backward compatible, and its existing
    test still passes.
11. **Tab selection** is still read from `?tab=` on mount only, and it is not written back to the URL. That is unchanged
    from before.
12. **Row action buttons** no longer wrap. The actions column is `w-px`, so a wrapping row stacked Test, Edit and
    Remove vertically on desktop. Fixed by dropping `flex-wrap` on the action group.

## Verification

Node 20.18.1 and Go 1.23.4 are not on the PATH here. I downloaded both into `/tmp/tools` (outside the repo). `make` is
not installed here, so I ran the Makefile steps by hand: `web` build, copy to `internal/webui/dist`, and `go build`.

- `web`: `npx prettier --check .` clean. `npm run lint` clean (`--max-warnings 0`). `npx tsc --noEmit` clean.
  `npx vitest run`: 58 files, 416 tests passed (was 407; +9 new). `npm run build` clean.
- `go test ./...`: every package ok. No Go source changed.
- `npx playwright test` against the built binary: 62 passed, 6 skipped. The 6 skips are the ffmpeg-dependent file-table
  tests, the same as before. Ports 8712 and 8799 were already served by other `shrinkyourshit` processes (pids 1480109
  and 1481462). I did not touch them. I ran the suite on a free port instead.
- Visual check (screenshots, not committed) at 1440 and 390 px: Servers, Webhooks and Plugins tabs, and the Add server
  dialog. Found and fixed the wrapped action buttons described above. The e2e suite does not cover `/integrations`
  (task 53 adds it to the audit).
- `DECISIONS.md` unchanged. `api.ts` unchanged. No dependency added.

## Summary

1. **Built:** the Integrations page (PageHeader, Tabs, one Card per tab with the primary in the header, DataList rows
   with state Badges, EmptyStates, dialogs with `Field`, CodeBlock plus warning Banner for the secret) and the Plugins
   tab on the same pattern. `CodeBlock` gained `copyTestId`.
2. **Skipped:** the browser audit at 320, 390, 414 and 768 px for `/integrations` (task 53). No new API or Go work.
3. **Owner calls:** the "Not tested" wording (judgement 1), the nested surfaces (8), and the duplicate empty-state action
   (9).
4. **Next task (53) should know:** add `/integrations` to the audit. The empty-state action is a spec example, so the
   audit should not flag two accent buttons on an empty tab. The open task 49 questions (`w-32` loses to `w-full`,
   `worker_concurrency` unread) still stand, and nothing here changes them.
5. **Not done here:** a real-browser check of the Plugins install dialog and the settings dialog. Only the Add server
   dialog was screenshotted.
