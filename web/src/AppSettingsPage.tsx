// SPDX-License-Identifier: AGPL-3.0-only

/**
 * App settings (task 10, restyled task 49, spec 5.11): server-level settings —
 * files processed at once, the scratch folder, and the login toggle with
 * confirmation and password change — then the About and updates section. The
 * Save action sits directly under the Server section because it only saves
 * Server. The encode defaults live at /encode.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getSettings, updateSettings, type SettingsUpdate } from './api'
import AboutCard from './AboutCard'
import ErrorBox from './ErrorBox'
import { Field, FormActions, FormSection } from './FormSection'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { buildUpdate, toForm, type FormState } from './settingsModel'
import { SkeletonCard } from './skeleton'
import { useToast } from './toastContext'
import { btn, btnDanger, btnPrimary, input } from './uiControls'

export default function AppSettingsPage() {
  useDocumentTitle(['App settings'])
  const query = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  const client = useQueryClient()
  const toast = useToast()
  const [form, setForm] = useState<FormState | null>(null)
  const [confirmingOff, setConfirmingOff] = useState(false)
  const [enablingLogin, setEnablingLogin] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')

  useEffect(() => {
    if (query.data && form === null) {
      setForm(toForm(query.data))
    }
  }, [query.data, form])

  const save = useMutation({
    mutationFn: (body: SettingsUpdate) => updateSettings(body),
    onSuccess: (updated) => {
      client.setQueryData(['settings'], updated)
      setForm(toForm(updated))
      setNewPassword('')
      setCurrentPassword('')
      setEnablingLogin(false)
      setConfirmingOff(false)
      toast.success('Settings saved.')
    },
    onError: (err: unknown) => toast.failure('Saving settings failed', err),
  })

  const header = <PageHeader title="App settings" />

  if (query.isPending) {
    return (
      <PageContainer data-testid="app-settings-page">
        {header}
        <SkeletonCard className="h-48" />
      </PageContainer>
    )
  }
  if (query.isError || !query.data) {
    return (
      <PageContainer data-testid="app-settings-page">
        {header}
        <ErrorBox
          title="Cannot load the settings."
          error={query.error}
          onRetry={() => void query.refetch()}
        />
      </PageContainer>
    )
  }
  if (form === null) {
    return (
      <PageContainer data-testid="app-settings-page">
        {header}
        <SkeletonCard className="h-48" />
      </PageContainer>
    )
  }

  const set = (patch: Partial<FormState>) => setForm({ ...form, ...patch })

  const savePlain = () => save.mutate(buildUpdate(form, {}))

  const confirmLoginOff = () =>
    save.mutate(buildUpdate(form, { login_enabled: false }))

  const confirmLoginOn = () =>
    save.mutate(
      buildUpdate(form, {
        login_enabled: true,
        password: newPassword,
      }),
    )

  const changePassword = () =>
    save.mutate(
      buildUpdate(form, {
        password: newPassword,
        current_password: currentPassword,
      }),
    )

  const turningOn = !query.data.login_enabled
  const turningOff = query.data.login_enabled

  return (
    <PageContainer data-testid="app-settings-page">
      {header}

      <FormSection title="Server" description="How this machine runs the work.">
        <Field label="Files processed at once" htmlFor="worker-concurrency">
          <input
            id="worker-concurrency"
            type="number"
            min={1}
            max={16}
            value={form.worker_concurrency}
            onChange={(e) => set({ worker_concurrency: e.target.value })}
            className={`${input} w-32`}
            data-testid="worker-concurrency"
          />
        </Field>
        <Field
          label="Scratch folder"
          htmlFor="scratch-dir"
          help="Where copies are made before they replace the original. Leave empty to use the default."
        >
          <input
            id="scratch-dir"
            type="text"
            value={form.scratch_dir}
            onChange={(e) => set({ scratch_dir: e.target.value })}
            placeholder="Default scratch folder"
            className={`${input} font-mono`}
            data-testid="scratch-dir"
          />
        </Field>
      </FormSection>

      {save.isError && (
        <ErrorBox
          title="Saving the settings failed."
          error={save.error}
          testId="settings-error"
        />
      )}
      <FormActions>
        <button
          type="button"
          onClick={savePlain}
          disabled={save.isPending}
          className={btnPrimary}
          data-testid="save-settings"
        >
          {save.isPending ? 'Saving…' : 'Save changes'}
        </button>
      </FormActions>

      <FormSection title="Login">
        <p className="text-body text-fg-muted" data-testid="login-state">
          {query.data.login_enabled
            ? `A login is required. You are signed in as ${query.data.admin_username}.`
            : 'No login is required; anyone who can reach this server can use it.'}
        </p>

        {query.data.login_enabled && (
          <>
            <Field label="Current password" htmlFor="current-password">
              <input
                id="current-password"
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                autoComplete="current-password"
                className={input}
                data-testid="current-password"
              />
            </Field>
            <Field label="New password" htmlFor="new-password">
              <input
                id="new-password"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
                className={input}
                data-testid="new-password"
              />
            </Field>
            <button
              type="button"
              onClick={changePassword}
              disabled={
                newPassword === '' || currentPassword === '' || save.isPending
              }
              className={`${btn} self-start`}
              data-testid="change-password"
            >
              Change password
            </button>
          </>
        )}

        <div className="flex flex-col gap-4">
          {turningOff && !confirmingOff && (
            <button
              type="button"
              onClick={() => setConfirmingOff(true)}
              className={`${btn} self-start`}
              data-testid="turn-login-off"
            >
              Turn login off
            </button>
          )}
          {turningOff && confirmingOff && (
            <div
              className="flex flex-col gap-3"
              data-testid="confirm-login-off"
            >
              <p className="text-body text-fg">
                Anyone who can reach this server will be able to use it without
                logging in. Turn login off?
              </p>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={confirmLoginOff}
                  disabled={save.isPending}
                  className={btnDanger}
                  data-testid="confirm-off"
                >
                  Yes, turn it off
                </button>
                <button
                  type="button"
                  onClick={() => setConfirmingOff(false)}
                  className={btn}
                  data-testid="cancel-off"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
          {turningOn && !enablingLogin && (
            <button
              type="button"
              onClick={() => setEnablingLogin(true)}
              className={`${btn} self-start`}
              data-testid="turn-login-on"
            >
              Turn login on
            </button>
          )}
          {turningOn && enablingLogin && (
            <div className="flex flex-col gap-4" data-testid="confirm-login-on">
              <Field label="Username" htmlFor="enable-username">
                <input
                  id="enable-username"
                  type="text"
                  value={form.admin_username}
                  onChange={(e) => set({ admin_username: e.target.value })}
                  className={input}
                  data-testid="enable-username"
                />
              </Field>
              <Field label="Password" htmlFor="enable-password">
                <input
                  id="enable-password"
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  autoComplete="new-password"
                  className={input}
                  data-testid="enable-password"
                />
              </Field>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  onClick={confirmLoginOn}
                  disabled={newPassword === '' || save.isPending}
                  className={btnPrimary}
                  data-testid="confirm-on"
                >
                  Save and require login
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setEnablingLogin(false)
                    setNewPassword('')
                  }}
                  className={btn}
                  data-testid="cancel-on"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </div>
      </FormSection>

      <AboutCard />
    </PageContainer>
  )
}
