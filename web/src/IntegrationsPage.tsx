// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Integrations page (task 23, 25 and 31, restyled task 50, spec 5.12): three
 * tabs under the page header. "Servers" holds the media servers and download
 * clients that hear about every shrunk file. "Webhooks" holds the outbound
 * hooks that tell something else what happened, signed with the hook's
 * secret. "Plugins" holds the hook plugins (see PluginsPage). Each tab is one
 * Card: the primary action sits in the Card header, the rows are a DataList
 * with a state Badge, and an empty tab gets an EmptyState.
 *
 * The add/edit forms, the deliveries list and the one-time secret are
 * dialogs. A saved API key never appears in the form: a blank key field
 * keeps the stored key, and the placeholder says so. The webhook secret is
 * shown once, with a copy button, in a dialog that says it will not show
 * again.
 */
import { useDocumentTitle } from './useDocumentTitle'
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
import Badge from './Badge'
import Banner from './Banner'
import Card from './Card'
import Checkbox from './Checkbox'
import CodeBlock from './CodeBlock'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import EmptyState from './EmptyState'
import ErrorBox from './ErrorBox'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'
import { Field } from './FormSection'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import PluginsPage from './PluginsPage'
import Switch from './Switch'
import Tabs, { type TabItem } from './Tabs'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import {
  btnDangerGhost,
  btnGhost,
  btnPrimary,
  input,
  select,
} from './uiControls'

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

/**
 * The result of the last Test press on a row, kept for this page visit only.
 * The API has no connection state, so "Connected" and "Not connected" only
 * appear after a Test in this session; an untested row says so.
 */
type Health = 'ok' | 'failed'

/** The state Badge for a server or hook row. */
function stateBadge(enabled: boolean, health: Health | undefined) {
  if (!enabled) return <Badge>Disabled</Badge>
  if (health === 'ok')
    return (
      <Badge tone="success" dot>
        Connected
      </Badge>
    )
  if (health === 'failed')
    return (
      <Badge tone="danger" dot>
        Not connected
      </Badge>
    )
  return <Badge>Not tested</Badge>
}

/** The tab: the three halves of the page. */
type Tab = 'servers' | 'webhooks' | 'plugins'

const TABS: TabItem[] = [
  { id: 'servers', label: 'Servers', testId: 'integrations-tab-servers' },
  { id: 'webhooks', label: 'Webhooks', testId: 'integrations-tab-webhooks' },
  { id: 'plugins', label: 'Plugins', testId: 'integrations-tab-plugins' },
]

