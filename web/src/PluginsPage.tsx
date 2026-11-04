// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Plugins tab (task 31, restyled task 50, spec 5.12): the hook plugins
 * installed from a git URL or a directory. Each row shows the plugin's
 * identity, a state Badge and a Switch that is the only thing that makes it
 * run — installed plugins are disabled until enabled on purpose. The settings
 * form is generated from the plugin's manifest; Test runs it once against a
 * synthetic job and shows the output. The security statement is plain: a
 * plugin runs as this server's user, with the same access to the media
 * folders, and there is no sandbox.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  deletePlugin,
  getPlugins,
  installPlugin,
  testPlugin,
  updatePlugin,
  type Plugin,
  type PluginTestResult,
  type PluginType,
} from './api'
import Badge from './Badge'
import Banner from './Banner'
import Card from './Card'
import CodeBlock from './CodeBlock'
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import EmptyState from './EmptyState'
import ErrorBox from './ErrorBox'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'
import { Field } from './FormSection'
import Switch from './Switch'
import Tabs from './Tabs'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { btnDangerGhost, btnGhost, btnPrimary, input, link } from './uiControls'

/** The plugin's type: the moment it runs. */
const PLUGIN_TYPES: { id: PluginType; label: string }[] = [
  { id: 'preflight', label: 'Before encoding' },
  { id: 'postprocess', label: 'After encoding' },
  { id: 'notify', label: 'When a job ends' },
]

function typeLabel(type: PluginType): string {
  return PLUGIN_TYPES.find((t) => t.id === type)?.label ?? type
}

/** The install form: a git URL plus an optional ref, or a local path. */
interface InstallForm {
  mode: 'git' | 'path'
  git_url: string
  ref: string
  path: string
}

function blankInstall(): InstallForm {
  return { mode: 'git', git_url: '', ref: '', path: '' }
}

/** The state Badge: a broken manifest first, then on or off. */
function pluginBadge(row: Plugin) {
  if (row.manifest_error)
    return (
      <Badge tone="danger" dot>
        Manifest error
      </Badge>
    )
  if (row.enabled)
    return (
      <Badge tone="success" dot>
        Enabled
      </Badge>
    )
  return <Badge>Disabled</Badge>
}

export default function PluginsPage() {
  return <PluginsTab />
}

