// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Integrations page (task 23, 25 and 31): three tabs. "Servers" holds the media
 * servers and download clients that hear about every shrunk file, so the
 * library at home updates itself. "Webhooks" holds the outbound hooks that
 * tell something else what happened: a posted event summary, signed with
 * the hook's secret. "Plugins" holds the hook plugins (task 31), installed
 * from a git URL or a directory and enabled one by one. Rows are DataList
 * cards (task 20) with the shared 44px control classes; the add/edit form
 * keeps the key secret — a saved key shows "(unchanged)" in its field and
 * never travels back out of the API, and the webhook secret is shown once
 * with a copy button.
 */
import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createIntegration,
  createWebhook,
  deleteIntegration,
  deleteWebhook,
  getIntegrations,
  getWebhookDeliveries,
  getWebhooks,
  testIntegration,
  testWebhook,
  updateIntegration,
  updateWebhook,
  type Integration,
  type IntegrationBody,
  type Webhook,
  type WebhookBody,
  type WebhookEvent,
  WEBHOOK_EVENTS,
} from './api'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import ErrorBox from './ErrorBox'
import ModalDialog from './dialog'
import PluginsPage from './PluginsPage'
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

/** The form: blank for "add", prefilled for edit (secret stays blank). */
interface WebhookFormState {
  id: string
  name: string
  url: string
  events: WebhookEvent[]
  enabled: boolean
}

function blankWebhookForm(): WebhookFormState {
  return { id: '', name: '', url: '', events: ['job.done'], enabled: true }
}

function webhookFormOf(row: Webhook): WebhookFormState {
  return {
    id: row.id,
    name: row.name,
    url: row.url,
    events: [...row.events],
    enabled: row.enabled,
  }
}

/** The event label: plain language, no event names. */
function eventLabel(id: string): string {
  return WEBHOOK_EVENTS.find((e) => e.id === id)?.label ?? id
}

/** The tab: the three halves of the page. */
type Tab = 'servers' | 'webhooks' | 'plugins'

export default function IntegrationsPage() {
  // The tab also answers a ?tab= deep link (the flow palette's
  // "install a plugin" hint links to the plugins tab).
  const [search] = useSearchParams()
  const asked = search.get('tab') as Tab | null
  const [tab, setTab] = useState<Tab>(
    asked === 'webhooks' || asked === 'plugins' ? asked : 'servers',
  )

  return (
    <PageContainer className="space-y-6" data-testid="integrations-page">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">Integrations</h1>
        <div
          className="flex gap-2"
          role="tablist"
          aria-label="Integrations"
          data-testid="integrations-tabs"
        >
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'servers'}
            className={tab === 'servers' ? btnPrimary : btnGhost}
            data-testid="integrations-tab-servers"
            onClick={() => setTab('servers')}
          >
            Servers
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'webhooks'}
            className={tab === 'webhooks' ? btnPrimary : btnGhost}
            data-testid="integrations-tab-webhooks"
            onClick={() => setTab('webhooks')}
          >
            Webhooks
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'plugins'}
            className={tab === 'plugins' ? btnPrimary : btnGhost}
            data-testid="integrations-tab-plugins"
            onClick={() => setTab('plugins')}
          >
            Plugins
          </button>
        </div>
      </div>

      {tab === 'servers' ? (
        <ServersTab />
      ) : tab === 'webhooks' ? (
        <WebhooksTab />
      ) : (
        <PluginsPage />
      )}
    </PageContainer>
  )
}

/* ------------------------------------------------------------------ */
/* Servers tab (task 23 content)                                       */
/* ------------------------------------------------------------------ */

