// SPDX-License-Identifier: AGPL-3.0-only

/**
 * New library page (task 07): the easy screen. A non-technical user picks a
 * folder, moves a slider, ticks a box and presses Start.
 */
import { useNavigate } from 'react-router-dom'
import EasyLibraryForm from './EasyLibraryForm'

export default function NewLibraryPage() {
  const navigate = useNavigate()
  return (
    <div className="max-w-2xl space-y-4" data-testid="new-library-page">
      <h1 className="text-2xl font-bold">Add a folder</h1>
      <p className="text-slate-400">
        Pick a folder of videos. ShrinkYourShit makes copies that need less
        space and swaps them in when they are smaller.
      </p>
      <EasyLibraryForm
        onSaved={(lib) => void navigate(`/libraries/${lib.id}`)}
      />
    </div>
  )
}
