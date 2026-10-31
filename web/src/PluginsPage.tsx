// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Plugins tab (task 31): the hook plugins installed from a git URL or a
 * directory. Each row shows the plugin's identity and a Switch that is
 * the only thing that makes it run — installed plugins are disabled
 * until enabled on purpose. The settings form is generated from the
 * plugin's manifest; Test runs it once against a synthetic job and shows
 * the output. The security statement is plain: a plugin runs as this
 * server's user, with the same access to the media folders, and there is
 * no sandbox.
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
import ConfirmDialog from './ConfirmDialog'
import DataList, { type Column } from './DataList'
import ErrorBox from './ErrorBox'
import ModalDialog from './dialog'
import { SkeletonRows } from './skeleton'
import Switch from './Switch'
import { useToast } from './toastContext'
import { btn, btnGhost, btnPrimary, input } from './uiControls'

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
    <div className="flex flex-wrap gap-2">
      <button
        type="button"
        className={btn}
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
        className={btnGhost}
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
          <span>{r.name}</span>
          {r.manifest_error && (
            <p
              className="mt-1 text-xs text-amber-400"
              data-testid={`plugin-manifest-error-${r.id}`}
            >
              The manifest could not be read: {r.manifest_error}
            </p>
          )}
        </div>
      ),
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
      cell: (r) => <span className="break-all text-slate-300">{r.source}</span>,
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
      cardNoLabel: true,
    },
  ]

  const submitInstall = () => {
    if (!install) return
    add.mutate(install)
  }

  return (
    <div className="space-y-6" data-testid="plugins-page">
      <p className="max-w-2xl text-sm text-slate-400">
        A plugin is a small program the server runs at a moment you choose —
        before encoding, after it, or when a job ends. Install one from a git
        repository or a folder. Nothing runs until you turn it on. A plugin runs
        as this server's user, with the same access to your media folders —
        install only plugins you trust; there is no sandbox (see{' '}
        <a
          className="underline hover:text-slate-200"
          href="https://github.com/PeiceOfToast/shrinkyourshit/blob/main/docs/plugins.md"
          target="_blank"
          rel="noreferrer"
          data-testid="plugins-docs-link"
        >
          the plugin docs
        </a>
        ).
      </p>

      {testResult && (
        <section
          className="rounded-2xl bg-slate-800/60 p-4 text-sm ring-1 ring-slate-700"
          data-testid="plugin-test-result"
        >
          <h3 className="font-semibold">Test run</h3>
          {testResult.result.skipped ? (
            <p className="mt-1 text-amber-300">
              The plugin did not run: {testResult.result.skip_reason}
            </p>
          ) : testResult.result.ok ? (
            <p className="mt-1 text-emerald-300">The plugin exited 0.</p>
          ) : (
            <p className="mt-1 text-red-300">
              The plugin failed: {testResult.result.error}
            </p>
          )}
          {(testResult.result.output?.length ?? 0) > 0 && (
            <pre
              className="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap break-all rounded-xl bg-black/40 p-3 font-mono text-xs text-slate-300"
              data-testid="plugin-test-output"
            >
              {testResult.result.output?.join('\n')}
            </pre>
          )}
        </section>
      )}

      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-semibold">Installed plugins</h2>
        <button
          type="button"
          className={btnPrimary}
          data-testid="plugins-add"
          onClick={() => setInstall(blankInstall())}
        >
          Install plugin
        </button>
      </div>

      {list.isPending && <SkeletonRows rows={2} />}
      {list.isError && (
        <ErrorBox title="Cannot load the plugins." error={list.error} />
      )}
      {list.data && list.data.items.length === 0 && (
        <p className="text-sm text-slate-500" data-testid="plugins-empty">
          No plugins installed yet.
        </p>
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

      {install && (
        <section
          className="rounded-2xl bg-slate-800/40 p-6"
          data-testid="plugin-install-form"
        >
          <h3 className="text-lg font-semibold">Install a plugin</h3>
          <div className="mt-4 space-y-4">
            <div
              className="flex gap-2"
              role="tablist"
              aria-label="Install source"
              data-testid="plugin-install-mode"
            >
              <button
                type="button"
                role="tab"
                aria-selected={install.mode === 'git'}
                className={install.mode === 'git' ? btnPrimary : btnGhost}
                data-testid="plugin-install-mode-git"
                onClick={() => setInstall({ ...install, mode: 'git' })}
              >
                From a git URL
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={install.mode === 'path'}
                className={install.mode === 'path' ? btnPrimary : btnGhost}
                data-testid="plugin-install-mode-path"
                onClick={() => setInstall({ ...install, mode: 'path' })}
              >
                From a folder
              </button>
            </div>
            {install.mode === 'git' ? (
              <>
                <label className="block">
                  <span className="text-sm text-slate-300">Repository URL</span>
                  <input
                    className={input}
                    value={install.git_url}
                    data-testid="plugin-git-url"
                    onChange={(e) =>
                      setInstall({ ...install, git_url: e.target.value })
                    }
                    placeholder="https://github.com/you/your-plugin"
                  />
                </label>
                <label className="block">
                  <span className="text-sm text-slate-300">
                    Branch or tag (optional)
                  </span>
                  <input
                    className={input}
                    value={install.ref}
                    data-testid="plugin-git-ref"
                    onChange={(e) =>
                      setInstall({ ...install, ref: e.target.value })
                    }
                    placeholder="main"
                  />
                </label>
              </>
            ) : (
              <label className="block">
                <span className="text-sm text-slate-300">
                  Folder the server can see
                </span>
                <input
                  className={input}
                  value={install.path}
                  data-testid="plugin-path"
                  onChange={(e) =>
                    setInstall({ ...install, path: e.target.value })
                  }
                  placeholder="/path/to/your-plugin"
                />
              </label>
            )}
            <p className="text-xs text-slate-500">
              The plugin is installed off. Turn it on after you have filled in
              its settings.
            </p>
          </div>
          <div className="mt-6 flex gap-2">
            <button
              type="button"
              className={btnPrimary}
              disabled={add.isPending}
              data-testid="plugin-install-submit"
              onClick={submitInstall}
            >
              Install
            </button>
            <button
              type="button"
              className={btnGhost}
              data-testid="plugin-install-cancel"
              onClick={() => setInstall(null)}
            >
              Cancel
            </button>
          </div>
        </section>
      )}

      {settings && (
        <ModalDialog
          label={`Settings for ${settings.name}`}
          testId="plugin-settings-dialog"
          onClose={() => setSettings(null)}
          className="max-h-[80vh] w-full max-w-xl overflow-y-auto rounded-2xl bg-slate-800/40 p-6 shadow-xl"
        >
          <h3 className="text-lg font-semibold">Settings — {settings.name}</h3>
          <p className="mt-1 text-sm text-slate-400">
            The values the plugin receives when it runs.
          </p>
          {(settings.settings?.length ?? 0) === 0 ? (
            <p
              className="mt-4 text-sm text-slate-500"
              data-testid="plugin-settings-empty"
            >
              This plugin takes no settings.
            </p>
          ) : (
            <PluginSettingsForm
              plugin={settings}
              busy={saveSettings.isPending}
              onSave={(values) =>
                saveSettings.mutate({ row: settings, values })
              }
            />
          )}
          <div className="mt-6">
            <button
              type="button"
              className={btnGhost}
              data-testid="plugin-settings-close"
              onClick={() => setSettings(null)}
            >
              Close
            </button>
          </div>
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
}: {
  plugin: Plugin
  busy: boolean
  onSave: (values: Record<string, string>) => void
}) {
  const [values, setValues] = useState<Record<string, string>>(() => {
    const out: Record<string, string> = {}
    for (const s of plugin.settings ?? []) {
      out[s.key] = plugin.values?.[s.key] ?? ''
    }
    return out
  })

  return (
    <div className="mt-4 space-y-4" data-testid="plugin-settings-form">
      {(plugin.settings ?? []).map((s) => (
        <label key={s.key} className="block">
          <span className="text-sm text-slate-300">
            {s.label}
            {s.required ? <span className="text-red-400"> *</span> : null}
          </span>
          <input
            className={input}
            value={values[s.key] ?? ''}
            required={s.required}
            data-testid={`plugin-setting-${s.key}`}
            onChange={(e) => setValues({ ...values, [s.key]: e.target.value })}
          />
        </label>
      ))}
      <button
        type="button"
        className={btnPrimary}
        disabled={busy}
        data-testid="plugin-settings-save"
        onClick={() => onSave(values)}
      >
        Save settings
      </button>
    </div>
  )
}
