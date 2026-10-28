// SPDX-License-Identifier: AGPL-3.0-only
import { Route, Routes } from 'react-router-dom'
import HomePage from './HomePage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<HomePage />} />
      {/* Unknown paths still get the shell; real routes arrive with later tasks. */}
      <Route path="*" element={<HomePage />} />
    </Routes>
  )
}
