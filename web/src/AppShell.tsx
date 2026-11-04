// SPDX-License-Identifier: AGPL-3.0-only

/**
 * App shell (tasks 10, 21 and 40): responsive, six-destination navigation.
 *
 * - `lg` and up: a labelled left rail, sticky, with no top bar. Its footer
 *   holds EncodingStatus, the user and logout, the version and the rail
 *   toggle (spec 3.1).
 * - `md` to `lg`: the same rail as icons only (`w-16`). The toggle sets an
 *   explicit override that sticks across reloads in `localStorage`
 *   under `sys.rail`.
 * - Under `md`: a 56 px top bar with the menu, the brand and a compact
 *   EncodingStatus pill. The menu opens the nav as a drawer, which holds
 *   the EncodingStatus switch and logout as well. The drawer closes on
 *   route change and on Escape.
 *
 * A skip link is the first focusable element and targets `main#content`.
 * The shell paints `min-h-dvh` so mobile browser chrome cannot clip it;
 * the page background itself is painted on html/body (index.css).
 */
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Link,
  NavLink,
  Outlet,
  useLocation,
  useNavigate,
} from 'react-router-dom'
import { getHealth, logout } from './api'
import Icon, { type IconName } from './Icon'
import ModalDialog from './dialog'
import EncodingStatus from './EncodingStatus'
import { LiveEvents } from './events'
import { meKey, useMe, useSetupStatus } from './queries'
import { useToast } from './toastContext'
import { useMediaQuery } from './useMediaQuery'
import { btnIcon } from './uiControls'

/** Nav item base, from spec 3.1. Active adds the 2 px accent bar. */
const navBase =
  'flex min-h-11 items-center gap-3 rounded-control px-3 text-body font-medium text-fg-muted hover:bg-raised hover:text-fg'
const navActive =
  'relative bg-raised text-fg before:absolute before:left-0 before:top-2 before:bottom-2 before:w-0.5 before:rounded-full before:bg-accent'

const NAV_ITEMS: Array<{
  to: string
  label: string
  end?: boolean
  icon: IconName
}> = [
  { to: '/', label: 'Dashboard', end: true, icon: 'dash' },
  { to: '/libraries', label: 'Libraries', icon: 'library' },
  { to: '/nodes', label: 'Nodes', icon: 'nodes' },
  { to: '/encode', label: 'Encode settings', icon: 'encode' },
  { to: '/integrations', label: 'Integrations', icon: 'integrations' },
  { to: '/app', label: 'App settings', icon: 'app' },
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
            `${navBase} ${showLabels ? '' : 'justify-center'} ${
              isActive ? navActive : ''
            }`
          }
        >
          <Icon name={item.icon} />
          {showLabels && <span>{item.label}</span>}
        </NavLink>
      ))}
    </nav>
  )
}

/** The mark (always) and the wordmark (only when there is room for it). */
function Brand({ showWord }: { showWord: boolean }) {
  if (!showWord) {
    return (
      <span
        role="img"
        aria-label="ShrinkYourShit"
        title="ShrinkYourShit"
        className="inline-flex shrink-0"
      >
        <Icon name="brand" className="h-5 w-5 shrink-0 text-accent-text" />
      </span>
    )
  }
  return (
    <span className="flex min-w-0 items-center gap-2 text-fg">
      <Icon name="brand" className="h-5 w-5 shrink-0 text-accent-text" />
      <span className="truncate text-heading font-semibold tracking-tight">
        ShrinkYourShit
      </span>
    </span>
  )
}

/** Logout as an icon button, with the user's name beside it when labels show. */
function LogoutControl({ showName }: { showName: boolean }) {
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
    <div className="flex min-w-0 items-center gap-2">
      {showName && (
        <span className="min-w-0 truncate text-body text-fg-muted">
          {me.data?.username ?? ''}
        </span>
      )}
      <button
        type="button"
        className={btnIcon}
        onClick={() => doLogout.mutate()}
        aria-label="Log out"
        title="Log out"
        data-testid="logout"
      >
        <Icon name="logout" />
      </button>
    </div>
  )
}

