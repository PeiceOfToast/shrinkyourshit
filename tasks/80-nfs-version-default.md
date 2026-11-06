# Task 80: NFS default options must be vers=4.2

Fix for the second real-hardware blocker found on 2026-11-06 (see
`tasks/real-hardware-mount-evidence-2026-11-06.md`). Follows task 79; both are in
the v1.11 fix queue. The spec is `docs/design/encode-profiles-and-storage-2026-11-06.md`.

`DECISIONS.md` is read only.

## The defect

`internal/api/storage.go` sets the NFS default option string to:

```go
const nfsDefaultOptions = "vers=3,soft,timeo=50,retrans=2"
```

NFSv3 needs the NFS lock manager, and the kernel lock manager needs `rpc.statd`
(`rpc.lockd`'s companion) running on the client. Nothing in the container runs
it, so a v3 mount fails:

```
rpc.statd is not running
```

The same share mounts first try with `vers=4.2`, which has its own state
protocol and needs neither rpcbind nor statd. Verified on the deployed image
against the real NAS (`10.10.1.50:/mnt/DataSSD/SSD`): v3 fails, v4.2 mounts and
serves files.

A default that cannot work in the container the app ships in is a defect, not a
preference.

## Read first

`internal/api/storage.go` (`nfsDefaultOptions`, `defaultStorageOptions`, the
option allowlist and its `versionRE`), spec section 4 around line 108 where the
default and its reasoning are written down, `tasks/65-notes.md` and
`tasks/66-notes.md` where the v3 default was carried into tests.

## Do

1. Change the default to `vers=4.2,soft,timeo=50,retrans=2`. Keep `soft` for the
   original reason (a hard mount inside a container turns a NAS hiccup into an
   unkillable process).
2. Update every test that asserts the old default: `internal/api` storage tests,
   `internal/mount` argument vector tests, and any web test or fixture that shows
   the default options string in the storage form.
3. Update the spec section that states the v3 default so the document matches the
   code: record that v3 needs `rpc.statd`, which the container does not run, and
   that v4.2 is the default as a result. Quote the observed error string.
4. Help text: where the storage form explains the defaults, say that `vers=3` is
   allowed but needs `rpc.statd` inside the container, which the shipped image
   does not include. The operator should learn this before a mount fails, not
   after.
5. Confirm the allowlist accepts `4.2` as a version. Do not widen it for
   three-part versions in this task: the SMB `vers=3.1.1` rejection is a separate
   gap recorded as G1 in `tasks/75-notes.md`. If that turns out to be a one line
   change with a test, note it in the notes as a recommendation instead of
   quietly folding it in.
6. Do not change how CIFS options are built. CIFS has no version default in code;
   the SMB dialect comes from the server unless the operator sets `vers=`.

## Acceptance

- [x] `nfsDefaultOptions` is `vers=4.2,soft,timeo=50,retrans=2`.
- [x] A test asserts the exact default option string and the exact mount vector for a storage saved with empty options.
- [x] No test, fixture, i18n string or UI copy still shows `vers=3` as the default. Prove it with a repo wide grep in the notes.
- [x] The spec text matches the code and explains why, naming `rpc.statd`.
- [x] The storage form help text warns that `vers=3` needs `rpc.statd`, with a web test or a snapshot update covering the new copy.
- [x] An operator can still set `vers=3` explicitly; the validation does not refuse it.
- [x] `make test` green, `make lint` green, `DECISIONS.md` unchanged.

## Notes

Record the before and after strings, the grep output, and the G1 recommendation in
`tasks/80-notes.md`.
