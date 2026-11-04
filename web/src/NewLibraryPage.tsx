// SPDX-License-Identifier: AGPL-3.0-only

/**
 * New library page (task 07, restyled in task 41): the easy screen. A
 * non-technical user picks a folder, moves a slider, ticks a box and presses
 * "Add library".
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useNavigate } from 'react-router-dom'
import EasyLibraryForm from './EasyLibraryForm'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'

export default function NewLibraryPage() {
  useDocumentTitle(['Add a library'])
  const navigate = useNavigate()
  return (
    <PageContainer data-testid="new-library-page">
      <PageHeader
        breadcrumb={[{ label: 'Libraries', to: '/libraries' }]}
        title="Add a library"
        subtitle="Pick a folder of videos. ShrinkYourShit makes copies that need less space and swaps them in when they are smaller."
      />
      <EasyLibraryForm
        onSaved={(lib) => void navigate(`/libraries/${lib.id}`)}
        onCancel={() => void navigate('/libraries')}
      />
    </PageContainer>
  )
}
