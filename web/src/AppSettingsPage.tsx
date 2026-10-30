// SPDX-License-Identifier: AGPL-3.0-only

/**
 * App settings (task 10): server-level settings — files processed at once,
 * the scratch folder, and the login toggle with confirmation and password
 * change. Split out of the old SettingsPage; the encode defaults live at
 * /encode.
 */
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getSettings, updateSettings, type SettingsUpdate } from './api'
import AboutCard from './AboutCard'
import ErrorBox from './ErrorBox'
import PageContainer from './PageContainer'
import { buildUpdate, toForm, type FormState } from './settingsModel'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { btn, btnPrimary, input } from './uiControls'

export default function AppSettingsPage() {
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

  if (query.isPending) {
    return (
      <PageContainer className="space-y-6" data-testid="app-settings-page">
        <h1 className="text-2xl font-bold">App settings</h1>
        <SkeletonRows rows={3} />
      </PageContainer>
    )
  }
  if (query.isError || !query.data) {
    return (
      <PageContainer className="space-y-6" data-testid="app-settings-page">
        <h1 className="text-2xl font-bold">App settings</h1>
        <ErrorBox title="Cannot load the settings." error={query.error} />
      </PageContainer>
    )
  }
  if (form === null) {
    return (
      <PageContainer className="space-y-6" data-testid="app-settings-page">
        <h1 className="text-2xl font-bold">App settings</h1>
        <SkeletonRows rows={3} />
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
    <PageContainer className="space-y-6" data-testid="app-settings-page">
      <h1 className="text-2xl font-bold">App settings</h1>
      {save.isError && (
        <ErrorBox
          title="Saving the settings failed."
          error={save.error}
          testId="settings-error"
        />
      )}

      <section className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <h2 className="font-semibold">Server</h2>
        <label className="mt-4 grid gap-1">
          <span className="text-sm text-slate-300">
            Files processed at once
          </span>
          <input
            type="number"
            min={1}
            max={16}
            value={form.worker_concurrency}
            onChange={(e) => set({ worker_concurrency: e.target.value })}
            className={input}
            data-testid="worker-concurrency"
          />
        </label>
        <label className="mt-3 grid gap-1">
          <span className="text-sm text-slate-300">
            Scratch folder for temporary work
          </span>
          <input
            type="text"
            value={form.scratch_dir}
            onChange={(e) => set({ scratch_dir: e.target.value })}
            className={input}
            data-testid="scratch-dir"
          />
        </label>
      </section>

      <section className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <h2 className="font-semibold">Login</h2>
        <p className="mt-1 text-sm text-slate-400" data-testid="login-state">
          {query.data.login_enabled
            ? `A login is required. You are signed in as ${query.data.admin_username}.`
            : 'No login is required; anyone who can reach this server can use it.'}
        </p>

        {query.data.login_enabled && (
          <div className="mt-4 grid gap-3">
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Current password</span>
              <input
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                autoComplete="current-password"
                className={input}
                data-testid="current-password"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">New password</span>
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                autoComplete="new-password"
                className={input}
                data-testid="new-password"
              />
            </label>
            <button
              type="button"
              onClick={changePassword}
              disabled={
                newPassword === '' || currentPassword === '' || save.isPending
              }
              className={`${btn} justify-self-start`}
              data-testid="change-password"
            >
              Change password
            </button>
          </div>
        )}

        <div className="mt-6 border-t border-slate-800 pt-4">
          {turningOff && !confirmingOff && (
            <button
              type="button"
              onClick={() => setConfirmingOff(true)}
              className={btn}
              data-testid="turn-login-off"
            >
              Turn login off
            </button>
          )}
          {turningOff && confirmingOff && (
            <div data-testid="confirm-login-off">
              <p className="text-sm text-slate-300">
                Anyone who can reach this server will be able to use it without
                logging in. Turn login off?
              </p>
              <div className="mt-2 flex gap-2">
                <button
                  type="button"
                  onClick={confirmLoginOff}
                  disabled={save.isPending}
                  className={`${btnPrimary} !bg-red-600 hover:!bg-red-500`}
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
              className={btn}
              data-testid="turn-login-on"
            >
              Turn login on
            </button>
          )}
          {turningOn && enablingLogin && (
            <div className="grid gap-3" data-testid="confirm-login-on">
              <label className="grid gap-1">
                <span className="text-sm text-slate-300">Username</span>
                <input
                  type="text"
                  value={form.admin_username}
                  onChange={(e) => set({ admin_username: e.target.value })}
                  className={input}
                  data-testid="enable-username"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-sm text-slate-300">Password</span>
                <input
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  autoComplete="new-password"
                  className={input}
                  data-testid="enable-password"
                />
              </label>
              <div className="flex gap-2">
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
      </section>

      <button
        type="button"
        onClick={savePlain}
        disabled={save.isPending}
        className={btnPrimary}
        data-testid="save-settings"
      >
        {save.isPending ? 'Saving…' : 'Save settings'}
      </button>

      <AboutCard />
    </PageContainer>
  )
}
