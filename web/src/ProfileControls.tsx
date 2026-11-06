// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Controls shared by the profile editors (task 72): the Basic, Moderate and
 * Advanced tier switch, and the save confirmation a profile in use needs.
 */
import Badge from './Badge'
import ConfirmDialog from './ConfirmDialog'
import SegmentedControl from './SegmentedControl'
import type { FlowSubject } from './flowSubject'
import type { RuleTier } from './api'
import { tierLabel } from './tierModel'

/**
 * The tier control of a profile's editors: Basic, Moderate and Advanced, each
 * a route. The profile's own tier carries "In use".
 */
export function ProfileTierSwitch({
  subject,
  current,
}: {
  subject: Pick<FlowSubject, 'paths' | 'profile'>
  current: RuleTier
}) {
  const inUse = subject.profile?.tier
  const tiers: Array<{ id: RuleTier; to: string }> = [
    { id: 'basic', to: subject.paths.basic },
    { id: 'moderate', to: subject.paths.tree },
    { id: 'advanced', to: subject.paths.flow },
  ]
  return (
    <div className="flex flex-col gap-2" data-testid="tier-selector">
      <SegmentedControl
        label="Rule editor"
        value={current}
        options={tiers.map((t) => ({
          id: t.id,
          label: tierLabel(t.id),
          testId: `tier-${t.id}`,
          to: t.to,
          badge:
            inUse === t.id ? (
              <Badge tone="success" testId={`tier-${t.id}-in-use`}>
                In use
              </Badge>
            ) : undefined,
        }))}
      />
    </div>
  )
}

/** The confirmation a save of a profile in use waits on. Renders nothing when no save is pending. */
export function ProfileSaveConfirm({
  pending,
}: {
  pending: FlowSubject['pendingSave']
}) {
  if (!pending) return null
  const count = pending.users.length
  return (
    <ConfirmDialog
      title="Save this profile?"
      confirmLabel="Save profile"
      danger={false}
      testId="profile-save-dialog"
      confirmTestId="profile-save-confirm"
      cancelTestId="profile-save-cancel"
      onConfirm={pending.confirm}
      onClose={pending.cancel}
    >
      <p>
        {count === 1
          ? 'One library uses this profile.'
          : `${count} libraries use this profile.`}{' '}
        The change applies to every one of them:
      </p>
      <ul
        className="list-disc space-y-1 pl-5"
        data-testid="profile-save-libraries"
      >
        {pending.users.map((name) => (
          <li key={name}>{name}</li>
        ))}
      </ul>
    </ConfirmDialog>
  )
}
