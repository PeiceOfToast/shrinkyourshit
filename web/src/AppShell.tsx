// SPDX-License-Identifier: AGPL-3.0-only

/**
 * App shell (task 10): responsive, five-page navigation.
 *
 * - Under `md`: a top bar with the product name, a menu button that opens
 *   the nav as a slide-over drawer, and a live job indicator. The drawer
 *   closes on route change and on Escape.
 * - `md` and up: the left rail. It shows labels at `lg` and up and icons
 *   only below that (task 21); the toggle sets an explicit override that
 *   sticks across reloads in `localStorage` under `sys.rail`.
 *
 * The shell paints `min-h-dvh` so mobile browser chrome cannot clip it;
 * the page background itself is painted on html/body (index.css) so no
 * region outside the shell renders white.
 */
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  getHealth,
  getJobs,
  getNodes,
  logout,
  pauseAllNodes,
  resumeAllNodes,
  type Job,
} from './api'
import ModalDialog from './dialog'
import { LiveEvents } from './events'
import { meKey, useMe, useSetupStatus } from './queries'
import { useToast } from './toastContext'
import { useMediaQuery } from './useMediaQuery'

/** Nav links keep at least a 44 px touch target (task 10). */
const touchLink =
  'flex min-h-[44px] items-center rounded-lg px-3 text-sm font-medium text-slate-300 hover:bg-slate-800 hover:text-white'
const navItemActive = 'bg-slate-800 text-white'

/** Minimal stroke icons; no icon dependency is on the stack. */
function Icon({
  path,
  className = 'h-5 w-5 shrink-0',
}: {
  path: string
  className?: string
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
    >
      <path d={path} />
    </svg>
  )
}

const ICONS = {
  menu: 'M4 6h16M4 12h16M4 18h16',
  close: 'M6 6l12 12M18 6 6 18',
  collapse: 'M11 17l-5-5 5-5M18 17l-5-5 5-5',
  expand: 'M13 17l5-5-5-5M6 17l5-5-5-5',
  dash: 'M3 10.5 12 3l9 7.5M5 9.5V21h5v-6h4v6h5V9.5',
  library:
    'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z',
  nodes: 'M4 5h16v6H4zM4 13h16v6H4zM7.5 8h.01M7.5 16h.01',
  encode: 'M4 6h16M4 12h16M4 18h16M9 4v4M15 10v4M11 16v4',
  app: 'M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6ZM12 4v2M12 18v2M4 12h2M18 12h2M6.3 6.3l1.4 1.4M16.3 17.7l1.4 1.4M17.7 6.3l-1.4 1.4M7.7 17.7l-1.4 1.4',
}

const NAV_ITEMS = [
  { to: '/', label: 'Dash', end: true, icon: ICONS.dash },
  { to: '/libraries', label: 'Library', icon: ICONS.library },
  { to: '/nodes', label: 'Nodes', icon: ICONS.nodes },
  { to: '/encode', label: 'Encode settings', icon: ICONS.encode },
  { to: '/app', label: 'App settings', icon: ICONS.app },
]

function NavLinks({ showLabels }: { showLabels: boolean }) {
  return (
    <nav aria-label="Main" className="flex flex-col gap-1 p-3">
      {NAV_ITEMS.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.end}
          title={showLabels ? undefined : item.label}
          className={({ isActive }) =>
            `${touchLink} ${showLabels ? '' : 'justify-center'} ${
              isActive ? navItemActive : ''
            }`
          }
        >
          <Icon path={item.icon} />
          {showLabels && <span className="ml-3">{item.label}</span>}
        </NavLink>
      ))}
    </nav>
  )
}

function LogoutButton() {
  const navigate = useNavigate()
  const client = useQueryClient()
  const toast = useToast()
  const status = useSetupStatus()
  const me = useMe(status.data?.login_enabled ?? false)
  const doLogout = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      toast.success('Logged out.')
      client.setQueryData(meKey, undefined)
      void navigate(status.data?.login_enabled ? '/login' : '/')
    },
    onError: (err: unknown) => toast.failure('Logging out failed', err),
  })

  if (status.data && !status.data.login_enabled) {
    return null
  }
  return (
    <button
      type="button"
      className="flex min-h-[44px] w-full items-center justify-center rounded-lg bg-slate-800 px-3 py-2 text-sm text-slate-200 hover:bg-slate-700"
      onClick={() => doLogout.mutate()}
      data-testid="logout"
    >
      Log out
      {me.data?.username ? ` (${me.data.username})` : ''}
    </button>
  )
}

