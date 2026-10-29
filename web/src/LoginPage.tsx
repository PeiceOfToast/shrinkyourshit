// SPDX-License-Identifier: AGPL-3.0-only

/** Login page (task 06). */
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { login, type ApiError } from './api'
import { meKey, useSetupStatus } from './queries'
import { useToast } from './toastContext'

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
    <main className="flex min-h-full items-center justify-center bg-slate-950 p-4 text-slate-100">
      <form
        className="w-full max-w-sm rounded-2xl bg-slate-900 p-8 shadow-xl ring-1 ring-slate-800"
        onSubmit={(e) => {
          e.preventDefault()
          setError(null)
          doLogin.mutate({ username, password })
        }}
        data-testid="login-form"
      >
        <h1 className="text-2xl font-bold tracking-tight">ShrinkYourShit</h1>
        <p className="mt-1 text-sm text-slate-400">Log in to continue.</p>
        <label className="mt-6 grid gap-1">
          <span className="text-sm text-slate-300">Username</span>
          <input
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
            data-testid="login-username"
          />
        </label>
        <label className="mt-3 grid gap-1">
          <span className="text-sm text-slate-300">Password</span>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
            data-testid="login-password"
          />
        </label>
        {error && (
          <p
            className="mt-3 text-sm text-red-400"
            data-testid="login-error"
            role="alert"
          >
            {error}
          </p>
        )}
        <button
          type="submit"
          disabled={doLogin.isPending}
          className="mt-6 w-full rounded-lg bg-emerald-600 px-4 py-2 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
          data-testid="login-submit"
        >
          {doLogin.isPending ? 'Logging in…' : 'Log in'}
        </button>
      </form>
    </main>
  )
}
