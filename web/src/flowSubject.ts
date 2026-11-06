// SPDX-License-Identifier: AGPL-3.0-only

/**
 * What a flow editor edits (task 72, spec 5.5): a library, the v1.10 pages
 * that keep their URLs, or an encode profile, the Encode settings editors.
 * One hook gives the Moderate tree, the Advanced canvas and the profile's
 * Basic form the same queries, tier lock, save, save confirmation and route
 * paths, so each editor is written once.
 *
 * A profile saves tier `basic`, `moderate` or `advanced` with its graph. The
 * editor that saved last sets the tier, the same rule the library tier
 * follows in v1.10. Saving a profile used by libraries asks first and names
 * them (spec 5.3).
 */
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getLibrary,
  getLibraryFlow,
  getProfile,
  putLibraryFlow,
  updateProfile,
  type FlowFieldError,
  type FlowGraph,
  type Profile,
  type ResultPolicy,
  type RuleTier,
} from './api'
import type { Crumb } from './PageHeader'

export type SubjectKind = 'library' | 'profile'

/** The source a profile's tier reads as, so the editors' source checks stay the same. */
function sourceOfTier(tier: RuleTier): string {
  if (tier === 'basic') return 'easy'
  if (tier === 'moderate') return 'steps'
  return 'advanced'
}

/** The fields a profile save carries besides the graph. */
export interface ProfileDetails {
  name: string
  description: string
  result_policy: ResultPolicy
}

export interface FlowSubject {
  kind: SubjectKind
  id: string
  /** The name shown in headers and titles, once loaded. */
  name: string | undefined
  /** The profile, for profile subjects. */
  profile: Profile | undefined
  /** The stored graph and the source the editors read (`easy`, `steps`, `advanced`). */
  flow: { graph: FlowGraph; source: string } | undefined
  /** Steps the requested lock tier cannot express. Empty or undefined when unlocked. */
  lock: FlowFieldError[] | undefined
  /** Route paths: the overview, the Basic editor, the tree and the canvas. */
  paths: { overview: string; basic: string; tree: string; flow: string }
  /** The first breadcrumb: Libraries, or Encode settings. */
  root: Crumb
  isPending: boolean
  isError: boolean
  error: unknown
  refetch: () => void
  /** Saves the graph at `tier` and returns the stored graph. */
  save: (
    graph: FlowGraph,
    tier: RuleTier,
    details?: ProfileDetails,
  ) => Promise<FlowGraph>
  /** Runs `run` now, or after the save confirmation when a profile is in use. */
  requestSave: (run: () => void) => void
  /** A save waiting on the confirmation: the libraries it changes, and the answers. */
  pendingSave: {
    users: string[]
    confirm: () => void
    cancel: () => void
  } | null
  /** Refreshes every query this subject feeds. */
  invalidate: () => void
}

export function useFlowSubject({
  kind,
  id,
  lockTier,
}: {
  kind: SubjectKind
  id: string
  /** Asks the server which steps this tier cannot express. Omit for no lock. */
  lockTier?: RuleTier
}): FlowSubject {
  const client = useQueryClient()
  const [pending, setPending] = useState<{ run: () => void } | null>(null)
  const isLibrary = kind === 'library'

  const library = useQuery({
    queryKey: ['library', id],
    queryFn: () => getLibrary(id),
    enabled: isLibrary,
  })
  const libraryFlow = useQuery({
    queryKey: ['library-flow', id],
    queryFn: () => getLibraryFlow(id),
    enabled: isLibrary,
  })
  const profileQuery = useQuery({
    queryKey: ['profile', id],
    queryFn: () => getProfile(id),
    enabled: !isLibrary,
  })
  const libraryLock = useQuery({
    queryKey: ['library-flow', id, lockTier],
    queryFn: () => getLibraryFlow(id, lockTier),
    enabled: isLibrary && lockTier !== undefined,
  })
  const profileLock = useQuery({
    queryKey: ['profile', id, lockTier],
    queryFn: () => getProfile(id, lockTier),
    enabled: !isLibrary && lockTier !== undefined,
  })

  const profile = profileQuery.data
  const lockQuery = isLibrary ? libraryLock : profileLock
  const lockPending = lockTier !== undefined && lockQuery.isPending
  const lockError = lockTier !== undefined && lockQuery.isError

  const flow = isLibrary
    ? libraryFlow.data
      ? { graph: libraryFlow.data.graph, source: libraryFlow.data.source }
      : undefined
    : profile
      ? { graph: profile.graph, source: sourceOfTier(profile.tier) }
      : undefined

  const lock =
    lockTier === undefined
      ? undefined
      : isLibrary
        ? libraryLock.data?.unrepresentable
        : profileLock.data?.unrepresentable

  const paths = isLibrary
    ? {
        overview: `/libraries/${id}`,
        basic: `/libraries/${id}/rules`,
        tree: `/libraries/${id}/tree`,
        flow: `/libraries/${id}/flow`,
      }
    : {
        overview: `/encode/profiles/${id}`,
        basic: `/encode/profiles/${id}`,
        tree: `/encode/profiles/${id}/tree`,
        flow: `/encode/profiles/${id}/flow`,
      }

  const save = (
    graph: FlowGraph,
    tier: RuleTier,
    details?: ProfileDetails,
  ): Promise<FlowGraph> => {
    if (isLibrary) {
      return putLibraryFlow(id, {
        source: tier === 'advanced' ? 'advanced' : 'steps',
        graph,
      }).then((resp) => resp.graph)
    }
    if (!profile) return Promise.reject(new Error('the profile is not loaded'))
    const d: ProfileDetails = details ?? {
      name: profile.name,
      description: profile.description,
      result_policy: profile.result_policy,
    }
    return updateProfile(id, { ...d, tier, graph }).then((resp) => resp.graph)
  }

  const invalidate = () => {
    if (isLibrary) {
      void client.invalidateQueries({ queryKey: ['library', id] })
      void client.invalidateQueries({ queryKey: ['library-flow', id] })
      void client.invalidateQueries({ queryKey: ['libraries'] })
    } else {
      void client.invalidateQueries({ queryKey: ['profile', id] })
      void client.invalidateQueries({ queryKey: ['profiles'] })
    }
  }

  const users = profile?.used_by_names ?? []
  const requestSave = (run: () => void) => {
    if (!isLibrary && users.length > 0) {
      setPending({ run })
      return
    }
    run()
  }

  const pendingSave = pending
    ? {
        users,
        confirm: () => {
          const run = pending.run
          setPending(null)
          run()
        },
        cancel: () => setPending(null),
      }
    : null

  return {
    kind,
    id,
    name: isLibrary ? library.data?.name : profile?.name,
    profile,
    flow,
    lock,
    paths,
    root: isLibrary
      ? { label: 'Libraries', to: '/libraries' }
      : { label: 'Encode settings', to: '/encode' },
    isPending: isLibrary
      ? library.isPending || libraryFlow.isPending || lockPending
      : profileQuery.isPending || lockPending,
    isError: isLibrary
      ? library.isError || libraryFlow.isError || lockError
      : profileQuery.isError || lockError,
    error: isLibrary
      ? (library.error ?? libraryFlow.error ?? lockQuery.error)
      : (profileQuery.error ?? lockQuery.error),
    refetch: () => {
      if (isLibrary) {
        void library.refetch()
        void libraryFlow.refetch()
      } else {
        void profileQuery.refetch()
      }
      if (lockTier !== undefined) void lockQuery.refetch()
    },
    save,
    requestSave,
    pendingSave,
    invalidate,
  }
}
