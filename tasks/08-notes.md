# Task 08 notes: node discovery, pairing, nodes UI

## Plan

Order of work (small commits):

1. `internal/discovery` (new, replaces the empty doc.go): `Announce` — mDNS
   `_shrinkyourshit._tcp` via `grandcat/zeroconf` (STACK.md pin) with TXT
   `version`, `name`, `id` (builtin node id, used to exclude self from the
   setup wizard list), plus a UDP broadcast responder on port 8766 that
   answers `SYS-DISCOVER` with a small JSON `{id,name,url,version}` (the
   URL is what the task asks for; id/name/version ride along so the wizard
   can list something readable). `Discover(ctx, timeout, Options) []ServerInfo`
   browses mDNS and probes broadcast addresses + 127.0.0.1 (both processes
   in the tests live on one machine), dedupes by URL. `Options{MDNS,
   Broadcast, Port}` toggles so each transport can be tested with the other
   disabled (acceptance 1).
2. `internal/store`: migration `0004_node_key_once.sql` — `nodes.key_once`
   holds the raw key until the approving node retrieves it once
   ("key is returned exactly once, then only the hash remains"); cleared on
   first retrieval, revoke and delete. Survives server restarts mid-pairing,
   which an in-memory map would not.
3. `internal/server`: `node.go` — keys become 32 random bytes base64url
   (task file); hash stays SHA-256 (documented choice: high-entropy key,
   argon2id adds nothing). Pairing service: `CreateToken` (24 h, single
   use), `PairWithToken` (→ approved + key), `PairPending` (→ pending), 
   `PollPending` (returns key once), `Approve`, `Revoke` (clears hashes,
   requeues in-flight jobs), `Delete`, `Update` (name/enabled/access_mode/
   path_mappings with validation). `queue.go` gains `RequeueNodeJobs`
   (claimed/running jobs of one node → queued, attempt+1, fail at max,
   via UpdateJobIfActive so a racing completion cannot be undone). Server
   gains a discovery hook + self id for `GET /setup/servers`.
4. `internal/config`: `SHRINK_DISCOVERY_PORT` (default 8766),
   `SHRINK_DISCOVERY_MDNS` and `SHRINK_DISCOVERY_BROADCAST` (default on) so
   CI can disable one transport each. Rows added to STACK.md.
5. `internal/api`: admin endpoints (GET /nodes, POST /nodes/tokens,
   POST /nodes/{id}/approve, POST /nodes/{id}/revoke, DELETE
   /nodes/{id}, PUT /nodes/{id}); open endpoints (POST /pair,
   GET /pair/{id}); `GET /setup/servers`; POST /setup accepts
   `mode: "join"` (server + optional token), writes `role: worker` +
   `server` into the data-dir config.yaml via a callback into main
   (`api.WithJoin`) and marks setup complete. Install commands are built
   from the request's own origin (r.Host, X-Forwarded-Proto aware).
6. `internal/worker`: `Client.Pair`/`PollPair` (Bearer omitted when no key
   yet); `EnsurePaired` first-run flow — use `SHRINK_SERVER` or discover,
   pair with token or as pending, persist the pending node id in
   `<data-dir>/node.json` so a restart resumes polling instead of creating
   duplicate pending nodes, print "Waiting for approval in the server UI",
   poll every 5 s, save `node.key` 0600.
7. `cmd/shrinkyourshit/main.go`: serve announces discovery; role `worker`
   in serve now runs the worker flow (pair if no key → run) so a joined
   install survives restarts from the same container/systemd unit;
   `POST /setup` join transitions the live process: HTTP shutdown →
   `EnsurePaired` → `workerpkg.Run`. `worker` subcommand runs
   `EnsurePaired` when `node.key` is missing.
8. Web: api.ts nodes/pair/setup-servers types; `NodesPage` (pending first
   with Approve/Reject, status badges, encoder chips, last heartbeat, edit
   dialog for name/enabled/access mode/path mappings, "Add node" token
   dialog with the three copyable commands); Dashboard nodes section live
   (per-node activity from nodes + running jobs); SetupWizard join card
   enabled with the discovered server list; route swap.
9. Docs in the same commits: API.md (task 08 endpoints + details + the two
   new setup endpoints + key/token formats), ARCHITECTURE.md (Node gains
   key_once; key format under Security), STACK.md config rows.
10. Tests per acceptance item (see below), `make test`, `make lint`,
    `make test-integration`, STATUS.md, PR.

## Acceptance → test mapping

1. Two processes + discovery: `cmd/shrinkyourshit` integration test builds
   the binary, starts serve (data dir A) and worker (data dir B) as real
   subprocesses, once with mDNS only and once with broadcast only
   (toggles via env); the worker discovers the server, pairs, completes a
   job on an ffmpeg fixture.
2. Token pairing: API-level unit tests (valid → approved + key, used →
   401, expired → 401, unknown → 401) + integration: token worker claims
   and completes a job (in-process stack, worker package).
3. Approval pairing: API-level tests (pending, claim 401, approve, poll →
   key once) + integration: pending worker cannot claim, gets key after
   approve, completes a job.
4. Revoke: API-level test (worker 401 after revoke) + integration: node's
   running job returns to queued and the built-in worker finishes it.
5. Shared mode + mapping: integration test in worker package: node with a
   path mapping to a second temp dir holding the same file; job spec
   source path resolves through the mapping; job completes.
6. Key once + never logged: unit tests — second poll returns no key; a
   slog capture over pair/approve/poll asserts the raw key and token never
   appear in log output; revoke clears the hash.
7. Nodes page: web tests — pending node visible, approve works, install
   commands contain the real server URL and token.

## Decisions taken inside scope

- Key format: 32 random bytes base64url, SHA-256 stored (task file allows
  the choice; documented in code + API.md + ARCHITECTURE.md).
- Broadcast reply is JSON `{id,name,url,version}` instead of a bare URL —
  strictly more information on the same wire format; the URL field is the
  contract, the rest feeds the wizard list.
- `nodes.key_once` column (migration 0004) instead of an in-memory key
  map: the raw key must survive a server restart between approve and the
  worker's first poll; it is wiped on first retrieval (only the hash
  remains afterwards, as the task specifies).
- `DELETE /api/v1/nodes/{id}` added to API.md (missing from the task 08
  table; the Reject button needs it — rejecting a pending node deletes the
  record, revoking keeps it as revoked).
- `GET /api/v1/setup/servers` added (none auth): the wizard's join card
  needs the discovered list; answers `{"servers":[]}` once setup is
  complete so a configured install stops advertising LAN scanning.
- POST /setup `mode:"join"`: stores role/server in the data-dir
  config.yaml (through a main-owned callback), marks setup complete, and
  the live serve process transitions to worker mode (HTTP shutdown → pair
  → worker loop). Serve with role worker runs the same flow, so the
  docker/systemd unit that always runs the default subcommand keeps
  working after a join.
- Pair duplicate-avoidance: the pending node id is persisted in
  `<data-dir>/node.json` and removed once the key is stored; retries poll
  the same node instead of stacking duplicate pending rows.
- Built-in node rename: the built-in node is displayed like any node;
  nothing in the pairing service treats it specially except
  EnsureBuiltinNode re-keying (pre-existing).

## Blocked / questions

None so far.

## Summary (end of run)

(to be written)
