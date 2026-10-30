// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Auth, settings and update query hooks shared by the guard, the shell,
 * the pages and the dashboard. Kept out of the component files so fast
 * refresh stays happy.
 */
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getMe, getSetupStatus, getUpdateStatus, type SetupStatus } from './api'

export const setupStatusKey = ['setup-status'] as const
export const meKey = ['me'] as const
export const updateStatusKey = ['update-status'] as const

export function useSetupStatus() {
  return useQuery({ queryKey: setupStatusKey, queryFn: getSetupStatus })
}

export function useMe(enabled: boolean) {
  return useQuery({ queryKey: meKey, queryFn: getMe, enabled })
}

/**
 * The last update check (task 17). While an in-app apply runs, the status
 * is polled every two seconds so the About card's progress line follows
 * the update without its own SSE connection; the shell's update events
 * invalidate the same key.
 */
export function useUpdateStatus() {
  return useQuery({
    queryKey: updateStatusKey,
    queryFn: getUpdateStatus,
    refetchInterval: (query) =>
      query.state.data?.progress?.active ? 2000 : false,
  })
}

/** Marks the setup query complete without a refetch (after POST /setup). */
export function useMarkSetupComplete() {
  const client = useQueryClient()
  return (status: SetupStatus) => {
    client.setQueryData<SetupStatus>(setupStatusKey, status)
  }
}
