// SPDX-License-Identifier: AGPL-3.0-only
import { lazy, Suspense, useEffect } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import AppShell from './AppShell'
import DashboardPage from './DashboardPage'
import { RequireReady } from './guards'
import LibrariesPage from './LibrariesPage'
import LibraryDetailPage from './LibraryDetailPage'
import LibraryEditPage from './LibraryEditPage'
import LoginPage from './LoginPage'
import NewLibraryPage from './NewLibraryPage'
import NodesPage from './NodesPage'
import RulesPage from './RulesPage'
import EncodeSettingsPage from './EncodeSettingsPage'
import AppSettingsPage from './AppSettingsPage'
import SetupPage from './SetupPage'
import PageContainer from './PageContainer'
import { SkeletonRows } from './skeleton'

// The visual flow editor (task 15) carries the canvas library; it loads
// only when the route is opened so every other page stays light.
const FlowCanvasPage = lazy(() => import('./FlowCanvasPage'))

function FlowLoading() {
  return (
    <PageContainer className="space-y-6" data-testid="flow-loading">
      <h1 className="text-2xl font-bold">Flow</h1>
      <SkeletonRows rows={4} />
    </PageContainer>
  )
}

export default function App() {
  // Mirrors the index.css rule for html/body so the dark background is
  // guaranteed at runtime for every route (shell, setup, login) and is
  // assertable in tests without a stylesheet engine.
  useEffect(() => {
    document.documentElement.classList.add('bg-slate-950')
    document.body.classList.add('bg-slate-950')
  }, [])

  return (
    <Routes>
      {/* Setup and login live outside the shell. */}
      <Route path="/setup" element={<SetupPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route element={<RequireReady />}>
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="libraries" element={<LibrariesPage />} />
          <Route path="libraries/new" element={<NewLibraryPage />} />
          <Route path="libraries/:id" element={<LibraryDetailPage />} />
          <Route path="libraries/:id/edit" element={<LibraryEditPage />} />
          <Route path="libraries/:id/rules" element={<RulesPage />} />
          <Route
            path="libraries/:id/flow"
            element={
              <Suspense fallback={<FlowLoading />}>
                <FlowCanvasPage />
              </Suspense>
            }
          />
          <Route path="nodes" element={<NodesPage />} />
          <Route path="encode" element={<EncodeSettingsPage />} />
          <Route path="app" element={<AppSettingsPage />} />
          {/* Old bookmarks and links keep working: /settings is /app now. */}
          <Route path="settings" element={<Navigate to="/app" replace />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