/** The tab content; the Integrations page renders it as its third tab. */
export function PluginsTab() {
  const client = useQueryClient()
  const toast = useToast()
  const list = useQuery({
    queryKey: ['plugins'],
    queryFn: getPlugins,
  })

  const [install, setInstall] = useState<InstallForm | null>(null)
  const [settings, setSettings] = useState<Plugin | null>(null)
  const [deleting, setDeleting] = useState<Plugin | null>(null)
  const [testResult, setTestResult] = useState<{
    id: string
    result: PluginTestResult
  } | null>(null)

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ['plugins'] })
  }

  const add = useMutation({
    mutationFn: (form: InstallForm) =>
      installPlugin(
        form.mode === 'git'
          ? { git_url: form.git_url, ref: form.ref }
          : { path: form.path },
      ),
    onSuccess: (created) => {
      toast.success(`${created.name} installed. It is off until you enable it.`)
      invalidate()
      setInstall(null)
    },
    onError: (err: unknown) =>
      toast.failure('Installing the plugin failed', err),
  })

  const toggle = useMutation({
    mutationFn: (input: { row: Plugin; enabled: boolean }) =>
      updatePlugin(input.row.id, { enabled: input.enabled }),
    onSuccess: () => invalidate(),
    onError: (err: unknown) => toast.failure('Saving the plugin failed', err),
  })

  const saveSettings = useMutation({
    mutationFn: (input: { row: Plugin; values: Record<string, string> }) =>
      updatePlugin(input.row.id, { settings: input.values }),
    onSuccess: () => {
      toast.success('Settings saved.')
      invalidate()
      setSettings(null)
    },
    onError: (err: unknown) => toast.failure('Saving the settings failed', err),
  })

  const remove = useMutation({
    mutationFn: (id: string) => deletePlugin(id),
    onSuccess: () => {
      toast.success('Plugin removed.')
      invalidate()
      setDeleting(null)
    },
    onError: (err: unknown) => toast.failure('Removing the plugin failed', err),
  })

  const test = useMutation({
    mutationFn: (id: string) => testPlugin(id),
    onSuccess: (result, id) => setTestResult({ id, result }),
    onError: (err: unknown) => toast.failure('Testing the plugin failed', err),
  })

  // Row controls take an onCard flag: the card tree's test ids carry a
  // -card suffix (the task 20 pattern), so tests target one tree.
  const rowSwitch = (row: Plugin, onCard: boolean) => (
    <Switch
      checked={row.enabled}
      label={`${row.name} on/off`}
      testId={`plugin-switch-${row.id}${onCard ? '-card' : ''}`}
      onChange={(next) => toggle.mutate({ row, enabled: next })}
    />
  )

  const rowActions = (row: Plugin, onCard: boolean) => (
    <div className="flex justify-end gap-2">
      <button
        type="button"
        className={btnGhost}
        disabled={test.isPending}
        data-testid={`plugin-test-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => test.mutate(row.id)}
      >
        Test
      </button>
      <button
        type="button"
        className={btnGhost}
        data-testid={`plugin-settings-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setSettings(row)}
      >
        Settings
      </button>
      <button
        type="button"
        className={btnDangerGhost}
        data-testid={`plugin-delete-${row.id}${onCard ? '-card' : ''}`}
        onClick={() => setDeleting(row)}
      >
        Remove
      </button>
    </div>
  )

  const columns: Column<Plugin>[] = [
    {
      key: 'name',
      header: 'Name',
      primary: true,
      cell: (r) => (
        <div className="min-w-0">
          <p className="font-semibold text-fg">{r.name}</p>
          {r.manifest_error && (
            <p
              className="mt-1 text-caption font-normal text-warning"
              data-testid={`plugin-manifest-error-${r.id}`}
            >
              The manifest could not be read: {r.manifest_error}
            </p>
          )}
        </div>
      ),
    },
    {
      key: 'state',
      header: 'Status',
      cell: (r) => pluginBadge(r),
      cardPlacement: 'badge',
    },
    { key: 'version', header: 'Version', cell: (r) => r.version },
    {
      key: 'type',
      header: 'Runs',
      cell: (r) => typeLabel(r.type),
    },
    {
      key: 'source',
      header: 'From',
      cell: (r) => (
        <span className="block max-w-xs truncate font-mono text-caption text-fg-muted">
          {r.source}
        </span>
      ),
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

  const submitInstall = () => {
    if (!install) return
    add.mutate(install)
  }

  return (
    <div className="flex flex-col gap-6" data-testid="plugins-page">
      {testResult && (
        <Banner
          tone={
            testResult.result.skipped
              ? 'warning'
              : testResult.result.ok
                ? 'success'
                : 'danger'
          }
          title="Test run"
          testId="plugin-test-result"
        >
          <div className="flex flex-col gap-2">
            {testResult.result.skipped ? (
              <p>The plugin did not run: {testResult.result.skip_reason}</p>
            ) : testResult.result.ok ? (
              <p>The plugin exited 0.</p>
            ) : (
              <p>The plugin failed: {testResult.result.error}</p>
            )}
            {(testResult.result.output?.length ?? 0) > 0 && (
              <CodeBlock testId="plugin-test-output">
                {testResult.result.output?.join('\n') ?? ''}
              </CodeBlock>
            )}
          </div>
        </Banner>
      )}

      <Card
        as="section"
        padding="none"
        title="Plugins"
        description={
          <>
            A plugin is a small program the server runs at a moment you choose —
            before encoding, after it, or when a job ends. Install one from a
            git repository or a folder. Nothing runs until you turn it on. A
            plugin runs as this server&apos;s user, with the same access to your
            media folders — install only plugins you trust; there is no sandbox
            (see{' '}
            <a
              className={link}
              href="https://github.com/PeiceOfToast/shrinkyourshit/blob/main/docs/plugins.md"
              target="_blank"
              rel="noreferrer"
              data-testid="plugins-docs-link"
            >
              the plugin docs
            </a>
            ).
          </>
        }
        actions={
          <button
            type="button"
            className={btnPrimary}
            data-testid="plugins-add"
            onClick={() => setInstall(blankInstall())}
          >
            Install plugin
          </button>
        }
      >
        {list.isPending && <SkeletonRows rows={2} />}
        {list.isError && (
          <div className="p-4 sm:p-6">
            <ErrorBox
              title="Cannot load the plugins."
              error={list.error}
              onRetry={() => void list.refetch()}
            />
          </div>
        )}
        {list.data && list.data.items.length === 0 && (
          <EmptyState
            icon="integrations"
            title="No plugins yet"
            body="A plugin adds a step that runs before encoding, after it, or when a job ends. Install one from a git repository or a folder."
            action={
              <button
                type="button"
                className={btnPrimary}
                onClick={() => setInstall(blankInstall())}
              >
                Install plugin
              </button>
            }
            testId="plugins-empty"
          />
        )}
        {list.data && list.data.items.length > 0 && (
          <DataList
            rows={list.data.items}
            columns={columns}
            testid="plugins-list"
            rowTestId={(r) => `plugin-row-${r.id}`}
            cardRowTestId={(r) => `plugin-card-${r.id}`}
          />
        )}
      </Card>

      {install && (
        <ModalDialog
          label="Install a plugin"
          testId="plugin-install-form"
          size="md"
          onClose={() => setInstall(null)}
        >
          <DialogHeader
            title="Install a plugin"
            onClose={() => setInstall(null)}
          />
          <DialogBody>
            <div className="flex flex-col gap-4">
              <div data-testid="plugin-install-mode">
                <Tabs
                  label="Install source"
                  tabs={[
                    {
                      id: 'git',
                      label: 'From a git URL',
                      testId: 'plugin-install-mode-git',
                    },
                    {
                      id: 'path',
                      label: 'From a folder',
                      testId: 'plugin-install-mode-path',
                    },
                  ]}
                  value={install.mode}
                  onChange={(id) =>
                    setInstall({ ...install, mode: id as 'git' | 'path' })
                  }
                />
              </div>
              {install.mode === 'git' ? (
                <>
                  <Field label="Repository URL" htmlFor="plugin-git-url">
                    <input
                      id="plugin-git-url"
                      className={input}
                      value={install.git_url}
                      data-testid="plugin-git-url"
                      onChange={(e) =>
                        setInstall({ ...install, git_url: e.target.value })
                      }
                      placeholder="https://github.com/you/your-plugin"
                    />
                  </Field>
                  <Field
                    label="Branch or tag"
                    optional
                    htmlFor="plugin-git-ref"
                  >
                    <input
                      id="plugin-git-ref"
                      className={input}
                      value={install.ref}
                      data-testid="plugin-git-ref"
                      onChange={(e) =>
                        setInstall({ ...install, ref: e.target.value })
                      }
                      placeholder="main"
                    />
                  </Field>
                </>
              ) : (
                <Field label="Folder the server can see" htmlFor="plugin-path">
                  <input
                    id="plugin-path"
                    className={input}
                    value={install.path}
                    data-testid="plugin-path"
                    onChange={(e) =>
                      setInstall({ ...install, path: e.target.value })
                    }
                    placeholder="/path/to/your-plugin"
                  />
                </Field>
              )}
              <p className="text-caption text-fg-muted">
                The plugin is installed off. Turn it on after you have filled in
                its settings.
              </p>
            </div>
          </DialogBody>
          <DialogFooter>
            <button
              type="button"
              className={btnGhost}
              data-testid="plugin-install-cancel"
              onClick={() => setInstall(null)}
            >
              Cancel
            </button>
            <button
              type="button"
              className={btnPrimary}
              disabled={add.isPending}
              data-testid="plugin-install-submit"
              onClick={submitInstall}
            >
              {add.isPending ? 'Installing…' : 'Install'}
            </button>
          </DialogFooter>
        </ModalDialog>
      )}

      {settings && (
        <ModalDialog
          label={`Settings for ${settings.name}`}
          testId="plugin-settings-dialog"
          size="md"
          onClose={() => setSettings(null)}
        >
          <DialogHeader
            title={`Settings — ${settings.name}`}
            onClose={() => setSettings(null)}
          />
          {(settings.settings?.length ?? 0) === 0 ? (
            <>
              <DialogBody>
                <p
                  className="text-body text-fg-muted"
                  data-testid="plugin-settings-empty"
                >
                  This plugin takes no settings.
                </p>
              </DialogBody>
              <DialogFooter>
                <button
                  type="button"
                  className={btnGhost}
                  data-testid="plugin-settings-close"
                  onClick={() => setSettings(null)}
                >
                  Close
                </button>
              </DialogFooter>
            </>
          ) : (
            <PluginSettingsForm
              plugin={settings}
              busy={saveSettings.isPending}
              onClose={() => setSettings(null)}
              onSave={(values) =>
                saveSettings.mutate({ row: settings, values })
              }
            />
          )}
        </ModalDialog>
      )}

      {deleting && (
        <ConfirmDialog
          title="Remove this plugin?"
          confirmLabel="Remove"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(deleting.id)}
          onClose={() => setDeleting(null)}
          testId="plugin-delete-dialog"
        >
          <p>
            {deleting.name} and its folder are removed. A plugin that is still
            on has to be turned off first.
          </p>
        </ConfirmDialog>
      )}
    </div>
  )
}