export default function IntegrationsPage() {
  useDocumentTitle(['Integrations'])
  // The tab also answers a ?tab= deep link (the flow palette's
  // "install a plugin" hint links to the plugins tab).
  const [search] = useSearchParams()
  const asked = search.get('tab') as Tab | null
  const [tab, setTab] = useState<Tab>(
    asked === 'webhooks' || asked === 'plugins' ? asked : 'servers',
  )

  return (
    <PageContainer data-testid="integrations-page">
      <PageHeader
        title="Integrations"
        subtitle="Tell other apps when files change, and add new steps."
      />

      <div data-testid="integrations-tabs">
        <Tabs
          label="Integrations"
          tabs={TABS}
          value={tab}
          onChange={(id) => setTab(id as Tab)}
        />
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
  const [health, setHealth] = useState<Record<string, Health>>({})
  const [testResult, setTestResult] = useState<{
    id: string
    text: string
    ok: boolean
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
      setHealth((prev) => ({ ...prev, [id]: result.ok ? 'ok' : 'failed' }))
      setTestResult({
        id,
        ok: result.ok,
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
    <div className="flex justify-end gap-2">
      <button
        type="button"
        className={btnGhost}
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
        className={btnDangerGhost}
        data-testid={`integration-delete-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setDeleting(row)}
      >
        Remove
      </button>
    </div>
  )

  const columns: Column<Integration>[] = [
    {
      key: 'name',
      header: 'Name',
      primary: true,
      cell: (r) => (
        <div className="min-w-0">
          <p className="font-semibold text-fg">{r.name}</p>
          <p
            className="mt-0.5 truncate font-mono text-caption font-normal text-fg-muted"
            title={`${kindLabel(r.kind)} · ${r.base_url}`}
          >
            <span>{kindLabel(r.kind)}</span>
            <span aria-hidden="true"> · </span>
            <span>{r.base_url}</span>
          </p>
        </div>
      ),
    },
    {
      key: 'state',
      header: 'Status',
      cell: (r) => stateBadge(r.enabled, health[r.id]),
      cardPlacement: 'badge',
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
      actions: true,
    },
  ]

  return (
    <div className="flex flex-col gap-6">
      {testResult && (
        <Banner
          tone={testResult.ok ? 'success' : 'warning'}
          testId="integration-test-result"
        >
          {testResult.text}
        </Banner>
      )}

      <Card
        as="section"
        padding="none"
        title="Media servers"
        description="After a file is shrunk, your media server or download client can be told to rescan — without anyone clicking Scan. Each entry here is one server to notify."
        actions={
          <button
            type="button"
            className={btnPrimary}
            data-testid="integrations-add"
            onClick={() => setForm(blankForm())}
          >
            Add server
          </button>
        }
      >
        {list.isPending && <SkeletonRows rows={3} />}
        {list.isError && (
          <div className="p-4 sm:p-6">
            <ErrorBox
              title="Cannot load the servers."
              error={list.error}
              onRetry={() => void list.refetch()}
            />
          </div>
        )}
        {list.data && list.data.items.length === 0 && (
          <EmptyState
            icon="integrations"
            title="No media servers yet"
            body="Add Plex, Jellyfin or Emby and it rescans by itself after a file is shrunk."
            action={
              <button
                type="button"
                className={btnPrimary}
                onClick={() => setForm(blankForm())}
              >
                Add server
              </button>
            }
            testId="integrations-empty"
          />
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
      </Card>

      {form && (
        <ModalDialog
          label={editing ? 'Edit server' : 'Add a server'}
          testId="integration-form"
          size="md"
          onClose={() => setForm(null)}
        >
          <DialogHeader
            title={editing ? 'Edit server' : 'Add a server'}
            onClose={() => setForm(null)}
          />
          <DialogBody>
            <div className="flex flex-col gap-4">
              <Field label="Kind" htmlFor="integration-kind">
                <select
                  id="integration-kind"
                  className={select}
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
              </Field>
              <Field label="Name" htmlFor="integration-name">
                <input
                  id="integration-name"
                  className={input}
                  value={form.name}
                  data-testid="integration-name"
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="Living room Jellyfin"
                />
              </Field>
              <Field label="Address (URL)" htmlFor="integration-base-url">
                <input
                  id="integration-base-url"
                  className={input}
                  value={form.base_url}
                  data-testid="integration-base-url"
                  onChange={(e) =>
                    setForm({ ...form, base_url: e.target.value })
                  }
                  placeholder="http://192.168.1.10:8096"
                />
              </Field>
              <Field
                label="API key"
                htmlFor="integration-api-key"
                help={
                  editing
                    ? 'Leave this blank to keep the saved key.'
                    : undefined
                }
              >
                <input
                  id="integration-api-key"
                  className={input}
                  type="password"
                  value={form.api_key}
                  data-testid="integration-api-key"
                  onChange={(e) =>
                    setForm({ ...form, api_key: e.target.value })
                  }
                  placeholder={editing ? '(unchanged)' : ''}
                />
              </Field>
              <Switch
                checked={form.enabled}
                label="Enabled"
                visibleLabel="Enabled"
                testId="integration-enabled"
                onChange={(next) => setForm({ ...form, enabled: next })}
              />
              <Switch
                checked={form.refresh_on_done}
                label="Tell it to rescan when a file is shrunk"
                visibleLabel="Tell it to rescan when a file is shrunk"
                testId="integration-refresh-on-done"
                onChange={(next) => setForm({ ...form, refresh_on_done: next })}
              />
            </div>
          </DialogBody>
          <DialogFooter>
            <button
              type="button"
              className={btnGhost}
              data-testid="integration-cancel"
              onClick={() => setForm(null)}
            >
              Cancel
            </button>
            <button
              type="button"
              className={btnPrimary}
              disabled={save.isPending}
              data-testid="integration-save"
              onClick={submit}
            >
              {save.isPending
                ? 'Saving…'
                : editing
                  ? 'Save changes'
                  : 'Add server'}
            </button>
          </DialogFooter>
        </ModalDialog>
      )}

      {deleting && (
        <ConfirmDialog
          title="Remove this server?"
          danger
          confirmLabel="Remove"
          confirmTestId="integration-delete-confirm"
          cancelTestId="integration-delete-cancel"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(deleting.id)}
          onClose={() => setDeleting(null)}
        >
          <p>
            Remove “{deleting.name}”? The server stops telling it about finished
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
      size="lg"
      onClose={onClose}
    >
      <DialogHeader title={`Deliveries — ${webhook.name}`} onClose={onClose} />
      <DialogBody>
        <p className="text-body text-fg-muted">
          The last attempts, newest first. Status 0 means the request never got
          an answer.
        </p>
        <div className="mt-4">
          {list.isPending && <SkeletonRows rows={2} />}
          {list.isError && (
            <ErrorBox
              title="Cannot load the deliveries."
              error={list.error}
              onRetry={() => void list.refetch()}
            />
          )}
          {list.data && list.data.items.length === 0 && (
            <p
              className="text-body text-fg-muted"
              data-testid="webhook-deliveries-empty"
            >
              Nothing delivered yet.
            </p>
          )}
          {list.data && list.data.items.length > 0 && (
            <table
              className="w-full text-left text-body"
              data-testid="webhook-deliveries-table"
            >
              <thead>
                <tr className="text-caption font-medium uppercase tracking-wider text-fg-muted">
                  <th className="py-2 pr-3 font-medium">When</th>
                  <th className="py-2 pr-3 font-medium">Event</th>
                  <th className="py-2 pr-3 font-medium">Status</th>
                  <th className="py-2 font-medium">Error</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {list.data.items.map((d) => (
                  <tr key={d.id} data-testid={`webhook-delivery-${d.id}`}>
                    <td
                      className="py-2 pr-3"
                      data-testid={`delivery-at-${d.id}`}
                    >
                      {new Date(d.at).toLocaleString()}
                    </td>
                    <td
                      className="py-2 pr-3"
                      data-testid={`delivery-event-${d.id}`}
                    >
                      {eventLabel(d.event)}
                    </td>
                    <td
                      className="py-2 pr-3 tabular-nums"
                      data-testid={`delivery-status-${d.id}`}
                    >
                      {d.status === 0 ? 'No answer' : d.status}
                    </td>
                    <td
                      className="py-2 text-fg-muted"
                      data-testid={`delivery-error-${d.id}`}
                    >
                      {d.error ?? ''}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </DialogBody>
      <DialogFooter>
        <button
          type="button"
          className={btnGhost}
          data-testid="webhook-deliveries-close"
          onClick={onClose}
        >
          Close
        </button>
      </DialogFooter>
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
  const [health, setHealth] = useState<Record<string, Health>>({})
  const [testResult, setTestResult] = useState<{
    id: string
    text: string
    ok: boolean
  } | null>(null)
  /** The secret, shown once, right after create. */
  const [secret, setSecret] = useState<string | null>(null)

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
      setHealth((prev) => ({ ...prev, [id]: result.ok ? 'ok' : 'failed' }))
      setTestResult({
        id,
        ok: result.ok,
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
    <div className="flex justify-end gap-2">
      <button
        type="button"
        className={btnGhost}
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
        className={btnDangerGhost}
        data-testid={`webhook-delete-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setDeleting(row)}
      >
        Remove
      </button>
    </div>
  )

  const columns: Column<Webhook>[] = [
    {
      key: 'name',
      header: 'Name',
      primary: true,
      cell: (r) => <span className="font-semibold text-fg">{r.name}</span>,
    },
    {
      key: 'url',
      header: 'URL',
      cell: (r) => (
        <span className="break-all font-mono text-caption text-fg-muted">
          {r.url}
        </span>
      ),
    },
    {
      key: 'events',
      header: 'Events',
      cell: (r) => r.events.map((e) => eventLabel(e)).join(', '),
    },
    {
      key: 'state',
      header: 'Status',
      cell: (r) => stateBadge(r.enabled, health[r.id]),
      cardPlacement: 'badge',
    },
    {
      key: 'enabled',
      header: 'On',
      cell: (r) => rowSwitch(r, false),
      cardCell: (r) => rowSwitch(r, true),
    },
    {
      key: 'actions',
      header: '',
      cell: (r) => rowActions(r, false),
      cardCell: (r) => rowActions(r, true),
      actions: true,
    },
  ]

  const toggleEvent = (event: WebhookEvent) => {
    if (!form) return
    const next = form.events.includes(event)
      ? form.events.filter((e) => e !== event)
      : [...form.events, event]
    setForm({ ...form, events: next })
  }

  return (
    <div className="flex flex-col gap-6">
      {testResult && (
        <Banner
          tone={testResult.ok ? 'success' : 'warning'}
          testId="webhook-test-result"
        >
          {testResult.text}
        </Banner>
      )}

      <Card
        as="section"
        padding="none"
        title="Webhooks"
        description="A webhook is a place outside this server that wants to hear what happened: the URL receives a one-line message every time the chosen thing occurs. Discord and Slack work out of the box."
        actions={
          <button
            type="button"
            className={btnPrimary}
            data-testid="webhooks-add"
            onClick={() => setForm(blankWebhookForm())}
          >
            Add webhook
          </button>
        }
      >
        {list.isPending && <SkeletonRows rows={3} />}
        {list.isError && (
          <div className="p-4 sm:p-6">
            <ErrorBox
              title="Cannot load the webhooks."
              error={list.error}
              onRetry={() => void list.refetch()}
            />
          </div>
        )}
        {list.data && list.data.items.length === 0 && (
          <EmptyState
            icon="integrations"
            title="No webhooks yet"
            body="Add one and the events you choose are sent to it as they happen, one line each."
            action={
              <button
                type="button"
                className={btnPrimary}
                onClick={() => setForm(blankWebhookForm())}
              >
                Add webhook
              </button>
            }
            testId="webhooks-empty"
          />
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
      </Card>

      {secret && (
        <ModalDialog
          label="Save this secret now"
          testId="webhook-secret-box"
          size="md"
          onClose={() => setSecret(null)}
        >
          <DialogHeader
            title="Save this secret now"
            onClose={() => setSecret(null)}
          />
          <DialogBody>
            <div className="flex flex-col gap-4">
              <p className="text-body text-fg-muted">
                Every message is signed with it. The server keeps it and never
                shows it again.
              </p>
              <Banner tone="warning">You will not see this again.</Banner>
              <CodeBlock
                copy
                testId="webhook-secret-value"
                copyTestId="webhook-secret-copy"
              >
                {secret}
              </CodeBlock>
            </div>
          </DialogBody>
          <DialogFooter>
            <button
              type="button"
              className={btnPrimary}
              data-testid="webhook-secret-done"
              onClick={() => setSecret(null)}
            >
              Done
            </button>
          </DialogFooter>
        </ModalDialog>
      )}

      {form && (
        <ModalDialog
          label={editing ? 'Edit webhook' : 'Add a webhook'}
          testId="webhook-form"
          size="md"
          onClose={() => setForm(null)}
        >
          <DialogHeader
            title={editing ? 'Edit webhook' : 'Add a webhook'}
            onClose={() => setForm(null)}
          />
          <DialogBody>
            <div className="flex flex-col gap-4">
              <Field label="Name" htmlFor="webhook-name">
                <input
                  id="webhook-name"
                  className={input}
                  value={form.name}
                  data-testid="webhook-name"
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="Discord #media"
                />
              </Field>
              <Field label="URL" htmlFor="webhook-url">
                <input
                  id="webhook-url"
                  className={input}
                  value={form.url}
                  data-testid="webhook-url"
                  onChange={(e) => setForm({ ...form, url: e.target.value })}
                  placeholder="https://discord.com/api/webhooks/…"
                />
              </Field>
              <fieldset className="flex flex-col gap-1">
                <legend className="mb-1 text-body font-medium text-fg">
                  Tell it about
                </legend>
                {WEBHOOK_EVENTS.map((e) => (
                  <Checkbox
                    key={e.id}
                    checked={form.events.includes(e.id)}
                    label={e.label}
                    testId={`webhook-event-${e.id}`}
                    onChange={() => toggleEvent(e.id)}
                  />
                ))}
              </fieldset>
              <Switch
                checked={form.enabled}
                label="Enabled"
                visibleLabel="Enabled"
                testId="webhook-enabled"
                onChange={(next) => setForm({ ...form, enabled: next })}
              />
            </div>
          </DialogBody>
          <DialogFooter>
            <button
              type="button"
              className={btnGhost}
              data-testid="webhook-cancel"
              onClick={() => setForm(null)}
            >
              Cancel
            </button>
            <button
              type="button"
              className={btnPrimary}
              disabled={save.isPending}
              data-testid="webhook-save"
              onClick={submit}
            >
              {save.isPending
                ? 'Saving…'
                : editing
                  ? 'Save changes'
                  : 'Add webhook'}
            </button>
          </DialogFooter>
        </ModalDialog>
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
          confirmLabel="Remove"
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
