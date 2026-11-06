// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The old library rule routes (`/libraries/:id/rules`, `/tree`, `/flow`),
 * kept so bookmarks still work (task 73, spec 5.5). A library no longer holds
 * steps, so these send the user to the editor of the profile the library
 * follows. A library with no profile goes to its edit page to choose one.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useQuery } from '@tanstack/react-query'
import { Navigate, useParams } from 'react-router-dom'
import { getLibrary } from './api'
import PageContainer from './PageContainer'
import { SkeletonRows } from './skeleton'

export default function LibraryRulesRedirect({
  to = '',
}: {
  /** The editor suffix on the profile route: '', 'tree' or 'flow'. */
  to?: '' | 'tree' | 'flow'
}) {
  const { id = '' } = useParams()
  const library = useQuery({
    queryKey: ['library', id],
    queryFn: () => getLibrary(id),
  })
  useDocumentTitle(['Rules'])

  if (library.isPending) {
    return (
      <PageContainer data-testid="library-rules-loading">
        <SkeletonRows rows={3} />
      </PageContainer>
    )
  }
  if (library.isError || !library.data) {
    return <Navigate to="/libraries" replace />
  }
  const profileId = library.data.profile_id ?? ''
  if (!profileId) {
    return <Navigate to={`/libraries/${id}/edit`} replace />
  }
  const suffix = to ? `/${to}` : ''
  return <Navigate to={`/encode/profiles/${profileId}${suffix}`} replace />
}