/** The running version, linking to App settings. Hidden until loaded. */
function VersionLink({ version }: { version?: string }) {
  if (!version) {
    return null
  }
  return (
    <Link
      to="/app"
      title="App settings"
      className="min-w-0 truncate text-caption text-fg-muted hover:text-fg"
      data-testid="version"
    >
      {version}
    </Link>
  )
}

export default function AppShell() {
  const health = useQuery({ queryKey: ['health'], queryFn: getHealth })
  const version = health.data?.version
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
    <div className="flex min-h-dvh bg-canvas text-fg" data-testid="app-shell">
      <a
        href="#content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:z-toast focus:inline-flex focus:min-h-11 focus:items-center focus:rounded-control focus:bg-accent focus:px-4 focus:text-body focus:font-medium focus:text-accent-fg"
      >
        Skip to content
      </a>
      <LiveEvents />

      {isDesktop && (
        <aside
          className={`sticky top-0 flex h-dvh shrink-0 flex-col border-r border-line bg-surface z-rail ${
            showLabels ? 'w-56' : 'w-16'
          }`}
          data-testid="nav-rail"
        >
          <div
            className={`flex py-5 ${showLabels ? 'px-4' : 'justify-center px-2'}`}
          >
            <Brand showWord={showLabels} />
          </div>
          <NavLinks showLabels={showLabels} />
          <div className="mt-auto flex flex-col gap-3 border-t border-line p-3">
            <EncodingStatus variant={showLabels ? 'rail' : 'icon'} />
            <LogoutControl showName={showLabels} />
            <div
              className={`flex items-center ${
                showLabels ? 'justify-between gap-2' : 'flex-col gap-1'
              }`}
            >
              <VersionLink version={version} />
              <button
                type="button"
                onClick={toggleRail}
                aria-label={showLabels ? 'Collapse navigation' : 'Show labels'}
                aria-expanded={showLabels}
                className={btnIcon}
                data-testid="nav-toggle"
              >
                <Icon name={showLabels ? 'collapse' : 'expand'} />
              </button>
            </div>
          </div>
        </aside>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        {!isDesktop && (
          <header className="sticky top-0 z-topbar flex h-14 items-center gap-2 border-b border-line bg-surface/95 px-2 backdrop-blur">
            <button
              type="button"
              onClick={() => setDrawerOpen(true)}
              aria-label="Open menu"
              aria-expanded={drawerOpen}
              className={btnIcon}
              data-testid="menu-button"
            >
              <Icon name="menu" className="h-6 w-6 shrink-0" />
            </button>
            <div className="min-w-0 flex-1">
              <Brand showWord />
            </div>
            <EncodingStatus variant="compact" />
          </header>
        )}

        <main
          id="content"
          tabIndex={-1}
          className="min-w-0 flex-1 py-4 sm:py-6 lg:py-8"
        >
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
          className="m-0 flex h-dvh w-72 max-w-[85vw] flex-col rounded-r-overlay bg-surface text-fg shadow-overlay"
        >
          <div className="flex items-center justify-between gap-2 border-b border-line py-2 pl-4 pr-2">
            <Brand showWord />
            <button
              type="button"
              onClick={() => setDrawerOpen(false)}
              aria-label="Close menu"
              className={btnIcon}
              data-testid="close-menu"
            >
              <Icon name="close" className="h-6 w-6 shrink-0" />
            </button>
          </div>
          <NavLinks showLabels />
          <div className="mt-auto flex flex-col gap-3 border-t border-line p-3">
            <EncodingStatus variant="rail" />
            <LogoutControl showName />
            <VersionLink version={version} />
          </div>
        </ModalDialog>
      )}
    </div>
  )
}
