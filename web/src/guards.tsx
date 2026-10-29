// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Route guard components (task 06).
 *
 * Rules from the task file:
 * - setup incomplete: every route redirects to /setup
 * - login enabled and no session: redirect to /login
 * - login disabled: everything is open
 *
 * The hooks live in queries.ts so this file only exports components.
 */
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useMe, useSetupStatus } from './queries'

function FullPageCard({ children }: { children: React.ReactNode }) {
  return (
    <main className="flex min-h-full items-center justify-center bg-slate-950 p-4 text-slate-100">
      <div className="w-full max-w-md rounded-2xl bg-slate-900 p-10 text-center shadow-xl ring-1 ring-slate-800">
        {children}
      </div>
    </main>
  )
}

function Loading() {
  return (
    <FullPageCard>
      <p className="text-slate-400">Loading…</p>
    </FullPageCard>
  )
}

/**
 * RequireReady wraps every signed-in route. While setup is incomplete the
 * user only ever sees the setup wizard; once it is done the wizard page
 * redirects away on its own.
 */
export function RequireReady() {
  const status = useSetupStatus()
  const location = useLocation()

  if (status.isPending) {
    return <Loading />
  }
  if (status.isError || !status.data) {
    return (
      <FullPageCard>
        <p className="text-red-400" data-testid="guard-error">
          Cannot reach the ShrinkYourShit server.
        </p>
      </FullPageCard>
    )
  }
  if (!status.data.setup_complete) {
    return <Navigate to="/setup" replace state={{ from: location.pathname }} />
  }
  return <RequireLogin loginEnabled={status.data.login_enabled} />
}

function RequireLogin({ loginEnabled }: { loginEnabled: boolean }) {
  const me = useMe(loginEnabled)
  const location = useLocation()

  if (!loginEnabled) {
    return <Outlet />
  }
  if (me.isPending) {
    return (
      <FullPageCard>
        <p className="text-slate-400">Checking your session…</p>
      </FullPageCard>
    )
  }
  if (me.isError) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />
  }
  return <Outlet />
}

/** Redirects away from /setup once setup has been completed. */
export function RedirectWhenSetupComplete({
  children,
}: {
  children: React.ReactNode
}) {
  const status = useSetupStatus()
  if (status.isPending) {
    return <Loading />
  }
  if (status.data?.setup_complete) {
    return <Navigate to="/" replace />
  }
  return <>{children}</>
}
