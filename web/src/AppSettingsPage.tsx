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
import PageContainer from './PageContainer'
import { buildUpdate, toForm, type FormState } from './settingsModel'

export default function AppSettingsPage() {
  const query = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  const client = useQueryClient()
  const [form, setForm] = useState<FormState | null>(null)
  const [confirmingOff, setConfirmingOff] = useState(false)
  const [enablingLogin, setEnablingLogin] = useState(false)
  const [newPassword, setNewPassword] = useState('')
  const [currentPassword, setCurrentPassword] = useState('')
  const [notice, setNotice] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

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
      setNotice('Settings saved.')
      setError(null)
    },
    onError: (err: Error) => {
      setError(err.message)
      setNotice(null)
    },
  })

  if (query.isPending || form === null) {
    return <p className="text-slate-400">Loading settings…</p>
  }
  if (query.isError || !query.data) {
    return <p className="text-red-400">Cannot load the settings.</p>
  }

  const set = (patch: Partial<FormState>) => setForm({ ...form, ...patch })

  const savePlain = () => {
    setNotice(null)
    setError(null)
    save.mutate(buildUpdate(form, {}))
  }

  const confirmLoginOff = () => {
    setNotice(null)
    setError(null)
    save.mutate(buildUpdate(form, { login_enabled: false }))
  }

  const confirmLoginOn = () => {
    setNotice(null)
    setError(null)
    save.mutate(
      buildUpdate(form, {
        login_enabled: true,
        password: newPassword,
      }),
    )
  }

  const changePassword = () => {
    setNotice(null)
    setError(null)
    save.mutate(
      buildUpdate(form, {
        password: newPassword,
        current_password: currentPassword,
      }),
    )
  }

  const turningOn = !query.data.login_enabled
  const turningOff = query.data.login_enabled

  return (
    <PageContainer className="space-y-6" data-testid="app-settings-page">
      <h1 className="text-2xl font-bold">App settings</h1>
      {(notice || error) && (
        <p
          className={
            error ? 'text-sm text-red-400' : 'text-sm text-emerald-400'
          }
          role="status"
          data-testid="settings-notice"
        >
          {error ?? notice}
        </p>
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
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
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
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
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
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
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
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
                data-testid="new-password"
              />
            </label>
            <button
              type="button"
              onClick={changePassword}
              disabled={
                newPassword === '' || currentPassword === '' || save.isPending
              }
              className="justify-self-start rounded-lg bg-slate-800 px-4 py-2 text-sm font-semibold hover:bg-slate-700 disabled:opacity-50"
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
              className="rounded-lg bg-slate-800 px-4 py-2 text-sm font-semibold hover:bg-slate-700"
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
                  className="rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-500"
                  data-testid="confirm-off"
                >
                  Yes, turn it off
                </button>
                <button
                  type="button"
                  onClick={() => setConfirmingOff(false)}
                  className="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800"
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
              className="rounded-lg bg-slate-800 px-4 py-2 text-sm font-semibold hover:bg-slate-700"
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
                  className="w-full rounded-lg bg-slate-800 px-3 py-2"
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
                  className="w-full rounded-lg bg-slate-800 px-3 py-2"
                  data-testid="enable-password"
                />
              </label>
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={confirmLoginOn}
                  disabled={newPassword === '' || save.isPending}
                  className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
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
                  className="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800"
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
        className="rounded-lg bg-emerald-600 px-4 py-2 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
        data-testid="save-settings"
      >
        {save.isPending ? 'Saving…' : 'Save settings'}
      </button>
    </PageContainer>
  )
}
