// SPDX-License-Identifier: AGPL-3.0-only

/**
 * About card (task 17, restyled task 49): what this install is, whether a
 * newer version exists, and the action that matches how the app runs. It is
 * the "About and updates" FormSection on App settings (spec 5.11). A
 * container gets the exact host commands in a copyable block; a binary gets
 * an update button that restarts the app when it finishes. Automatic checks
 * are on by default and nothing applies an update without an explicit click.
 * A failed check is a quiet note in this card — never a toast, never a
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
import Banner from './Banner'
import Checkbox from './Checkbox'
import CodeBlock from './CodeBlock'
import { FormSection, Field } from './FormSection'
import { formatDateTime, shortHash } from './format'
import { updateStatusKey, useUpdateStatus } from './queries'
import { useToast } from './toastContext'
import {
  checkFailureMessage,
  formatPublished,
  runtimeLabel,
  stageLabel,
} from './updateModel'
import { btn, btnPrimary, linkTouch, select } from './uiControls'

function Row({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <>
      <dt className="text-fg-muted">{label}</dt>
      <dd className="min-w-0 break-words text-fg">{children}</dd>
    </>
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
    <FormSection
      title="About and updates"
      description="What this install is, and whether a newer version is out."
      testId="about-card"
    >
      <dl className="grid grid-cols-[minmax(0,8rem)_minmax(0,1fr)] gap-x-6 gap-y-2 text-body">
        <Row label="Version">
          <span className="font-mono" data-testid="about-version">
            {info?.version ?? '…'}
          </span>
        </Row>
        <Row label="Commit">
          <span
            className="font-mono"
            title={info?.commit ?? undefined}
            data-testid="about-commit"
          >
            {info?.commit ? shortHash(info.commit) : '…'}
          </span>
        </Row>
        <Row label="Built">
          <span data-testid="about-date">
            {info ? formatDateTime(info.date) : '…'}
          </span>
        </Row>
        <Row label="Install type">
          <span data-testid="about-runtime">
            {info ? runtimeLabel(info.runtime) : '…'}
          </span>
        </Row>
      </dl>

      <div className="flex flex-wrap items-center gap-x-6 gap-y-2">
        <button
          type="button"
          onClick={() => check.mutate()}
          disabled={check.isPending}
          className={btn}
          data-testid="check-now"
        >
          {check.isPending ? 'Checking…' : 'Check now'}
        </button>
        <Checkbox
          checked={settings.data?.update_check ?? true}
          onChange={(on) => saveSettings.mutate({ update_check: on })}
          label="Check for updates automatically"
          testId="update-check-toggle"
        />
      </div>

      {/* A failed check is a quiet note, never a toast (task 17). */}
      {check.isError && (
        <p className="text-body text-fg-muted" data-testid="check-error">
          {checkFailureMessage(check.error)}
        </p>
      )}

      {update && update.update_available && update.latest && (
        <Banner
          tone="info"
          testId="update-banner"
          title={`Version ${update.latest} is available${
            update.published_at
              ? ` (published ${formatPublished(update.published_at)})`
              : ''
          }.`}
        >
          {update.release_url && (
            <a
              href={update.release_url}
              target="_blank"
              rel="noreferrer"
              className={linkTouch}
              data-testid="release-notes-link"
            >
              Release notes
            </a>
          )}

          {/* The action matches the runtime (task 17). */}
          {isContainer ? (
            <div className="mt-3">
              <ContainerInstructions reason={update.reason ?? ''} />
            </div>
          ) : update.self_update_supported ? (
            <div className="mt-3 flex flex-col items-start gap-2">
              <button
                type="button"
                onClick={() => apply.mutate()}
                disabled={applying || apply.isPending}
                className={btnPrimary}
                data-testid="apply-update"
              >
                {applying || apply.isPending ? 'Updating…' : 'Update now'}
              </button>
              <p
                className="text-caption text-fg-muted"
                data-testid="update-restart-warning"
              >
                The app restarts by itself when the update finishes. The
                database is backed up first.
              </p>
            </div>
          ) : (
            <p className="mt-2 text-body" data-testid="update-unsupported">
              {update.reason}
            </p>
          )}
        </Banner>
      )}

      {/* Live progress while an in-app update runs. */}
      {update?.progress?.active && (
        <p className="text-body text-fg" data-testid="update-progress">
          {stageLabel(update.progress.stage)}
        </p>
      )}
      {update?.progress && !update.progress.active && update.progress.error && (
        <p className="text-body text-fg-muted" data-testid="update-error">
          The update did not finish: {update.progress.error}
        </p>
      )}

      {/* Container installs never self update; the commands are the fix,
          shown whether or not a newer version is out. */}
      {isContainer && !update?.update_available && (
        <ContainerInstructions reason={status.data?.reason ?? ''} />
      )}

      {!update?.update_available && update?.reason && !isContainer && (
        <p className="text-body text-fg-muted" data-testid="update-note">
          {update.reason}
        </p>
      )}

      <Field label="Update channel" htmlFor="update-channel">
        <select
          id="update-channel"
          value={channel}
          onChange={(e) =>
            saveSettings.mutate({ update_channel: e.target.value })
          }
          className={`${select} max-w-xs`}
          data-testid="update-channel"
        >
          <option value="stable">Stable releases</option>
          <option value="prerelease">Prereleases (early test versions)</option>
        </select>
      </Field>
    </FormSection>
  )
}

/** The exact host commands a container install runs to update. */
function ContainerInstructions({ reason }: { reason: string }) {
  return (
    <div className="flex flex-col gap-2">
      <CodeBlock copy testId="container-instructions">
        {reason}
      </CodeBlock>
      <p className="text-caption text-fg-muted">
        Run these on the machine that hosts the container.
      </p>
    </div>
  )
}
