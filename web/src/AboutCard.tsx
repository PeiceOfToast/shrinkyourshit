// SPDX-License-Identifier: AGPL-3.0-only

/**
 * About card (task 17): what this install is, whether a newer version
 * exists, and the action that matches how the app runs. A container gets
 * the exact host commands in a copyable block; a binary gets an update
 * button that restarts the app when it finishes. Automatic checks are on
 * by default and nothing applies an update without an explicit click. A
 * failed check is a quiet note in this card — never a toast, never a
 * warning anywhere else.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  applyUpdate,
  checkForUpdate,
  getSettings,
  getVersion,
  updateSettings,
} from './api'
import { updateStatusKey, useUpdateStatus } from './queries'
import { useToast } from './toastContext'
import {
  checkFailureMessage,
  formatPublished,
  runtimeLabel,
  stageLabel,
} from './updateModel'

function Row({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex justify-between gap-4 py-1 text-sm">
      <span className="text-slate-400">{label}</span>
      <span className="truncate text-slate-200">{children}</span>
    </div>
  )
}

export default function AboutCard() {
  const client = useQueryClient()
  const toast = useToast()
  const version = useQuery({ queryKey: ['version'], queryFn: getVersion })
  const status = useUpdateStatus()
  const settings = useQuery({ queryKey: ['settings'], queryFn: getSettings })

  const check = useMutation({
    mutationFn: checkForUpdate,
    // The fresh status lands in the cache; the banner and the note follow.
    onSuccess: (fresh) => client.setQueryData(updateStatusKey, fresh),
    // Quiet by design: check failures render in this card, nothing else.
  })

  const apply = useMutation({
    mutationFn: applyUpdate,
    // The server streams progress as update.* events, which invalidate
    // the status query; refetching starts right away anyway.
    onSuccess: () => client.invalidateQueries({ queryKey: updateStatusKey }),
  })

  const saveSettings = useMutation({
    mutationFn: updateSettings,
    onSuccess: (updated) => {
      client.setQueryData(['settings'], updated)
      toast.success('Settings saved.')
    },
    onError: (err: unknown) =>
      toast.failure('Saving the update settings failed', err),
  })

  const info = version.data
  const update = status.data
  const isContainer = info?.runtime === 'container'
  const applying = Boolean(update?.progress?.active)
  const channel = settings.data?.update_channel ?? 'stable'

  return (
    <section
      className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800"
      data-testid="about-card"
    >
      <h2 className="font-semibold">About</h2>

      <div className="mt-3">
        <Row label="Version">
          <span data-testid="about-version">{info?.version ?? '…'}</span>
        </Row>
        <Row label="Commit">
          <span data-testid="about-commit">{info?.commit ?? '…'}</span>
        </Row>
        <Row label="Build date">
          <span data-testid="about-date">{info?.date ?? '…'}</span>
        </Row>
        <Row label="Install type">
          <span data-testid="about-runtime">
            {info ? runtimeLabel(info.runtime) : '…'}
          </span>
        </Row>
      </div>

      <div className="mt-4 flex items-center gap-3">
        <button
          type="button"
          onClick={() => check.mutate()}
          disabled={check.isPending}
          className="rounded-lg bg-slate-800 px-4 py-2 text-sm font-semibold hover:bg-slate-700 disabled:opacity-50"
          data-testid="check-now"
        >
          {check.isPending ? 'Checking…' : 'Check now'}
        </button>
        <label className="flex select-none items-center gap-2 text-sm text-slate-300">
          <input
            type="checkbox"
            checked={settings.data?.update_check ?? true}
            onChange={(e) =>
              saveSettings.mutate({ update_check: e.target.checked })
            }
            className="h-4 w-4 rounded accent-emerald-500"
            data-testid="update-check-toggle"
          />
          Check for updates automatically
        </label>
      </div>

      {/* A failed check is a quiet note, never a toast (task 17). */}
      {check.isError && (
        <p className="mt-3 text-sm text-slate-400" data-testid="check-error">
          {checkFailureMessage(check.error)}
        </p>
      )}

      {update && update.update_available && update.latest && (
        <div
          className="mt-4 rounded-xl bg-sky-950/60 p-4 ring-1 ring-sky-800/60"
          data-testid="update-banner"
        >
          <p className="text-sm font-semibold text-sky-100">
            Version {update.latest} is available
            {update.published_at
              ? ` (published ${formatPublished(update.published_at)})`
              : ''}
            .
          </p>
          {update.release_url && (
            <a
              href={update.release_url}
              target="_blank"
              rel="noreferrer"
              className="mt-1 inline-block text-sm text-sky-300 underline"
              data-testid="release-notes-link"
            >
              Release notes
            </a>
          )}

          {/* The action matches the runtime (task 17). */}
          {isContainer ? (
            <ContainerInstructions reason={update.reason ?? ''} />
          ) : update.self_update_supported ? (
            <div className="mt-3">
              <button
                type="button"
                onClick={() => apply.mutate()}
                disabled={applying || apply.isPending}
                className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
                data-testid="apply-update"
              >
                {applying || apply.isPending ? 'Updating…' : 'Update now'}
              </button>
              <p
                className="mt-2 text-xs text-amber-200"
                data-testid="update-restart-warning"
              >
                The app restarts by itself when the update finishes. The
                database is backed up first.
              </p>
            </div>
          ) : (
            <p
              className="mt-2 text-sm text-slate-300"
              data-testid="update-unsupported"
            >
              {update.reason}
            </p>
          )}
        </div>
      )}

      {/* Live progress while an in-app update runs. */}
      {update?.progress?.active && (
        <p
          className="mt-3 text-sm text-slate-300"
          data-testid="update-progress"
        >
          {stageLabel(update.progress.stage)}
        </p>
      )}
      {update?.progress && !update.progress.active && update.progress.error && (
        <p className="mt-3 text-sm text-slate-400" data-testid="update-error">
          The update did not finish: {update.progress.error}
        </p>
      )}

      {/* Container installs never self update; the commands are the fix,
          shown whether or not a newer version is out. */}
      {isContainer && !update?.update_available && (
        <ContainerInstructions reason={status.data?.reason ?? ''} />
      )}

      {!update?.update_available && update?.reason && !isContainer && (
        <p className="mt-3 text-sm text-slate-400" data-testid="update-note">
          {update.reason}
        </p>
      )}

      <div className="mt-4 border-t border-slate-800 pt-4">
        <label className="grid max-w-xs gap-1 text-sm">
          <span className="text-slate-300">Update channel</span>
          <select
            value={channel}
            onChange={(e) =>
              saveSettings.mutate({ update_channel: e.target.value })
            }
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
            data-testid="update-channel"
          >
            <option value="stable">Stable releases</option>
            <option value="prerelease">
              Prereleases (early test versions)
            </option>
          </select>
        </label>
      </div>
    </section>
  )
}

/** The exact host commands a container install runs to update. */
function ContainerInstructions({ reason }: { reason: string }) {
  return (
    <div className="mt-3">
      <pre
        className="overflow-x-auto rounded-lg bg-slate-950 p-3 text-xs text-slate-300"
        data-testid="container-instructions"
      >
        {reason}
      </pre>
      <p className="mt-1 text-xs text-slate-500">
        Run these on the machine that hosts the container.
      </p>
    </div>
  )
}
