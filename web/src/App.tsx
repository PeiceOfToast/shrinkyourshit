// SPDX-License-Identifier: AGPL-3.0-only
import { Navigate, Route, Routes } from 'react-router-dom'
import AppShell from './AppShell'
import { RequireReady } from './guards'
import HomePage from './HomePage'
import LoginPage from './LoginPage'
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
          <Route index element={<HomePage />} />
          <Route
            path="libraries"
            element={
              <PlaceholderPage
                title="Libraries"
                message="Libraries arrive with the next update."
              />
            }
          />
          <Route
            path="nodes"
            element={
              <PlaceholderPage
                title="Nodes"
                message="Nodes arrive with the next update."
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
