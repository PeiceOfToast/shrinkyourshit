// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Integrations page (task 23): the media servers and download clients
 * that hear about every shrunk file, so the library at home updates
 * itself. Rows are DataList cards (task 20) with the shared 44px control
 * classes; the add/edit form keeps the key secret — a saved key shows
 * "(unchanged)" in its field and never travels back out of the API.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createIntegration,
  deleteIntegration,
  getIntegrations,
  testIntegration,
  updateIntegration,
  type Integration,
  type IntegrationBody,
} from './api'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import ErrorBox from './ErrorBox'
import PageContainer from './PageContainer'
import { SkeletonRows } from './skeleton'
import Switch from './Switch'
import { useToast } from './toastContext'
import { btn, btnGhost, btnPrimary, input } from './uiControls'

const KINDS = [
  { id: 'jellyfin', label: 'Jellyfin' },
  { id: 'emby', label: 'Emby' },
  { id: 'plex', label: 'Plex' },
  { id: 'sonarr', label: 'Sonarr' },
  { id: 'radarr', label: 'Radarr' },
]

function kindLabel(kind: string): string {
  return KINDS.find((k) => k.id === kind)?.label ?? kind
}

/** The form: blank for "add", prefilled for edit (api_key stays blank). */
interface FormState {
  id: string
  kind: Integration['kind']
  name: string
  base_url: string
  api_key: string
  enabled: boolean
  refresh_on_done: boolean
}

function blankForm(): FormState {
  return {
    id: '',
    kind: 'jellyfin',
    name: '',
    base_url: '',
    api_key: '',
    enabled: true,
    refresh_on_done: true,
  }
}

function formOf(row: Integration): FormState {
  return {
    id: row.id,
    kind: row.kind,
    name: row.name,
    base_url: row.base_url,
    api_key: '',
    enabled: row.enabled,
    refresh_on_done: row.refresh_on_done,
  }
}

