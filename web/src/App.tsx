// SPDX-License-Identifier: AGPL-3.0-only
import { Navigate, Route, Routes } from 'react-router-dom'
import AppShell from './AppShell'
import DashboardPage from './DashboardPage'
import { RequireReady } from './guards'
import LibrariesPage from './LibrariesPage'
import LibraryDetailPage from './LibraryDetailPage'
import LibraryEditPage from './LibraryEditPage'
import LoginPage from './LoginPage'
import NewLibraryPage from './NewLibraryPage'
import PlaceholderPage from './PlaceholderPage'
import SettingsPage from './SettingsPage'
import SetupPage from './SetupPage'

export default function App() {
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
          <Route
            path="nodes"
            element={
              <PlaceholderPage
                title="Nodes"
                message="Other machines that help out arrive with the next update."
              />
            }
          />
          <Route path="settings" element={<SettingsPage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
