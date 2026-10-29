// SPDX-License-Identifier: AGPL-3.0-only

/**
 * App shell (task 06): left navigation, top bar with the version and
 * logout. Dashboard and Libraries are placeholders until task 07.
 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { NavLink, Outlet, useNavigate } from 'react-router-dom'
import { getHealth, logout } from './api'
import { LiveEvents } from './events'
import { meKey, useMe, useSetupStatus } from './queries'

const navItem =
  'block rounded-lg px-3 py-2 text-sm font-medium text-slate-300 hover:bg-slate-800 hover:text-white'
const navItemActive = 'bg-slate-800 text-white'

function Nav() {
  const items = [
    { to: '/', label: 'Dashboard', end: true },
    { to: '/libraries', label: 'Libraries' },
    { to: '/nodes', label: 'Nodes' },
    { to: '/settings', label: 'Settings' },
  ]
  return (
    <nav aria-label="Main" className="flex flex-col gap-1 p-3">
      {items.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.end}
          className={({ isActive }) =>
            `${navItem} ${isActive ? navItemActive : ''}`
          }
        >
          {item.label}
        </NavLink>
      ))}
    </nav>
  )
}

function LogoutButton() {
  const navigate = useNavigate()
  const client = useQueryClient()
  const status = useSetupStatus()
  const me = useMe(status.data?.login_enabled ?? false)
  const doLogout = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      client.setQueryData(meKey, undefined)
      void navigate(status.data?.login_enabled ? '/login' : '/')
    },
  })

  if (status.data && !status.data.login_enabled) {
    return null
  }
  return (
    <button
      type="button"
      className="rounded-lg bg-slate-800 px-3 py-1.5 text-sm text-slate-200 hover:bg-slate-700"
      onClick={() => doLogout.mutate()}
      data-testid="logout"
    >
      Log out
      {me.data?.username ? ` (${me.data.username})` : ''}
    </button>
  )
}

export default function AppShell() {
  const health = useQuery({ queryKey: ['health'], queryFn: getHealth })

  return (
    <div className="flex min-h-full bg-slate-950 text-slate-100">
      <LiveEvents />
      <aside className="flex w-56 shrink-0 flex-col border-r border-slate-800 bg-slate-900">
        <div className="px-4 py-5">
          <span className="text-lg font-bold tracking-tight">
            ShrinkYourShit
          </span>
        </div>
        <Nav />
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex items-center justify-between border-b border-slate-800 bg-slate-900 px-6 py-3">
          <span className="text-sm text-slate-400" data-testid="version">
            {health.data ? health.data.version : ''}
          </span>
          <LogoutButton />
        </header>
        <main className="min-w-0 flex-1 p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