/** The settings form generated from the manifest's settings list. */
function PluginSettingsForm({
  plugin,
  busy,
  onSave,
  onClose,
}: {
  plugin: Plugin
  busy: boolean
  onSave: (values: Record<string, string>) => void
  onClose: () => void
}) {
  const [values, setValues] = useState<Record<string, string>>(() => {
    const out: Record<string, string> = {}
    for (const s of plugin.settings ?? []) {
      out[s.key] = plugin.values?.[s.key] ?? ''
    }
    return out
  })

  return (
    <>
      <DialogBody>
        <div className="flex flex-col gap-4" data-testid="plugin-settings-form">
          {(plugin.settings ?? []).map((s) => (
            <Field
              key={s.key}
              htmlFor={`plugin-setting-${s.key}`}
              label={
                <>
                  {s.label}
                  {s.required ? <span className="text-danger"> *</span> : null}
                </>
              }
            >
              <input
                id={`plugin-setting-${s.key}`}
                className={input}
                value={values[s.key] ?? ''}
                required={s.required}
                data-testid={`plugin-setting-${s.key}`}
                onChange={(e) =>
                  setValues({ ...values, [s.key]: e.target.value })
                }
              />
            </Field>
          ))}
        </div>
      </DialogBody>
      <DialogFooter>
        <button
          type="button"
          className={btnGhost}
          data-testid="plugin-settings-close"
          onClick={onClose}
        >
          Close
        </button>
        <button
          type="button"
          className={btnPrimary}
          disabled={busy}
          data-testid="plugin-settings-save"
          onClick={() => onSave(values)}
        >
          {busy ? 'Saving…' : 'Save settings'}
        </button>
      </DialogFooter>
    </>
  )
}