/**
 * Live job indicator (task 10): counts running and claimed jobs. The
 * shell's SSE subscription invalidates the `jobs` prefix on job.updated,
 * so the pill follows the queue without polling. Hidden when idle.
 */
function JobIndicator() {
  const jobs = useQuery({
    queryKey: ['jobs', 'indicator'],
    queryFn: () => getJobs({ limit: 50 }),
  })
  const active = (jobs.data?.items ?? []).filter((j: Job) => {
    return j.status === 'running' || j.status === 'claimed'
  }).length
  if (active === 0) {
    return null
  }
  return (
    <span
      className="inline-flex min-h-[28px] items-center gap-1.5 rounded-full bg-sky-950 px-2.5 py-1 text-xs font-medium text-sky-300"
      data-testid="job-indicator"
    >
      <span
        className="h-1.5 w-1.5 animate-pulse rounded-full bg-sky-400"
        aria-hidden="true"
      />
      {active} running
    </span>
  )
}

const brand = (
  <span className="text-lg font-bold tracking-tight">ShrinkYourShit</span>
)

/**
 * Global "Pause all encoding" switch (task 11): stops every node in one
 * click and restores the previous per-node intent when turned off. ON
 * means every node is stopped/draining; off means at least one is running.
 */
function PauseAllSwitch() {
  const client = useQueryClient()
  const toast = useToast()
  const nodes = useQuery({ queryKey: ['nodes'], queryFn: getNodes })
  const items = nodes.data?.items ?? []
  const paused =
    items.length > 0 && items.every((n) => n.desired_state !== 'running')

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ['nodes'] })
  }
  const pause = useMutation({
    mutationFn: pauseAllNodes,
    onSuccess: ({ paused: n }) => {
      toast.success(
        n > 0
          ? `Encoding paused on ${n} node${n === 1 ? '' : 's'}.`
          : 'Encoding paused.',
      )
      invalidate()
    },
    onError: (err: unknown) => toast.failure('Pausing all nodes failed', err),
  })
  const resume = useMutation({
    mutationFn: resumeAllNodes,
    onSuccess: ({ resumed: n }) => {
      toast.success(
        n > 0
          ? `Encoding resumed on ${n} node${n === 1 ? '' : 's'}.`
          : 'Nothing to resume.',
      )
      invalidate()
    },
    onError: (err: unknown) => toast.failure('Resuming all nodes failed', err),
  })
  const busy = pause.isPending || resume.isPending

  return (
    <label
      className="flex cursor-pointer select-none items-center gap-2 text-xs font-medium text-slate-300"
      data-testid="pause-all-switch"
    >
      <input
        type="checkbox"
        checked={paused}
        disabled={busy || items.length === 0}
        onChange={() => (paused ? resume.mutate() : pause.mutate())}
        className="h-4 w-4 rounded accent-amber-500"
        data-testid="pause-all-checkbox"
      />
      Pause all encoding
    </label>
  )
}