export default function IntegrationsPage() {
  const client = useQueryClient()
  const toast = useToast()
  const list = useQuery({
    queryKey: ['integrations'],
    queryFn: getIntegrations,
  })

  const [form, setForm] = useState<FormState | null>(null)
  const [deleting, setDeleting] = useState<Integration | null>(null)
  const [testResult, setTestResult] = useState<{
    id: string
    text: string
  } | null>(null)

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ['integrations'] })
  }

  const save = useMutation({
    mutationFn: (input: { id: string; body: IntegrationBody }) =>
      input.id
        ? updateIntegration(input.id, input.body)
        : createIntegration(input.body),
    onSuccess: () => {
      toast.success('Integration saved.')
      invalidate()
      setForm(null)
    },
    onError: (err: unknown) =>
      toast.failure('Saving the integration failed', err),
  })

  // The row's on/off switch flips the integration without opening the
  // form: an empty api_key means "keep the stored key" (API.md).
  const toggle = useMutation({
    mutationFn: (input: { row: Integration; enabled: boolean }) =>
      updateIntegration(input.row.id, {
        kind: input.row.kind,
        name: input.row.name,
        base_url: input.row.base_url,
        api_key: '',
        enabled: input.enabled,
        refresh_on_done: input.row.refresh_on_done,
      }),
    onSuccess: () => invalidate(),
    onError: (err: unknown) =>
      toast.failure('Saving the integration failed', err),
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteIntegration(id),
    onSuccess: (_result, id) => {
      toast.success('Integration deleted.')
      setDeleting(null)
      invalidate()
      if (form?.id === id) setForm(null)
    },
    onError: (err: unknown) => {
      toast.failure('Deleting the integration failed', err)
      setDeleting(null)
    },
  })

  const test = useMutation({
    mutationFn: (id: string) => testIntegration(id),
    onSuccess: (result, id) => {
      const name = list.data?.items.find((i) => i.id === id)?.name ?? id
      setTestResult({
        id,
        text: result.ok
          ? `Reached ${name} — the server said it is listening.`
          : (result.error ??
            `${name} did not answer (status ${result.status}).`),
      })
    },
    onError: (err: unknown) =>
      toast.failure('Testing the integration failed', err),
  })

  const editing = form !== null && form.id !== ''

  const submit = () => {
    if (!form) return
    save.mutate({
      id: form.id,
      body: {
        kind: form.kind,
        name: form.name,
        base_url: form.base_url,
        api_key: form.api_key,
        enabled: form.enabled,
        refresh_on_done: form.refresh_on_done,
      },
    })
  }

  // Row controls take an onCard flag: the card tree's test ids carry a
  // -card suffix (the task 20 pattern), so tests target one tree.
  const rowSwitch = (row: Integration, onCard: boolean) => (
    <Switch
      checked={row.enabled}
      label={`${row.name} on/off`}
      testId={`integration-switch-${row.id}${onCard ? '-card' : ''}`}
      onChange={(next) => {
        if (form?.id === row.id) {
          setForm({ ...form, enabled: next })
          return
        }
        toggle.mutate({ row, enabled: next })
      }}
    />
  )

  const rowActions = (row: Integration, onCard: boolean) => (
    <div className="flex flex-wrap gap-2">
      <button
        type="button"
        className={btn}
        disabled={test.isPending}
        data-testid={`integration-test-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => test.mutate(row.id)}
      >
        Test
      </button>
      <button
        type="button"
        className={btnGhost}
        data-testid={`integration-edit-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setForm(formOf(row))}
      >
        Edit
      </button>
      <button
        type="button"
        className={btnGhost}
        data-testid={`integration-delete-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setDeleting(row)}
      >
        Delete
      </button>
    </div>
  )

  const columns: Column<Integration>[] = [
    { key: 'kind', header: 'Kind', cell: (r) => kindLabel(r.kind) },
    { key: 'name', header: 'Name', cell: (r) => r.name, primary: true },
    {
      key: 'base_url',
      header: 'Address',
      cell: (r) => (
        <span className="break-all text-slate-300">{r.base_url}</span>
      ),
    },
    {
      key: 'enabled',
      header: 'On',
      cell: (r) => rowSwitch(r, false),
      cardCell: (r) => rowSwitch(r, true),
    },
    {
      key: 'refresh_on_done',
      header: 'On finish',
      cell: (r) => (r.refresh_on_done ? 'Yes' : 'No'),
      hideInCard: (r) => !r.refresh_on_done,
    },
    {
      key: 'actions',
      header: '',
      cell: (r) => rowActions(r, false),
      cardCell: (r) => rowActions(r, true),
      cardNoLabel: true,
    },
  ]

  return (
    <PageContainer className="space-y-6" data-testid="integrations-page">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold">Integrations</h1>
          <p className="mt-1 max-w-xl text-sm text-slate-400">
            After a file is shrunk, your media server or download client can be
            told to rescan — without anyone clicking Scan. Each entry here is
            one server to notify.
          </p>
        </div>
        <button
          type="button"
          className={btnPrimary}
          data-testid="integrations-add"
          onClick={() => setForm(blankForm())}
        >
          Add integration
        </button>
      </div>

      {testResult && (
        <p
          role="status"
          className={`rounded-2xl p-4 text-sm ring-1 ${
            testResult.text.startsWith('Reached')
              ? 'bg-emerald-950/40 text-emerald-200 ring-emerald-900/60'
              : 'bg-red-950/40 text-red-200 ring-red-900/60'
          }`}
          data-testid="integration-test-result"
        >
          {testResult.text}
        </p>
      )}

      {list.isPending && (
        <>
          <h2 className="text-lg font-semibold">Configured</h2>
          <SkeletonRows rows={2} />
        </>
      )}
      {list.isError && (
        <ErrorBox title="Cannot load the integrations." error={list.error} />
      )}
      {list.data && (
        <section>
          <h2 className="text-lg font-semibold">Configured</h2>
          {list.data.items.length === 0 ? (
            <p className="mt-2 text-sm text-slate-400">
              Nothing configured yet — add your media server above.
            </p>
          ) : (
            <DataList
              rows={list.data.items}
              columns={columns}
              testid="integrations-list"
              rowTestId={(r) => `integration-row-${r.id}`}
              cardRowTestId={(r) => `integration-card-${r.id}`}
            />
          )}
        </section>
      )}

      {form && (
        <section
          className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800"
          data-testid="integration-form"
        >
          <h2 className="text-lg font-semibold">
            {editing ? 'Edit integration' : 'Add integration'}
          </h2>
          <div className="mt-4 grid max-w-xl gap-4">
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Kind</span>
              <select
                className={input}
                value={form.kind}
                onChange={(e) =>
                  setForm({
                    ...form,
                    kind: e.target.value as Integration['kind'],
                  })
                }
                data-testid="integration-kind"
              >
                {KINDS.map((k) => (
                  <option key={k.id} value={k.id}>
                    {k.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Name</span>
              <input
                type="text"
                className={input}
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="Living room TV"
                data-testid="integration-name"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Address</span>
              <input
                type="text"
                className={input}
                value={form.base_url}
                onChange={(e) => setForm({ ...form, base_url: e.target.value })}
                placeholder="http://media.local:8096"
                data-testid="integration-base-url"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">
                API key
                {editing && (
                  <span className="ml-1 text-slate-500">
                    (leave blank to keep the saved one)
                  </span>
                )}
              </span>
              <input
                type="password"
                className={input}
                value={form.api_key}
                onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                placeholder={editing ? '(unchanged)' : 'Paste the API key'}
                data-testid="integration-api-key"
              />
            </label>
            <div className="flex items-center justify-between gap-4">
              <span className="text-sm text-slate-300">On</span>
              <Switch
                checked={form.enabled}
                label="Integration on/off"
                testId="integration-enabled"
                onChange={(next) => setForm({ ...form, enabled: next })}
              />
            </div>
            <div className="flex items-center justify-between gap-4">
              <span className="text-sm text-slate-300">
                Rescan after every finished file
              </span>
              <Switch
                checked={form.refresh_on_done}
                label="Rescan after every finished file"
                testId="integration-refresh-on-done"
                onChange={(next) => setForm({ ...form, refresh_on_done: next })}
              />
            </div>
          </div>
          <div className="mt-4 flex gap-2">
            <button
              type="button"
              className={btnPrimary}
              disabled={save.isPending}
              data-testid="integration-save"
              onClick={submit}
            >
              {save.isPending ? 'Saving…' : 'Save'}
            </button>
            <button
              type="button"
              className={btnGhost}
              data-testid="integration-cancel"
              onClick={() => setForm(null)}
            >
              Cancel
            </button>
          </div>
        </section>
      )}

      {deleting && (
        <ConfirmDialog
          title="Delete integration"
          danger
          confirmLabel="Delete"
          confirmTestId="integration-delete-confirm"
          cancelTestId="integration-delete-cancel"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(deleting.id)}
          onClose={() => setDeleting(null)}
        >
          <p>
            Delete “{deleting.name}”? The server stops telling it about finished
            files, and the saved API key is removed.
          </p>
        </ConfirmDialog>
      )}
    </PageContainer>
  )
}
