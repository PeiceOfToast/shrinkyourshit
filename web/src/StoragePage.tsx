// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Storage page (task 68, spec 4.6): the network shares this server mounts
 * at /storage/<name>. One Card lists them: name, protocol, source, mounted
 * state, the mounter's last error, and how many libraries sit inside each
 * share. Add and edit are one dialog. Test, mount, unmount, edit and remove
 * are row actions, and every mutation answers with a toast.
 *
 * Login gate (spec 4.6): while login is off the page is read only. It says
 * why and links to App settings, and every write control is disabled. The
 * server answers the same writes with 403 login_required, so the page never
 * depends on the disabled state alone.
 *
 * Secrets: the SMB password is typed into the form and sent once. A stored
 * password is never returned, so the form shows "(unchanged)" instead. The
 * page renders only the fields it names, never a whole response.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ApiError,
  createStorage,
  deleteStorage,
  getLibraries,
  getSettings,
  getStorages,
  mountStorage,
  testStorage,
  unmountStorage,
  updateStorage,
  type Storage,
  type StorageProtocol,
} from './api'
import Badge from './Badge'
import Banner from './Banner'
import Card from './Card'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import EmptyState from './EmptyState'
import ErrorBox from './ErrorBox'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'
import { Field } from './FormSection'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import Switch from './Switch'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import {
  blankStorageForm,
  fieldErrorsOf,
  librariesUsing,
  removeText,
  sourcePlaceholder,
  storageBodyOf,
  storageFormOf,
  storageState,
  storagesKey,
  ALLOWED_OPTIONS,
  NFS_DEFAULT_OPTIONS,
  type StorageForm,
} from './storageModel'
import {
  btnDangerGhost,
  btnGhost,
  btnPrimary,
  input,
  linkTouch,
  select,
} from './uiControls'

/** Field paths the form shows inline. Any other path shows at the top. */
const FORM_FIELDS = [
  'name',
  'protocol',
  'source',
  'options',
  'username',
  'domain',
  'password',
] as const

const LOGIN_REASON =
  'Storage is read only while login is off. Storage can reach your network, so turn on login in App settings before you add, test or mount a share.'

