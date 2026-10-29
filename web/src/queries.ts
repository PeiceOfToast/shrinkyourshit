// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Auth and settings query hooks shared by the guard, the shell and the
 * pages. Kept out of the component files so fast refresh stays happy.
 */
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getMe, getSetupStatus, type SetupStatus } from './api'

export const setupStatusKey = ['setup-status'] as const
export const meKey = ['me'] as const

export function useSetupStatus() {
  return useQuery({ queryKey: setupStatusKey, queryFn: getSetupStatus })
}

export function useMe(enabled: boolean) {
  return useQuery({ queryKey: meKey, queryFn: getMe, enabled })
}

/** Marks the setup query complete without a refetch (after POST /setup). */
export function useMarkSetupComplete() {
  const client = useQueryClient()
  return (status: SetupStatus) => {
    client.setQueryData<SetupStatus>(setupStatusKey, status)
  }
}
