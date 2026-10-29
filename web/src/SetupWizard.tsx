// SPDX-License-Identifier: AGPL-3.0-only

/**
 * First-run setup wizard (task 06, join card from task 08): three steps.
 * 1. Make this the server, or join an existing one (the card lists the
 *    servers discovered on the LAN and starts the pairing flow).
 * 2. Choose whether login is required, then create the account.
 * 3. Summary and finish.
 *
 * The state model and validation live in wizardModel.ts.
 */
import { useState } from 'react'
import type { DiscoveredServer, SetupRequest } from './api'
import { validateAccountStep, type WizardState } from './wizardModel'

interface SetupWizardProps {
  state: WizardState
  setState: (next: WizardState) => void
  onFinish: (request: SetupRequest) => Promise<void>
  /** Servers found on the LAN; empty or missing keeps the card disabled. */
  servers?: DiscoveredServer[]
  /** Starts the pairing flow for the chosen server. */
  onJoin?: (server: DiscoveredServer, token: string) => void
  /** True while the join request is in flight. */
  joining?: boolean
  submitting?: boolean
  submitError?: string | null
}

export default function SetupWizard({
  state,
  setState,
  onFinish,
  servers,
  onJoin,
  joining = false,
  submitting = false,
  submitError = null,
}: SetupWizardProps) {
  const [errors, setErrors] = useState<string[]>([])
  const [picking, setPicking] = useState(false)
  const [selected, setSelected] = useState<DiscoveredServer | null>(null)
  const [token, setToken] = useState('')

  const joinable = (servers?.length ?? 0) > 0

  const next = () => {
    if (state.step === 2) {
      const problems = validateAccountStep(state)
      setErrors(problems)
      if (problems.length > 0) {
        return
      }
    }
    setErrors([])
    setState({ ...state, step: Math.min(state.step + 1, 3) })
  }

  const back = () => {
    setErrors([])
    setState({ ...state, step: Math.max(state.step - 1, 1) })
  }

  const finish = () => {
    const request: SetupRequest = {
      mode: 'server',
      login_enabled: state.loginEnabled,
    }
    if (state.loginEnabled) {
      request.username = state.username.trim()
      request.password = state.password
    }
    void onFinish(request)
  }

  return (
    <div className="w-full max-w-xl rounded-2xl bg-slate-900 p-8 shadow-xl ring-1 ring-slate-800">
      <ol
        className="mb-6 flex gap-2 text-xs text-slate-400"
        aria-label="Progress"
      >
        <li className={state.step === 1 ? 'font-bold text-white' : ''}>
          1. Server
        </li>
        <li className={state.step === 2 ? 'font-bold text-white' : ''}>
          2. Account
        </li>
        <li className={state.step === 3 ? 'font-bold text-white' : ''}>
          3. Finish
        </li>
      </ol>

      {state.step === 1 && (
        <section data-testid="wizard-step-1">
          <h1 className="text-xl font-bold">How will you use this machine?</h1>
          <div className="mt-4 grid gap-3">
            <button
              type="button"
              aria-pressed={state.mode === 'server'}
              onClick={() => setState({ ...state, mode: 'server' })}
              className={`rounded-xl border p-4 text-left ${
                state.mode === 'server'
                  ? 'border-emerald-500 bg-slate-800'
                  : 'border-slate-700 hover:border-slate-500'
              }`}
              data-testid="mode-server"
            >
              <span className="block font-semibold">Make this the server</span>
              <span className="mt-1 block text-sm text-slate-400">
                It stores your library settings and does the shrinking.
              </span>
            </button>
            <div
              className={`rounded-xl border p-4 text-left ${
                joinable
                  ? 'cursor-pointer border-slate-700 hover:border-slate-500'
                  : 'cursor-not-allowed border-slate-800 opacity-50'
              }`}
              data-testid="mode-join"
              onClick={() => {
                if (joinable && (servers?.length ?? 0) > 0) setPicking(true)
              }}
            >
              <span className="block font-semibold">
                Join an existing server
              </span>
              <span className="mt-1 block text-sm text-slate-400">
                {joinable
                  ? picking
                    ? 'Pick the server to join:'
                    : (servers?.length ?? 0) > 0
                      ? 'Servers were found on your network.'
                      : 'Searching your network for a server…'
                  : 'No other server was found on your network.'}
              </span>
              {joinable && picking && (
                <ul className="mt-3 space-y-2" data-testid="join-servers">
                  {(servers ?? []).map((server) => (
                    <li key={server.id}>
                      <button
                        type="button"
                        className="w-full rounded-lg bg-slate-950 px-3 py-2 text-left text-sm hover:bg-slate-800"
                        data-testid={`join-server-${server.id}`}
                        onClick={() => {
                          setSelected(server)
                          setPicking(false)
                        }}
                      >
                        <span className="font-medium text-slate-100">
                          {server.name || server.url}
                        </span>
                        <span className="block text-xs text-slate-500">
                          {server.url}
                          {server.version ? ` · ${server.version}` : ''}
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
              {selected && (
                <div className="mt-3 rounded-lg bg-slate-950 p-3">
                  <p className="text-sm text-slate-300">
                    Joining{' '}
                    <span className="font-medium">
                      {selected.name || selected.url}
                    </span>
                    . This machine becomes a helper; you approve it in that
                    server's Nodes page.
                  </p>
                  <label className="mt-2 block">
                    <span className="text-xs text-slate-500">
                      Pairing code (optional, if the server gave you one)
                    </span>
                    <input
                      type="text"
                      value={token}
                      onChange={(e) => setToken(e.target.value)}
                      className="mt-1 w-full rounded-lg bg-slate-800 px-3 py-2 text-sm"
                      data-testid="join-token"
                    />
                  </label>
                  <div className="mt-3 flex gap-2">
                    <button
                      type="button"
                      onClick={() => onJoin?.(selected, token.trim())}
                      disabled={joining}
                      className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
                      data-testid="join-confirm"
                    >
                      {joining ? 'Joining…' : 'Join this server'}
                    </button>
                    <button
                      type="button"
                      onClick={() => {
                        setSelected(null)
                        setToken('')
                      }}
                      className="rounded-lg px-3 py-2 text-sm text-slate-300 hover:bg-slate-800"
                      data-testid="join-cancel"
                    >
                      Back
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </section>
      )}

      {state.step === 2 && (
        <section data-testid="wizard-step-2">
          <h1 className="text-xl font-bold">Who can use it?</h1>
          <label className="mt-4 flex items-center gap-3">
            <input
              type="checkbox"
              checked={state.loginEnabled}
              onChange={(e) =>
                setState({ ...state, loginEnabled: e.target.checked })
              }
              data-testid="login-toggle"
            />
            <span>Require a login on this server</span>
          </label>
          {state.loginEnabled && (
            <div className="mt-4 grid gap-3">
              <label className="grid gap-1">
                <span className="text-sm text-slate-300">Username</span>
                <input
                  type="text"
                  value={state.username}
                  onChange={(e) =>
                    setState({ ...state, username: e.target.value })
                  }
                  className="rounded-lg bg-slate-800 px-3 py-2"
                  data-testid="username"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-sm text-slate-300">Password</span>
                <input
                  type="password"
                  value={state.password}
                  onChange={(e) =>
                    setState({ ...state, password: e.target.value })
                  }
                  className="rounded-lg bg-slate-800 px-3 py-2"
                  data-testid="password"
                />
              </label>
              <label className="grid gap-1">
                <span className="text-sm text-slate-300">Confirm password</span>
                <input
                  type="password"
                  value={state.confirm}
                  onChange={(e) =>
                    setState({ ...state, confirm: e.target.value })
                  }
                  className="rounded-lg bg-slate-800 px-3 py-2"
                  data-testid="confirm"
                />
              </label>
            </div>
          )}
          {errors.length > 0 && (
            <ul className="mt-3 text-sm text-red-400" data-testid="step-errors">
              {errors.map((error) => (
                <li key={error}>{error}</li>
              ))}
            </ul>
          )}
        </section>
      )}

      {state.step === 3 && (
        <section data-testid="wizard-step-3">
          <h1 className="text-xl font-bold">Ready to start</h1>
          <p className="mt-3 text-slate-300">
            This machine will be the server
            {state.loginEnabled
              ? `, and the account "${state.username.trim()}" will be required to log in.`
              : ', open for anyone on your network to use.'}
          </p>
          {submitError && (
            <p className="mt-3 text-sm text-red-400" data-testid="submit-error">
              {submitError}
            </p>
          )}
        </section>
      )}

      <div className="mt-6 flex justify-between">
        <button
          type="button"
          onClick={back}
          disabled={state.step === 1}
          className="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-0"
          data-testid="wizard-back"
        >
          Back
        </button>
        {state.step < 3 ? (
          <button
            type="button"
            onClick={next}
            className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500"
            data-testid="wizard-next"
          >
            Next
          </button>
        ) : (
          <button
            type="button"
            onClick={finish}
            disabled={submitting}
            className="rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
            data-testid="wizard-finish"
          >
            {submitting ? 'Setting up…' : 'Finish'}
          </button>
        )}
      </div>
    </div>
  )
}