export default function StoragePage() {
  useDocumentTitle(['Storage'])
  const client = useQueryClient()
  const toast = useToast()

  const settings = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  const storages = useQuery({
    queryKey: storagesKey,
    queryFn: getStorages,
    // The watchdog changes mount state on its own; poll so the row follows.
    refetchInterval: 15_000,
  })
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: getLibraries })

  // Writes stay off until settings say login is on. A failed settings read
  // keeps them off too: unknown is not a reason to open the gate.
  const loginOn = settings.data?.login_enabled === true
  const loginOff = settings.data?.login_enabled === false
  const writable = loginOn

  const [form, setForm] = useState<StorageForm | null>(null)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [deleting, setDeleting] = useState<Storage | null>(null)
  const [testResult, setTestResult] = useState<string | null>(null)

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: storagesKey })
  }

  const save = useMutation({
    mutationFn: (input: { id: string; form: StorageForm }) => {
      const body = storageBodyOf(input.form)
      return input.id ? updateStorage(input.id, body) : createStorage(body)
    },
    onSuccess: (row) => {
      toast.success(`Share “${row.name}” saved.`)
      invalidate()
      setForm(null)
      setErrors({})
    },
    onError: (err: unknown) => {
      setErrors(fieldErrorsOf(err))
      toast.failure('Saving the share failed', err)
    },
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteStorage(id),
    onSuccess: () => {
      toast.success('Share removed.')
      invalidate()
      setDeleting(null)
    },
    onError: (err: unknown) => toast.failure('Removing the share failed', err),
  })

  const test = useMutation({
    mutationFn: (id: string) => testStorage(id),
    onSuccess: (result, id) => {
      const name = storages.data?.items.find((s) => s.id === id)?.name ?? ''
      if (result.ok) {
        setTestResult(
          `The test passed. “${name}” mounted and unmounted cleanly.`,
        )
        toast.success(`Test passed for “${name}”.`)
      } else {
        const reason = result.error || 'the share did not mount'
        setTestResult(null)
        toast.failure(`Testing “${name}” failed`, new Error(reason))
      }
      invalidate()
    },
    onError: (err: unknown) => toast.failure('Testing the share failed', err),
  })

  const mount = useMutation({
    mutationFn: (id: string) => mountStorage(id),
    onSuccess: (row) => {
      toast.success(`Mounted at ${row.mountpoint}.`)
      invalidate()
    },
    onError: (err: unknown) => {
      toast.failure('Mounting the share failed', err)
      invalidate()
    },
  })

  const unmount = useMutation({
    mutationFn: (id: string) => unmountStorage(id),
    onSuccess: (row) => {
      toast.success(`Unmounted ${row.mountpoint}.`)
      invalidate()
    },
    onError: (err: unknown) => {
      toast.failure('Unmounting the share failed', err)
      invalidate()
    },
  })

  // Row actions are disabled for a share with a call in flight. Other
  // shares stay usable; the server serialises per share the same way.
  const busyIds = new Set(
    [test, mount, unmount, remove]
      .filter((m) => m.isPending && typeof m.variables === 'string')
      .map((m) => m.variables as string),
  )

  const openAdd = () => {
    setErrors({})
    setForm(blankStorageForm())
  }
  const openEdit = (row: Storage) => {
    setErrors({})
    setForm(storageFormOf(row))
  }

  const setProtocol = (next: StorageProtocol) => {
    if (!form) return
    // Adding: the options follow the protocol's default. Editing keeps them.
    const options = form.id
      ? form.options
      : next === 'nfs'
        ? NFS_DEFAULT_OPTIONS
        : ''
    setForm({ ...form, protocol: next, options })
  }

  const rows = storages.data?.items ?? []
  const libs = libraries.data?.items ?? []
  const hostPolicy = storages.data?.host_policy
  const notAvailable =
    storages.error instanceof ApiError && storages.error.status === 501

  const rowLocked = (row: Storage) => !writable || busyIds.has(row.id)

  const columns: Column<Storage>[] = [
    {
      key: 'name',
      header: 'Share',
      primary: true,
      cell: (r) => (
        <div className="min-w-0">
          <p className="font-semibold text-fg">{r.name}</p>
          <p
            className="mt-0.5 truncate font-mono text-caption font-normal text-fg-muted"
            title={`${r.protocol.toUpperCase()} · ${r.source}`}
          >
            <span>{r.protocol.toUpperCase()}</span>
            <span aria-hidden="true"> · </span>
            <span>{r.source}</span>
          </p>
          <p className="mt-0.5 truncate font-mono text-caption text-fg-muted">
            {r.mountpoint}
          </p>
        </div>
      ),
    },
    {
      key: 'state',
      header: 'State',
      cardPlacement: 'badge',
      cell: (r) => {
        const s = storageState(r)
        return (
          <div className="flex flex-col items-start gap-1">
            <Badge tone={s.tone} dot={r.mounted}>
              {s.label}
            </Badge>
            {r.error && (
              <p className="max-w-xs break-words text-caption text-danger">
                {r.error}
              </p>
            )}
          </div>
        )
      },
    },
    {
      key: 'used',
      header: 'Used by',
      cell: (r) => {
        const count = librariesUsing(r, libs)
        return count === 1 ? '1 library' : `${count} libraries`
      },
    },
    {
      key: 'actions',
      header: '',
      actions: true,
      cell: (r) => rowActions(r, false),
      cardCell: (r) => rowActions(r, true),
    },
  ]

  // The card view repeats every row, so its test ids carry a -card suffix.
  const rowActions = (row: Storage, onCard: boolean) => {
    const locked = rowLocked(row)
    const id = (name: string) =>
      `storage-${name}-${row.id}${onCard ? '-card' : ''}`
    return (
      <div className="flex flex-wrap justify-end gap-2">
        <button
          type="button"
          className={btnGhost}
          disabled={locked}
          data-testid={id('test')}
          onClick={() => test.mutate(row.id)}
        >
          Test
        </button>
        {row.mounted ? (
          <button
            type="button"
            className={btnGhost}
            disabled={locked}
            data-testid={id('unmount')}
            onClick={() => unmount.mutate(row.id)}
          >
            Unmount
          </button>
        ) : (
          <button
            type="button"
            className={btnGhost}
            disabled={locked || !row.enabled}
            data-testid={id('mount')}
            onClick={() => mount.mutate(row.id)}
          >
            Mount
          </button>
        )}
        <button
          type="button"
          className={btnGhost}
          disabled={locked}
          data-testid={id('edit')}
          onClick={() => openEdit(row)}
        >
          Edit
        </button>
        <button
          type="button"
          className={btnDangerGhost}
          // The server answers 409 while mounted, so the button waits too.
          disabled={locked || row.mounted}
          title={row.mounted ? 'Unmount this share first' : undefined}
          data-testid={id('delete')}
          onClick={() => setDeleting(row)}
        >
          Remove
        </button>
      </div>
    )
  }

  // One test id per button on the page: the header's and the empty state's.
  const addButton = (testId: string) => (
    <button
      type="button"
      className={btnPrimary}
      disabled={!writable}
      data-testid={testId}
      onClick={openAdd}
    >
      Add share
    </button>
  )

  const editing = form !== null && form.id !== ''
  const unmatched = Object.entries(errors).filter(
    ([path]) => !(FORM_FIELDS as readonly string[]).includes(path),
  )

  return (
    <PageContainer className="space-y-6" data-testid="storage-page">
      <PageHeader
        title="Storage"
        subtitle="Network shares the server mounts, so a library can point at a folder inside one."
        actions={addButton('storage-add')}
      />

      {loginOff && (
        <Banner
          tone="warning"
          title="Read only: login is off"
          testId="storage-readonly"
          action={
            <Link
              to="/app"
              className={linkTouch}
              data-testid="storage-readonly-link"
            >
              Go to App settings
            </Link>
          }
        >
          {LOGIN_REASON}
        </Banner>
      )}

      {hostPolicy?.error && (
        <Banner
          tone="danger"
          testId="storage-host-policy-error"
          title="No share can be mounted"
        >
          {hostPolicy.error}
        </Banner>
      )}
      {hostPolicy && !hostPolicy.error && (
        <Banner
          tone="info"
          testId="storage-host-policy"
          title={
            hostPolicy.restricted
              ? 'Only listed hosts can be mounted'
              : 'Any host can be mounted'
          }
        >
          {hostPolicy.message}
          {hostPolicy.restricted && hostPolicy.allowed.length > 0 && (
            <span className="mt-1 block font-mono text-caption text-fg-muted">
              {hostPolicy.allowed.join(', ')}
            </span>
          )}
        </Banner>
      )}

      {testResult && (
        <Banner
          tone="success"
          testId="storage-test-result"
          action={
            <button
              type="button"
              className={btnGhost}
              onClick={() => setTestResult(null)}
            >
              Dismiss
            </button>
          }
        >
          {testResult}
        </Banner>
      )}

      <Card
        as="section"
        padding="none"
        title="Shares"
        description="Each share mounts at /storage/<name> inside the server. Nothing to edit in compose or fstab."
      >
        {notAvailable && (
          <div className="p-4 sm:p-6">
            <Banner tone="info" testId="storage-unavailable">
              Network storage is not available on this server. Its mount tools
              or permissions are missing, so shares cannot be mounted here.
            </Banner>
          </div>
        )}
        {!notAvailable && storages.isPending && <SkeletonRows rows={3} />}
        {!notAvailable && storages.isError && (
          <div className="p-4 sm:p-6">
            <ErrorBox
              title="Cannot load the shares."
              error={storages.error}
              onRetry={() => void storages.refetch()}
            />
          </div>
        )}
        {!notAvailable && storages.data && rows.length === 0 && (
          <EmptyState
            icon="folder"
            title="No network shares yet"
            body="Add an NFS or SMB share and the server mounts it for you. Libraries can then use folders inside it."
            action={writable ? addButton('storage-add-empty') : undefined}
            testId="storage-empty"
          />
        )}
        {!notAvailable && rows.length > 0 && (
          <DataList
            rows={rows}
            columns={columns}
            testid="storage-list"
            rowTestId={(r) => `storage-row-${r.id}`}
            cardRowTestId={(r) => `storage-card-${r.id}`}
          />
        )}
      </Card>

      {form && (
        <ModalDialog
          label={editing ? 'Edit share' : 'Add a share'}
          testId="storage-form"
          size="md"
          onClose={() => setForm(null)}
        >
          <DialogHeader
            title={editing ? 'Edit share' : 'Add a share'}
            onClose={() => setForm(null)}
          />
          <DialogBody>
            <div className="flex flex-col gap-4">
              {unmatched.length > 0 && (
                <Banner tone="danger" testId="storage-form-errors">
                  {unmatched.map(([path, message]) => (
                    <span key={path} className="block">
                      {message}
                    </span>
                  ))}
                </Banner>
              )}

              <Field
                label="Name"
                htmlFor="storage-name"
                help="Letters, numbers and dashes. The share mounts at /storage/<name>. The name cannot change later."
                error={errors.name}
              >
                <input
                  id="storage-name"
                  className={input}
                  value={form.name}
                  disabled={editing}
                  data-testid="storage-name"
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="movies"
                />
              </Field>

              <Field
                label="Protocol"
                htmlFor="storage-protocol"
                help={
                  editing
                    ? 'The protocol cannot change. Add a new share to use another one.'
                    : undefined
                }
                error={errors.protocol}
              >
                <select
                  id="storage-protocol"
                  className={select}
                  value={form.protocol}
                  disabled={editing}
                  data-testid="storage-protocol"
                  onChange={(e) =>
                    setProtocol(e.target.value as StorageProtocol)
                  }
                >
                  <option value="nfs">NFS (most NAS boxes)</option>
                  <option value="smb">SMB (Windows shares)</option>
                </select>
              </Field>

              <Field
                label="Source"
                htmlFor="storage-source"
                help={
                  form.protocol === 'smb'
                    ? 'Host and share, for example //192.168.1.10/media. No spaces.'
                    : 'Host and export path, for example 192.168.1.10:/export/media. No spaces.'
                }
                error={errors.source}
              >
                <input
                  id="storage-source"
                  className={input}
                  value={form.source}
                  data-testid="storage-source"
                  onChange={(e) => setForm({ ...form, source: e.target.value })}
                  placeholder={sourcePlaceholder(form.protocol)}
                />
              </Field>

              <Field
                label="Mount options"
                htmlFor="storage-options"
                optional
                help={
                  <>
                    Comma separated. Allowed: {ALLOWED_OPTIONS.join(', ')}.
                    {form.protocol === 'nfs' &&
                      ' The NFS default is version 4.2, soft, with a short timeout. A hard mount turns a NAS hiccup into a process that cannot be stopped, and a soft mount fails the job instead. Change it only if you know why. vers=3 is allowed, but it needs rpc.statd for locking, and this container does not run it, so a vers=3 mount fails with "rpc.statd is not running".'}
                  </>
                }
                error={errors.options}
              >
                <input
                  id="storage-options"
                  className={input}
                  value={form.options}
                  data-testid="storage-options"
                  onChange={(e) =>
                    setForm({ ...form, options: e.target.value })
                  }
                  placeholder={
                    form.protocol === 'nfs' ? NFS_DEFAULT_OPTIONS : 'ro,nosuid'
                  }
                />
              </Field>

              {form.protocol === 'smb' && (
                <>
                  <Field
                    label="Username"
                    htmlFor="storage-username"
                    optional
                    error={errors.username}
                  >
                    <input
                      id="storage-username"
                      className={input}
                      value={form.username}
                      autoComplete="off"
                      data-testid="storage-username"
                      onChange={(e) =>
                        setForm({ ...form, username: e.target.value })
                      }
                    />
                  </Field>
                  <Field
                    label="Domain"
                    htmlFor="storage-domain"
                    optional
                    error={errors.domain}
                  >
                    <input
                      id="storage-domain"
                      className={input}
                      value={form.domain}
                      autoComplete="off"
                      data-testid="storage-domain"
                      onChange={(e) =>
                        setForm({ ...form, domain: e.target.value })
                      }
                    />
                  </Field>
                  <Field
                    label="Password"
                    htmlFor="storage-password"
                    optional
                    help={
                      form.password_set
                        ? 'A password is saved. Leave this blank to keep it. It is never shown again.'
                        : 'Saved encrypted on this server. It is never shown again.'
                    }
                    error={errors.password}
                  >
                    <input
                      id="storage-password"
                      className={input}
                      type="password"
                      value={form.password}
                      autoComplete="new-password"
                      data-testid="storage-password"
                      placeholder={form.password_set ? '(unchanged)' : ''}
                      onChange={(e) =>
                        setForm({ ...form, password: e.target.value })
                      }
                    />
                  </Field>
                </>
              )}

              <Switch
                checked={form.read_only}
                label="Read only"
                visibleLabel="Read only: the server never writes to this share"
                testId="storage-read-only"
                onChange={(next) => setForm({ ...form, read_only: next })}
              />
              <Switch
                checked={form.enabled}
                label="Mount at startup"
                visibleLabel="Mount at startup and keep it mounted"
                testId="storage-enabled"
                onChange={(next) => setForm({ ...form, enabled: next })}
              />
            </div>
          </DialogBody>
          <DialogFooter>
            <button
              type="button"
              className={btnGhost}
              data-testid="storage-cancel"
              onClick={() => setForm(null)}
            >
              Cancel
            </button>
            <button
              type="button"
              className={btnPrimary}
              disabled={save.isPending || !writable}
              data-testid="storage-save"
              onClick={() => save.mutate({ id: form.id, form })}
            >
              {save.isPending
                ? 'Saving…'
                : editing
                  ? 'Save changes'
                  : 'Add share'}
            </button>
          </DialogFooter>
        </ModalDialog>
      )}

      {deleting && (
        <ConfirmDialog
          title="Remove this share?"
          danger
          confirmLabel="Remove"
          confirmTestId="storage-delete-confirm"
          cancelTestId="storage-delete-cancel"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(deleting.id)}
          onClose={() => setDeleting(null)}
        >
          <p>
            Remove “{deleting.name}”?{' '}
            {removeText(librariesUsing(deleting, libs))}
          </p>
        </ConfirmDialog>
      )}
    </PageContainer>
  )
}
