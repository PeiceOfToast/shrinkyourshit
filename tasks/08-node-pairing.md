# Task 08: Node discovery, pairing, nodes UI

## Goal
A second machine running the same binary finds the server, asks to join, the admin approves it (or it used a token), and it starts receiving jobs. Shared storage only for now; file transfer is task 09.

## Read first
API.md (task 08 endpoints), ARCHITECTURE.md (one binary three roles, Node model, security), STACK.md (mDNS, config), DECISIONS.md (13, 16). Depends on task 07.

## Do
- `internal/discovery`: server advertises `_shrinkyourshit._tcp` with TXT `version` and `name`; UDP broadcast on port 8766 as fallback (request "SYS-DISCOVER", reply with URL). `Discover(ctx, timeout) []ServerInfo`.
- Pairing (`internal/api`, `internal/server`):
  - `POST /nodes/tokens`: creates a one-time token (24h), returns the token plus ready-to-copy commands: Docker run, `curl -fsSL https://<server>/install.sh | sh -s -- --server <url> --token <token>`, and the PowerShell equivalent. The install script endpoint itself is task 09; here the commands are strings.
  - `POST /pair` with a valid token: create node `approved`, return key. Without token: create `pending`, return id. `GET /pair/{id}`: returns key once approved (key is returned exactly once, then only the hash remains).
  - Approve, revoke, update endpoints. Revoking deletes the key hash; an in-flight job on that node is returned to `queued`.
  - Keys: 32 random bytes, base64url; server stores argon2id hash (or SHA-256 is acceptable for high-entropy keys; pick one and document it).
- `worker` subcommand first-run flow: if `<data-dir>/node.key` is missing, discover servers (or use `SHRINK_SERVER`), call `/pair` with `SHRINK_TOKEN` if set, otherwise wait for approval by polling `/pair/{id}` every 5s, print "Waiting for approval in the server UI", save the key, then run.
- `serve` with role `both` on a fresh data dir, when a server is discovered on the LAN: the setup wizard's "Join an existing server" card becomes enabled and lists discovered servers; choosing one switches the install to role `worker` and starts the pairing flow (store `role=worker` in the data dir config).
- Shared storage path mappings: node reports the library roots it can read (it tries the server's library paths plus mappings from `PUT /nodes/{id}`). Access check per ARCHITECTURE.md B7 is implemented in task 09; for now a node in `shared` mode must have mappings configured by the admin and the worker resolves paths through them.
- `/web`: Nodes page with status badges, capabilities (encoder chips), last heartbeat, running jobs; pending nodes shown at the top with Approve/Reject; "Add node" dialog generates a token and shows the three commands with copy buttons; node edit for name, enabled, access mode, path mappings. Dashboard per-node activity now live.

## Out of scope
File transfer, the install script endpoint, installer service setup (task 09).

## Acceptance
- [ ] Integration test: two processes on one machine with separate data dirs; worker discovers the server via mDNS (or broadcast when mDNS is unavailable in CI; test both with the other disabled).
- [ ] Token pairing: worker with a valid token is approved immediately and claims a job; a used or expired token is rejected with 401.
- [ ] Approval pairing: worker without a token appears `pending`, cannot claim (401), receives the key after admin approval, then claims a job.
- [ ] Revoke: node's next request gets 401; its running job returns to `queued` and is completed by the built-in worker.
- [ ] Shared mode with a path mapping (`/media` on the server maps to a different temp dir on the "node") resolves the source path correctly and the job completes.
- [ ] Node key is never returned after the first retrieval and never logged.
- [ ] Nodes page shows the pending node, the approve action works, and the generated install command contains the real server URL and token.
