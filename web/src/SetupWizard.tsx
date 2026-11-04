// SPDX-License-Identifier: AGPL-3.0-only

/**
 * First-run setup wizard (task 06, join card from task 08, redesign task 51,
 * spec 5.1): three steps.
 * 1. Make this the server, or join an existing one (the card lists the
 *    servers discovered on the LAN and starts the pairing flow).
 * 2. Choose whether login is required, then create the account.
 * 3. Summary and finish.
 *
 * Each step's title is the screen's only h1. The state model and validation
 * live in wizardModel.ts.
 */
import { useState, type ReactNode } from 'react'
import type { DiscoveredServer, SetupRequest } from './api'
import Banner from './Banner'
import Card from './Card'
import Icon from './Icon'
import { Field } from './FormSection'
import Switch from './Switch'
import { validateAccountStep, type WizardState } from './wizardModel'
import { btnGhost, btnPrimary, input } from './uiControls'

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

const STEP_LABELS = ['Server', 'Account', 'Finish']

/** One option in step 1: a real button with `aria-pressed`. */
function optionClass(selected: boolean): string {
  return `block w-full rounded-card p-4 text-left transition-colors duration-fast ease-standard ${
    selected
      ? 'bg-raised ring-2 ring-accent'
      : 'ring-1 ring-line hover:bg-raised'
  }`
}