function ServersTab() {
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
  // form: an empty api_key keeps the stored key (the form shows
  // "(unchanged)" in its field and never travels back out of the API).
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
    onSuccess: () => {
      invalidate()
      setDeleting(null)
    },
    onError: (err: unknown) =>
      toast.failure('Deleting the integration failed', err),
  })

  const test = useMutation({
    mutationFn: (id: string) => testIntegration(id),
    onSuccess: (result, id) => {
      const name = list.data?.items.find((row) => row.id === id)?.name ?? ''
      setTestResult({
        id,
        text: result.ok
          ? `Reached ${name} — it is listening.`
          : result.error
            ? result.error
            : `${name} answered with status ${result.status}.`,
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
      onChange={(next) => toggle.mutate({ row, enabled: next })}
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
    <div className="space-y-6">
      <p className="max-w-2xl text-sm text-slate-400">
        After a file is shrunk, your media server or download client can be told
        to rescan — without anyone clicking Scan. Each entry here is one server
        to notify.
      </p>

      {testResult && (
        <p
          role="status"
          className="rounded-2xl bg-slate-800/60 p-4 text-sm ring-1 ring-slate-700"
          data-testid="integration-test-result"
        >
          {testResult.text}
        </p>
      )}

      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-semibold">Configured servers</h2>
        <button
          type="button"
          className={btnPrimary}
          data-testid="integrations-add"
          onClick={() => setForm(blankForm())}
        >
          Add server
        </button>
      </div>

      {list.isPending && <SkeletonRows rows={3} />}
      {list.isError && (
        <ErrorBox title="Cannot load the servers." error={list.error} />
      )}
      {list.data && list.data.items.length === 0 && (
        <p className="text-sm text-slate-500" data-testid="integrations-empty">
          No servers configured yet.
        </p>
      )}
      {list.data && list.data.items.length > 0 && (
        <DataList
          rows={list.data.items}
          columns={columns}
          testid="integrations-list"
          rowTestId={(r) => `integration-row-${r.id}`}
          cardRowTestId={(r) => `integration-card-${r.id}`}
        />
      )}

      {form && (
        <section
          className="rounded-2xl bg-slate-800/40 p-6"
          data-testid="integration-form"
        >
          <h3 className="text-lg font-semibold">
            {editing ? 'Edit server' : 'Add a server'}
          </h3>
          <div className="mt-4 space-y-4">
            <label className="block">
              <span className="text-sm text-slate-300">Kind</span>
              <select
                className={input}
                value={form.kind}
                data-testid="integration-kind"
                onChange={(e) =>
                  setForm({
                    ...form,
                    kind: e.target.value as Integration['kind'],
                  })
                }
              >
                {KINDS.map((k) => (
                  <option key={k.id} value={k.id}>
                    {k.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="block">
              <span className="text-sm text-slate-300">Name</span>
              <input
                className={input}
                value={form.name}
                data-testid="integration-name"
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="Living room Jellyfin"
              />
            </label>
            <label className="block">
              <span className="text-sm text-slate-300">Address (URL)</span>
              <input
                className={input}
                value={form.base_url}
                data-testid="integration-base-url"
                onChange={(e) => setForm({ ...form, base_url: e.target.value })}
                placeholder="http://192.168.1.10:8096"
              />
            </label>
            <label className="block">
              <span className="text-sm text-slate-300">API key</span>
              <input
                className={input}
                type="password"
                value={form.api_key}
                data-testid="integration-api-key"
                onChange={(e) => setForm({ ...form, api_key: e.target.value })}
                placeholder={editing ? '(unchanged)' : ''}
              />
            </label>
            <Switch
              checked={form.enabled}
              label="Enabled"
              testId="integration-enabled"
              onChange={(next) => setForm({ ...form, enabled: next })}
            />
            <Switch
              checked={form.refresh_on_done}
              label="Tell it to rescan when a file is shrunk"
              testId="integration-refresh-on-done"
              onChange={(next) => setForm({ ...form, refresh_on_done: next })}
            />
          </div>
          <div className="mt-6 flex gap-2">
            <button
              type="button"
              className={btnPrimary}
              disabled={save.isPending}
              data-testid="integration-save"
              onClick={submit}
            >
              Save
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
    </div>
  )
}

/* ------------------------------------------------------------------ */
/* Webhooks tab (task 25)                                              */
/* ------------------------------------------------------------------ */

/** One hook's deliveries dialog: the last 50 attempts, newest first. */
function DeliveriesDialog({
  webhook,
  onClose,
}: {
  webhook: Webhook
  onClose: () => void
}) {
  const list = useQuery({
    queryKey: ['webhook-deliveries', webhook.id],
    queryFn: () => getWebhookDeliveries(webhook.id),
  })

  return (
    <ModalDialog
      label="Deliveries"
      testId="webhook-deliveries-dialog"
      onClose={onClose}
      className="max-h-[80vh] w-full max-w-xl overflow-y-auto rounded-2xl bg-slate-800/40 p-6 shadow-xl"
    >
      <h3 className="text-lg font-semibold">Deliveries — {webhook.name}</h3>
      <p className="mt-1 text-sm text-slate-400">
        The last attempts, newest first. Status 0 means the request never got an
        answer.
      </p>
      {list.isPending && <SkeletonRows rows={2} />}
      {list.isError && (
        <ErrorBox title="Cannot load the deliveries." error={list.error} />
      )}
      {list.data && list.data.items.length === 0 && (
        <p
          className="mt-4 text-sm text-slate-500"
          data-testid="webhook-deliveries-empty"
        >
          Nothing delivered yet.
        </p>
      )}
      {list.data && list.data.items.length > 0 && (
        <table
          className="mt-4 w-full text-sm"
          data-testid="webhook-deliveries-table"
        >
          <thead>
            <tr className="text-left text-slate-400">
              <th className="py-1 pr-3 font-medium">When</th>
              <th className="py-1 pr-3 font-medium">Event</th>
              <th className="py-1 pr-3 font-medium">Status</th>
              <th className="py-1 font-medium">Error</th>
            </tr>
          </thead>
          <tbody>
            {list.data.items.map((d) => (
              <tr key={d.id} data-testid={`webhook-delivery-${d.id}`}>
                <td className="py-1 pr-3" data-testid={`delivery-at-${d.id}`}>
                  {new Date(d.at).toLocaleString()}
                </td>
                <td
                  className="py-1 pr-3"
                  data-testid={`delivery-event-${d.id}`}
                >
                  {eventLabel(d.event)}
                </td>
                <td
                  className="py-1 pr-3"
                  data-testid={`delivery-status-${d.id}`}
                >
                  {d.status === 0 ? 'No answer' : d.status}
                </td>
                <td
                  className="py-1 pr-3"
                  data-testid={`delivery-error-${d.id}`}
                >
                  {d.error ?? ''}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="mt-6">
        <button
          type="button"
          className={btnGhost}
          data-testid="webhook-deliveries-close"
          onClick={onClose}
        >
          Close
        </button>
      </div>
    </ModalDialog>
  )
}

function WebhooksTab() {
  const client = useQueryClient()
  const toast = useToast()
  const list = useQuery({
    queryKey: ['webhooks'],
    queryFn: getWebhooks,
  })

  const [form, setForm] = useState<WebhookFormState | null>(null)
  const [deleting, setDeleting] = useState<Webhook | null>(null)
  const [deliveries, setDeliveries] = useState<Webhook | null>(null)
  const [testResult, setTestResult] = useState<{
    id: string
    text: string
  } | null>(null)
  /** The secret, shown once, right after create. */
  const [secret, setSecret] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ['webhooks'] })
  }

  const save = useMutation({
    mutationFn: (input: { id: string; body: WebhookBody }) =>
      input.id
        ? updateWebhook(input.id, input.body)
        : createWebhook(input.body),
    onSuccess: (created, input) => {
      if (input.id === '' && 'secret' in created) {
        // A new hook: the secret is on screen exactly once.
        setSecret(String(created.secret))
      }
      toast.success('Webhook saved.')
      invalidate()
      setForm(null)
    },
    onError: (err: unknown) => toast.failure('Saving the webhook failed', err),
  })

  const toggle = useMutation({
    mutationFn: (input: { row: Webhook; enabled: boolean }) =>
      updateWebhook(input.row.id, {
        name: input.row.name,
        url: input.row.url,
        events: input.row.events,
        enabled: input.enabled,
      }),
    onSuccess: () => invalidate(),
    onError: (err: unknown) => toast.failure('Saving the webhook failed', err),
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteWebhook(id),
    onSuccess: () => {
      invalidate()
      setDeleting(null)
    },
    onError: (err: unknown) =>
      toast.failure('Deleting the webhook failed', err),
  })

  const test = useMutation({
    mutationFn: (id: string) => testWebhook(id),
    onSuccess: (result, id) => {
      const name = list.data?.items.find((row) => row.id === id)?.name ?? ''
      setTestResult({
        id,
        text: result.ok
          ? `Delivered the sample to ${name}.`
          : result.error
            ? result.error
            : `${name} answered with status ${result.status}.`,
      })
    },
    onError: (err: unknown) => toast.failure('Testing the webhook failed', err),
  })

  const editing = form !== null && form.id !== ''

  const submit = () => {
    if (!form) return
    save.mutate({
      id: form.id,
      body: {
        name: form.name,
        url: form.url,
        events: form.events,
        enabled: form.enabled,
      },
    })
  }

  // Row controls take an onCard flag: the card tree's test ids carry a
  // -card suffix (the task 20 pattern), so tests target one tree.
  const rowSwitch = (row: Webhook, onCard: boolean) => (
    <Switch
      checked={row.enabled}
      label={`${row.name} on/off`}
      testId={`webhook-switch-${row.id}${onCard ? '-card' : ''}`}
      onChange={(next) => toggle.mutate({ row, enabled: next })}
    />
  )

  const rowActions = (row: Webhook, onCard: boolean) => (
    <div className="flex flex-wrap gap-2">
      <button
        type="button"
        className={btn}
        disabled={test.isPending}
        data-testid={`webhook-test-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => test.mutate(row.id)}
      >
        Test
      </button>
      <button
        type="button"
        className={btnGhost}
        data-testid={`webhook-deliveries-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setDeliveries(row)}
      >
        Deliveries
      </button>
      <button
        type="button"
        className={btnGhost}
        data-testid={`webhook-edit-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setForm(webhookFormOf(row))}
      >
        Edit
      </button>
      <button
        type="button"
        className={btnGhost}
        data-testid={`webhook-delete-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setDeleting(row)}
      >
        Delete
      </button>
    </div>
  )

  const columns: Column<Webhook>[] = [
    { key: 'name', header: 'Name', cell: (r) => r.name, primary: true },
    {
      key: 'url',
      header: 'URL',
      cell: (r) => <span className="break-all text-slate-300">{r.url}</span>,
    },
    {
      key: 'events',
      header: 'Events',
      cell: (r) => r.events.map((e) => eventLabel(e)).join(', '),
    },
    {
      key: 'enabled',
      header: 'On',
      cell: (r) => rowSwitch(r, false),
      cardCell: (r) => rowSwitch(r, true),
    },
    {
      key: 'actions',
      header: 'Actions',
      cell: (r) => rowActions(r, false),
      cardCell: (r) => rowActions(r, true),
    },
  ]

  const toggleEvent = (event: WebhookEvent) => {
    if (!form) return
    const next = form.events.includes(event)
      ? form.events.filter((e) => e !== event)
      : [...form.events, event]
    setForm({ ...form, events: next })
  }

  const copySecret = () => {
    if (!secret) return
    void navigator.clipboard?.writeText(secret).then(
      () => setCopied(true),
      () => setCopied(false),
    )
  }

  return (
    <div className="space-y-6">
      <p className="max-w-2xl text-sm text-slate-400">
        A webhook is a place outside this server that wants to hear what
        happened: the URL receives a one-line message every time the chosen
        thing occurs. Discord and Slack work out of the box.
      </p>

      {testResult && (
        <p
          role="status"
          className="rounded-2xl bg-slate-800/60 p-4 text-sm ring-1 ring-slate-700"
          data-testid="webhook-test-result"
        >
          {testResult.text}
        </p>
      )}

      {secret && (
        <section
          className="rounded-2xl bg-emerald-950/40 p-6 ring-1 ring-emerald-800"
          data-testid="webhook-secret-box"
        >
          <h3 className="text-lg font-semibold">Save this secret now</h3>
          <p className="mt-1 text-sm text-emerald-200/80">
            Every message is signed with it. You will not see this again — the
            server keeps it and never shows it again.
          </p>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <code
              className="break-all rounded bg-slate-900 px-3 py-2 text-xs text-emerald-200"
              data-testid="webhook-secret-value"
            >
              {secret}
            </code>
            <button
              type="button"
              className={btn}
              data-testid="webhook-secret-copy"
              onClick={copySecret}
            >
              {copied ? 'Copied' : 'Copy'}
            </button>
          </div>
        </section>
      )}

      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-semibold">Configured webhooks</h2>
        <button
          type="button"
          className={btnPrimary}
          data-testid="webhooks-add"
          onClick={() => setForm(blankWebhookForm())}
        >
          Add webhook
        </button>
      </div>

      {list.isPending && <SkeletonRows rows={3} />}
      {list.isError && (
        <ErrorBox title="Cannot load the webhooks." error={list.error} />
      )}
      {list.data && list.data.items.length === 0 && (
        <p className="text-sm text-slate-500" data-testid="webhooks-empty">
          No webhooks configured yet.
        </p>
      )}
      {list.data && list.data.items.length > 0 && (
        <DataList
          rows={list.data.items}
          columns={columns}
          testid="webhooks-list"
          rowTestId={(r) => `webhook-row-${r.id}`}
          cardRowTestId={(r) => `webhook-card-${r.id}`}
        />
      )}

      {form && (
        <section
          className="rounded-2xl bg-slate-800/40 p-6"
          data-testid="webhook-form"
        >
          <h3 className="text-lg font-semibold">
            {editing ? 'Edit webhook' : 'Add a webhook'}
          </h3>
          <div className="mt-4 space-y-4">
            <label className="block">
              <span className="text-sm text-slate-300">Name</span>
              <input
                className={input}
                value={form.name}
                data-testid="webhook-name"
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder="Discord #media"
              />
            </label>
            <label className="block">
              <span className="text-sm text-slate-300">URL</span>
              <input
                className={input}
                value={form.url}
                data-testid="webhook-url"
                onChange={(e) => setForm({ ...form, url: e.target.value })}
                placeholder="https://discord.com/api/webhooks/…"
              />
            </label>
            <fieldset className="space-y-2">
              <legend className="text-sm text-slate-300">Tell it about</legend>
              {WEBHOOK_EVENTS.map((e) => (
                <label key={e.id} className="flex items-center gap-3 text-sm">
                  <input
                    type="checkbox"
                    className="size-5 accent-emerald-600"
                    checked={form.events.includes(e.id)}
                    data-testid={`webhook-event-${e.id}`}
                    onChange={() => toggleEvent(e.id)}
                  />
                  {e.label}
                </label>
              ))}
            </fieldset>
            <Switch
              checked={form.enabled}
              label="Enabled"
              testId="webhook-enabled"
              onChange={(next) => setForm({ ...form, enabled: next })}
            />
          </div>
          <div className="mt-6 flex gap-2">
            <button
              type="button"
              className={btnPrimary}
              disabled={save.isPending}
              data-testid="webhook-save"
              onClick={submit}
            >
              Save
            </button>
            <button
              type="button"
              className={btnGhost}
              data-testid="webhook-cancel"
              onClick={() => setForm(null)}
            >
              Cancel
            </button>
          </div>
        </section>
      )}

      {deliveries && (
        <DeliveriesDialog
          webhook={deliveries}
          onClose={() => setDeliveries(null)}
        />
      )}

      {deleting && (
        <ConfirmDialog
          title="Delete this webhook?"
          confirmLabel="Delete"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(deleting.id)}
          onClose={() => setDeleting(null)}
          testId="webhook-delete-dialog"
        >
          <p>
            {deleting.name} stops hearing about anything, and its delivery
            history is removed.
          </p>
        </ConfirmDialog>
      )}
    </div>
  )
}
