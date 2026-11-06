// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The encode profile choice on the library pages (task 73, spec 5.5). A
 * library points at one profile. ProfileSelect is the dropdown; ProfileSummary
 * says in plain language what the chosen profile does. The lines come from
 * POST /flows/preview, the same server rendering the rest of the app uses, so
 * this file holds no second copy of the step wording.
 */
import { useQuery } from '@tanstack/react-query'
import { previewFlow, type Profile } from './api'
import { tierLabel } from './tierModel'
import { select } from './uiControls'

/**
 * A dropdown of profiles. An empty value shows a placeholder the user must
 * replace before saving. The option text names the editor each profile is
 * kept in, so a Basic and an Advanced profile can be told apart.
 */
export function ProfileSelect({
  id,
  value,
  onChange,
  profiles,
  testId = 'library-profile-select',
}: {
  id: string
  value: string
  onChange: (profileId: string) => void
  profiles: Profile[]
  testId?: string
}) {
  return (
    <div className="flex flex-col gap-2">
      <label htmlFor={id} className="text-body font-medium text-fg">
        Encode profile
      </label>
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={select}
        data-testid={testId}
      >
        {value === '' && (
          <option value="" disabled>
            Choose a profile
          </option>
        )}
        {profiles.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name} · {tierLabel(p.tier)}
          </option>
        ))}
      </select>
    </div>
  )
}

/**
 * The chosen profile's steps, one plain line each. Keyed on the profile's
 * update time, so a saved profile re-renders its own summary.
 */
export function ProfileSummary({
  profile,
  testId = 'profile-summary',
}: {
  profile: Profile
  testId?: string
}) {
  const plain = useQuery({
    queryKey: ['profile-plain', profile.id, profile.updated_at],
    queryFn: () => previewFlow({ graph: profile.graph }),
  })
  const steps = plain.data?.steps ?? []
  return (
    <div className="flex flex-col gap-2" data-testid={testId}>
      {plain.isPending && (
        <p className="text-caption text-fg-muted">Reading the profile…</p>
      )}
      {plain.isError && (
        <p className="text-caption text-danger">
          The steps of this profile could not be read.
        </p>
      )}
      {steps.length > 0 && (
        <ul
          className="list-disc space-y-1 pl-5 text-body text-fg"
          data-testid="profile-summary-lines"
        >
          {steps.map((s) => (
            <li key={s.id} data-testid="profile-summary-line">
              {s.plain}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