function Stepper({ step }: { step: number }) {
  return (
    <ol aria-label="Progress" className="flex items-center gap-2">
      {STEP_LABELS.map((label, index) => {
        const number = index + 1
        const done = number < step
        const current = number === step
        return (
          <li
            key={label}
            aria-current={current ? 'step' : undefined}
            className="flex flex-1 items-center gap-2 last:flex-none"
          >
            <span
              aria-hidden="true"
              className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-caption font-semibold ${
                done
                  ? 'bg-accent text-accent-fg'
                  : current
                    ? 'text-fg ring-2 ring-accent'
                    : 'text-fg-muted ring-1 ring-line-strong'
              }`}
            >
              {done ? <Icon name="check" className="h-4 w-4" /> : number}
            </span>
            <span
              className={`text-caption ${current ? 'font-medium text-fg' : 'text-fg-muted'}`}
            >
              {label}
              {done && <span className="sr-only"> (done)</span>}
            </span>
            {number < STEP_LABELS.length && (
              <span aria-hidden="true" className="h-px flex-1 bg-line" />
            )}
          </li>
        )
      })}
    </ol>
  )
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
      start_node: state.startNode,
    }
    if (state.loginEnabled) {
      request.username = state.username.trim()
      request.password = state.password
    }
    void onFinish(request)
  }

  let joinHelp: string
  if (!joinable) {
    joinHelp = 'No other server was found on your network.'
  } else if (picking) {
    joinHelp = 'Pick the server to join:'
  } else {
    joinHelp = 'Servers were found on your network.'
  }

  return (
    <Card padding="none" className="w-full p-6 sm:p-8" testId="setup-wizard">
      <div className="flex flex-col gap-6">
        <Stepper step={state.step} />

        {state.step === 1 && (
          <section data-testid="wizard-step-1" className="flex flex-col gap-4">
            <h1 className="text-title text-fg">
              How will you use this machine?
            </h1>
            <div className="grid gap-3">
              <button
                type="button"
                aria-pressed={state.mode === 'server'}
                onClick={() => setState({ ...state, mode: 'server' })}
                className={optionClass(state.mode === 'server')}
                data-testid="mode-server"
              >
                <span className="block text-body font-semibold text-fg">
                  Make this the server
                </span>
                <span className="mt-1 block text-body text-fg-muted">
                  It stores your library settings and does the shrinking.
                </span>
              </button>

              <div className="grid gap-3">
                <button
                  type="button"
                  aria-pressed={picking || selected !== null}
                  disabled={!joinable}
                  onClick={() => {
                    if (joinable) setPicking(true)
                  }}
                  className={`${optionClass(picking || selected !== null)} disabled:cursor-not-allowed disabled:opacity-60`}
                  data-testid="mode-join"
                >
                  <span className="block text-body font-semibold text-fg">
                    Join an existing server
                  </span>
                  <span className="mt-1 block text-body text-fg-muted">
                    {joinHelp}
                  </span>
                </button>

                {joinable && picking && (
                  <ul
                    className="divide-y divide-line overflow-hidden rounded-card ring-1 ring-line"
                    data-testid="join-servers"
                  >
                    {(servers ?? []).map((server) => (
                      <li key={server.id}>
                        <button
                          type="button"
                          className="flex min-h-11 w-full flex-col justify-center px-4 py-2 text-left hover:bg-raised"
                          data-testid={`join-server-${server.id}`}
                          onClick={() => {
                            setSelected(server)
                            setPicking(false)
                          }}
                        >
                          <span className="text-body font-medium text-fg">
                            {server.name || server.url}
                          </span>
                          <span className="text-caption text-fg-muted">
                            {server.url}
                            {server.version ? ` · ${server.version}` : ''}
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}

                {selected && (
                  <div className="flex flex-col gap-4 rounded-card p-4 ring-1 ring-line">
                    <p className="text-body text-fg">
                      Joining{' '}
                      <span className="font-medium">
                        {selected.name || selected.url}
                      </span>
                      . This machine becomes a helper; you approve it in that
                      server's Nodes page.
                    </p>
                    <Field
                      label="Pairing code"
                      htmlFor="setup-join-token"
                      optional
                      help="Only if the server gave you one."
                    >
                      <input
                        id="setup-join-token"
                        type="text"
                        value={token}
                        onChange={(e) => setToken(e.target.value)}
                        className={input}
                        data-testid="join-token"
                      />
                    </Field>
                    <div className="flex flex-wrap gap-2">
                      <button
                        type="button"
                        onClick={() => onJoin?.(selected, token.trim())}
                        disabled={joining}
                        className={btnPrimary}
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
                        className={btnGhost}
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
          <section data-testid="wizard-step-2" className="flex flex-col gap-4">
            <h1 className="text-title text-fg">Who can use it?</h1>
            <Switch
              checked={state.loginEnabled}
              onChange={(next) => setState({ ...state, loginEnabled: next })}
              label="Require a login on this server"
              visibleLabel="Require a login on this server"
              testId="login-toggle"
            />
            {state.loginEnabled && (
              <div className="grid gap-4">
                <Field label="Username" htmlFor="setup-username">
                  <input
                    id="setup-username"
                    type="text"
                    autoComplete="username"
                    value={state.username}
                    onChange={(e) =>
                      setState({ ...state, username: e.target.value })
                    }
                    className={input}
                    data-testid="username"
                  />
                </Field>
                <Field label="Password" htmlFor="setup-password">
                  <input
                    id="setup-password"
                    type="password"
                    autoComplete="new-password"
                    value={state.password}
                    onChange={(e) =>
                      setState({ ...state, password: e.target.value })
                    }
                    className={input}
                    data-testid="password"
                  />
                </Field>
                <Field label="Confirm password" htmlFor="setup-confirm">
                  <input
                    id="setup-confirm"
                    type="password"
                    autoComplete="new-password"
                    value={state.confirm}
                    onChange={(e) =>
                      setState({ ...state, confirm: e.target.value })
                    }
                    className={input}
                    data-testid="confirm"
                  />
                </Field>
              </div>
            )}
            {errors.length > 0 && (
              <Banner tone="danger" testId="step-errors">
                <ul className="list-disc space-y-1 pl-4">
                  {errors.map((error) => (
                    <li key={error}>{error}</li>
                  ))}
                </ul>
              </Banner>
            )}
          </section>
        )}

        {state.step === 3 && (
          <section data-testid="wizard-step-3" className="flex flex-col gap-4">
            <h1 className="text-title text-fg">Ready to start</h1>
            <dl className="grid gap-3 text-body">
              <SummaryRow label="Role">This machine is the server</SummaryRow>
              <SummaryRow label="Login">
                {state.loginEnabled
                  ? `Required, for the account "${state.username.trim()}"`
                  : 'Off, open to anyone on your network'}
              </SummaryRow>
            </dl>
            <fieldset className="grid gap-3">
              <legend className="mb-1 text-body font-semibold text-fg">
                When should the built-in node start working?
              </legend>
              <label className="flex min-h-11 cursor-pointer items-start gap-3 rounded-card p-4 ring-1 ring-line hover:bg-raised">
                <input
                  type="radio"
                  name="start-node"
                  checked={!state.startNode}
                  onChange={() => setState({ ...state, startNode: false })}
                  className="mt-0.5 h-5 w-5 accent-accent"
                  data-testid="leave-node-stopped"
                />
                <span className="min-w-0">
                  <span className="block text-body font-medium text-fg">
                    Leave it stopped
                  </span>
                  <span className="block text-caption text-fg-muted">
                    Nothing is encoded until you start it from the Nodes page.
                  </span>
                </span>
              </label>
              <label className="flex min-h-11 cursor-pointer items-start gap-3 rounded-card p-4 ring-1 ring-line hover:bg-raised">
                <input
                  type="radio"
                  name="start-node"
                  checked={state.startNode}
                  onChange={() => setState({ ...state, startNode: true })}
                  className="mt-0.5 h-5 w-5 accent-accent"
                  data-testid="start-node-now"
                />
                <span className="min-w-0">
                  <span className="block text-body font-medium text-fg">
                    Start the built-in node now
                  </span>
                  <span className="block text-caption text-fg-muted">
                    Queued files begin processing right away.
                  </span>
                </span>
              </label>
            </fieldset>
            {submitError && (
              <Banner tone="danger" testId="submit-error">
                {submitError}
              </Banner>
            )}
          </section>
        )}

        <div className="flex items-center justify-between gap-3">
          <button
            type="button"
            onClick={back}
            disabled={state.step === 1}
            className={`${btnGhost} ${state.step === 1 ? 'invisible' : ''}`}
            data-testid="wizard-back"
          >
            Back
          </button>
          {state.step < 3 ? (
            <button
              type="button"
              onClick={next}
              className={btnPrimary}
              data-testid="wizard-next"
            >
              Next
            </button>
          ) : (
            <button
              type="button"
              onClick={finish}
              disabled={submitting}
              className={btnPrimary}
              data-testid="wizard-finish"
            >
              {submitting ? 'Setting up…' : 'Finish'}
            </button>
          )}
        </div>
      </div>
    </Card>
  )
}

function SummaryRow({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className="flex flex-wrap justify-between gap-x-4 gap-y-1 border-b border-line pb-3">
      <dt className="text-fg-muted">{label}</dt>
      <dd className="text-right text-fg">{children}</dd>
    </div>
  )
}
