// SPDX-License-Identifier: AGPL-3.0-only

/** Login page (task 06). */
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { login, type ApiError } from './api'
import { meKey, useSetupStatus } from './queries'
import { useToast } from './toastContext'
import Banner from './Banner'
import Icon from './Icon'
import { Field } from './FormSection'
import { btnBlock, btnPrimary, card, input } from './uiControls'

export default function LoginPage() {
  const status = useSetupStatus()
  const navigate = useNavigate()
  const location = useLocation()
  const client = useQueryClient()
  const toast = useToast()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)

  const doLogin = useMutation({
    mutationFn: login,
    onSuccess: (me) => {
      client.setQueryData(meKey, me)
      const from = (location.state as { from?: string } | null)?.from
      void navigate(from && from !== '/login' ? from : '/', { replace: true })
    },
    onError: (err: ApiError) => {
      toast.failure('Login failed', err)
      setError(
        err.status === 429
          ? 'Too many attempts. Try again in a minute.'
          : 'Wrong username or password.',
      )
    },
  })

  // Setup must come first; once it is done the wizard page redirects away.
  if (status.data && !status.data.setup_complete) {
    return <Navigate to="/setup" replace />
  }
  if (status.data && !status.data.login_enabled) {
    return <Navigate to="/" replace />
  }

  return (
    <main className="grid min-h-dvh place-items-center bg-canvas p-4 text-fg">
      <form
        className={`${card} flex w-full max-w-sm flex-col gap-6 p-6 sm:p-8`}
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          doLogin.mutate({ username, password })
        }}
        data-testid="login-form"
      >
        <div className="flex flex-col items-center gap-2 text-center">
          <Icon name="brand" className="h-8 w-8 text-accent-text" />
          <h1 className="text-title text-fg">Log in</h1>
          <p className="text-body text-fg-muted">to ShrinkYourShit</p>
        </div>
        <div className="flex flex-col gap-4">
          <Field label="Username" htmlFor="login-username">
            <input
              id="login-username"
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              className={input}
              data-testid="login-username"
            />
          </Field>
          <Field label="Password" htmlFor="login-password">
            <input
              id="login-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              className={input}
              data-testid="login-password"
            />
          </Field>
        </div>
        {error && (
          <Banner tone="danger" testId="login-error">
            {error}
          </Banner>
        )}
        <button
          type="submit"
          disabled={doLogin.isPending}
          className={`${btnPrimary} ${btnBlock}`}
          data-testid="login-submit"
        >
          {doLogin.isPending ? 'Logging in…' : 'Log in'}
        </button>
      </form>
    </main>
  )
}