export default function AppShell() {
  const health = useQuery({ queryKey: ['health'], queryFn: getHealth })
  const isDesktop = useMediaQuery('(min-width: 768px)')
  const wide = useMediaQuery('(min-width: 1024px)')
  const [drawerOpen, setDrawerOpen] = useState(false)
  const location = useLocation()

  // The rail collapses itself below lg (task 21). `auto` follows the
  // viewport; an explicit toggle wins until it is toggled back, and the
  // choice survives a reload via localStorage. Anything unrecognised in
  // storage is treated as auto, not as a choice.
  const [override, setOverride] = useState<'auto' | 'expanded' | 'collapsed'>(
    () => {
      const saved =
        typeof localStorage === 'undefined'
          ? null
          : localStorage.getItem('sys.rail')
      return saved === 'expanded' || saved === 'collapsed' ? saved : 'auto'
    },
  )
  const showLabels = override === 'auto' ? wide : override === 'expanded'

  function toggleRail() {
    const next = showLabels ? 'collapsed' : 'expanded'
    setOverride(next)
    localStorage.setItem('sys.rail', next)
  }

  // The drawer closes on every route change, including plain link clicks.
  useEffect(() => {
    setDrawerOpen(false)
  }, [location.pathname])

  // Leaving the narrow layout (rotation, window resize) closes the drawer.
  useEffect(() => {
    if (isDesktop) {
      setDrawerOpen(false)
    }
  }, [isDesktop])

  return (
    <div
      className="flex min-h-dvh bg-slate-950 text-slate-100"
      data-testid="app-shell"
    >
      <LiveEvents />

      {isDesktop && (
        <aside
          className={`flex shrink-0 flex-col border-r border-slate-800 bg-slate-900 ${
            showLabels ? 'w-56' : 'w-16'
          }`}
          data-testid="nav-rail"
        >
          <div
            className={`py-5 ${showLabels ? 'px-4' : 'px-2 text-center'}`}
          >
            {showLabels ? (
              brand
            ) : (
              <span
                className="text-lg font-bold tracking-tight"
                title="ShrinkYourShit"
              >
                SYS
              </span>
            )}
          </div>
          <NavLinks showLabels={showLabels} />
          <div className={`mt-auto p-3 ${showLabels ? '' : 'px-2'}`}>
            <button
              type="button"
              onClick={toggleRail}
              aria-label={showLabels ? 'Collapse navigation' : 'Show labels'}
              aria-expanded={showLabels}
              className="flex min-h-[44px] w-full items-center justify-center rounded-lg px-3 text-slate-400 hover:bg-slate-800 hover:text-white"
              data-testid="nav-toggle"
            >
              <Icon path={showLabels ? ICONS.collapse : ICONS.expand} />
            </button>
          </div>
        </aside>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        {isDesktop ? (
          <header className="flex items-center justify-between border-b border-slate-800 bg-slate-900 px-6 py-3">
            <span className="text-sm text-slate-400" data-testid="version">
              {health.data ? health.data.version : ''}
            </span>
            <div className="flex items-center gap-3">
              <JobIndicator />
              <PauseAllSwitch />
              <div className="w-auto">
                <LogoutButton />
              </div>
            </div>
          </header>
        ) : (
          <header className="flex items-center justify-between gap-2 border-b border-slate-800 bg-slate-900 px-3 py-2">
            <button
              type="button"
              onClick={() => setDrawerOpen(true)}
              aria-label="Open menu"
              aria-expanded={drawerOpen}
              className="flex h-11 w-11 items-center justify-center rounded-lg text-slate-300 hover:bg-slate-800 hover:text-white"
              data-testid="menu-button"
            >
              <Icon path={ICONS.menu} className="h-6 w-6" />
            </button>
            {brand}
            <div className="flex items-center gap-2">
              <PauseAllSwitch />
              <JobIndicator />
            </div>
          </header>
        )}

        <main className="min-w-0 flex-1 py-4 sm:py-6 lg:py-8">
          <Outlet />
        </main>
      </div>

      {/* The nav drawer is a native dialog: Escape, the backdrop click and
          the focus trap/return come from ModalDialog (task 13). */}
      {!isDesktop && drawerOpen && (
        <ModalDialog
          onClose={() => setDrawerOpen(false)}
          label="Navigation"
          testId="nav-drawer"
          dismissOnBackdrop
          className="m-0 flex h-dvh w-72 max-w-[85vw] flex-col rounded-r-2xl bg-slate-900 text-slate-100 shadow-xl"
        >
          <div className="flex items-center justify-between border-b border-slate-800 py-2 pl-4 pr-2">
            {brand}
            <button
              type="button"
              onClick={() => setDrawerOpen(false)}
              aria-label="Close menu"
              className="flex h-11 w-11 items-center justify-center rounded-lg text-slate-300 hover:bg-slate-800 hover:text-white"
              data-testid="close-menu"
            >
              <Icon path={ICONS.close} className="h-6 w-6" />
            </button>
          </div>
          <NavLinks showLabels />
          <div className="mt-auto border-t border-slate-800 p-3">
            <LogoutButton />
          </div>
        </ModalDialog>
      )}
    </div>
  )
}
