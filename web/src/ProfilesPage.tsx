// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Encode profiles (task 72, spec 5.5). Two pieces:
 *
 * - `ProfilesSection`: the Profiles list on the Encode settings page. Name,
 *   tier, how many libraries use each profile and which, result policy and
 *   last edit. New makes a profile at the app's default tier; Duplicate copies
 *   one and opens the copy; a row opens the profile's editor.
 * - `ProfileEditorPage`: one profile's editor at /encode/profiles/:id. Its
 *   Basic form is here; the Moderate tree and the Advanced canvas are the
 *   shared flow editors, bound to the profile by `useFlowSubject`.
 *
 * Saves go through the server, which decides validity. A profile used by
 * libraries asks for confirmation first and names them (spec 5.3).
 */
import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createProfile,
  duplicateProfile,
  listProfiles,
  previewFlow,
  type EasyInput,
  type Profile,
  type ResultPolicy,
  type RuleTier,
} from './api'
import Badge from './Badge'
import Banner from './Banner'
import DataList, { type Column } from './DataList'
import ErrorBox from './ErrorBox'
import FlowPreviewPanel from './FlowPreviewPanel'
import {
  AudioSlider,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'
import { parseEasy } from './easyModel'
import { Field, FormActions, FormSection } from './FormSection'
import { formatDateTime } from './format'
import { useFlowSubject, type ProfileDetails } from './flowSubject'
import { ProfileSaveConfirm, ProfileTierSwitch } from './ProfileControls'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { isLocked, tierLabel } from './tierModel'
import { ReadOnlyReason, UnrepresentableBanner } from './TierNotices'
import { SkeletonRows } from './skeleton'
import { useDocumentTitle } from './useDocumentTitle'
import { useToast } from './toastContext'
import { btn, btnPrimary, input, select } from './uiControls'

/** Result policy wording on the editor (the same three the app settings offer). */
const POLICY_OPTIONS: Array<{ value: ResultPolicy; label: string }> = [
  {
    value: 'replace_if_smaller',
    label: 'Replace the original if the copy is smaller',
  },
  { value: 'replace', label: 'Always replace the original' },
  { value: 'keep_both', label: 'Keep both files' },
]

/** Short wording for the list. */
const POLICY_SHORT: Record<ResultPolicy, string> = {
  replace_if_smaller: 'Replace if smaller',
  replace: 'Always replace',
  keep_both: 'Keep both',
}

/** The first free name: "New profile", then "New profile (2)", and so on. */
function nextProfileName(existing: Profile[]): string {
  const taken = new Set(existing.map((p) => p.name))
  if (!taken.has('New profile')) return 'New profile'
  for (let n = 2; ; n++) {
    const candidate = `New profile (${n})`
    if (!taken.has(candidate)) return candidate
  }
}

/** "Used by 2 libraries: Movies, TV" or "Not used by any library yet". */
function usedByText(names: string[]): string {
  if (names.length === 0) return 'Not used by any library yet.'
  const count = names.length === 1 ? '1 library' : `${names.length} libraries`
  return `Used by ${count}: ${names.join(', ')}.`
}

/** The Profiles list on the Encode settings page. */
export function ProfilesSection({ defaultTier }: { defaultTier: RuleTier }) {
  const client = useQueryClient()
  const navigate = useNavigate()
  const toast = useToast()
  const list = useQuery({ queryKey: ['profiles'], queryFn: listProfiles })

  const create = useMutation({
    mutationFn: async () => {
      // A new profile starts from the basic sliders at their defaults. The
      // server compiles the graph, so the client never builds one.
      const preview = await previewFlow({ easy: parseEasy(null) })
      return createProfile({
        name: nextProfileName(list.data?.items ?? []),
        description: '',
        tier: defaultTier,
        result_policy: 'replace_if_smaller',
        graph: preview.graph,
      })
    },
    onSuccess: (created) => {
      void client.invalidateQueries({ queryKey: ['profiles'] })
      toast.success('Profile created.')
      navigate(`/encode/profiles/${created.id}`)
    },
    onError: (err: unknown) =>
      toast.failure('Creating the profile failed', err),
  })

  const duplicate = useMutation({
    mutationFn: (profileID: string) => duplicateProfile(profileID),
    onSuccess: (copy) => {
      void client.invalidateQueries({ queryKey: ['profiles'] })
      toast.success('Profile copied.')
      navigate(`/encode/profiles/${copy.id}`)
    },
    onError: (err: unknown) => toast.failure('Copying the profile failed', err),
  })

  const columns: Column<Profile>[] = [
    {
      key: 'name',
      header: 'Name',
      primary: true,
      cell: (p) => (
        <span
          className="font-semibold text-fg"
          data-testid={`profile-name-${p.id}`}
        >
          {p.name}
        </span>
      ),
    },
    {
      key: 'tier',
      header: 'Tier',
      cardPlacement: 'badge',
      cell: (p) => (
        <Badge tone="info" testId={`profile-tier-${p.id}`}>
          {tierLabel(p.tier)}
        </Badge>
      ),
    },
    {
      key: 'used',
      header: 'Used by',
      cell: (p) => (
        <span data-testid={`profile-used-${p.id}`}>
          {p.used_by === 0
            ? 'No libraries'
            : p.used_by === 1
              ? '1 library'
              : `${p.used_by} libraries`}
          {p.used_by_names.length > 0 && (
            <span className="block text-caption text-fg-muted">
              {p.used_by_names.join(', ')}
            </span>
          )}
        </span>
      ),
    },
    {
      key: 'policy',
      header: 'Result',
      cell: (p) => POLICY_SHORT[p.result_policy],
    },
    {
      key: 'edited',
      header: 'Last edited',
      cell: (p) => formatDateTime(p.updated_at),
    },
    {
      key: 'actions',
      header: 'Actions',
      actions: true,
      cell: (p) => (
        <button
          type="button"
          onClick={() => duplicate.mutate(p.id)}
          disabled={duplicate.isPending}
          className={btn}
          data-testid={`profile-duplicate-${p.id}`}
        >
          Duplicate
        </button>
      ),
    },
  ]

  return (
    <FormSection
      title="Profiles"
      description="A profile is a set of encode rules. A library picks one, and every library that uses a profile follows its changes."
      testId="profiles-section"
    >
      {list.isPending && <SkeletonRows rows={3} />}
      {list.isError && (
        <ErrorBox
          title="Cannot load the profiles."
          error={list.error}
          testId="profiles-error"
          onRetry={() => void list.refetch()}
        />
      )}
      {list.isSuccess && (
        <DataList
          rows={list.data.items}
          columns={columns}
          testid="profiles-table"
          cardsBelow="md"
          rowHref={(p) => `/encode/profiles/${p.id}`}
          rowTestId={(p) => `profile-row-${p.id}`}
          cardRowTestId={(p) => `profile-card-${p.id}`}
          empty={
            <p className="text-body text-fg-muted" data-testid="profiles-empty">
              No profiles yet.
            </p>
          }
        />
      )}
      <FormActions>
        <button
          type="button"
          onClick={() => create.mutate()}
          disabled={create.isPending || !list.isSuccess}
          className={btnPrimary}
          data-testid="profile-new"
        >
          {create.isPending ? 'Creating…' : 'New profile'}
        </button>
      </FormActions>
    </FormSection>
  )
}

/** One profile's editor: the Basic form, with the Moderate and Advanced editors one tab away. */
export default function ProfileEditorPage() {
  const { id = '' } = useParams()
  const toast = useToast()
  const subj = useFlowSubject({ kind: 'profile', id, lockTier: 'basic' })
  const [draft, setDraft] = useState<ProfileDetails | null>(null)
  const [easy, setEasy] = useState<EasyInput | null>(null)

  useDocumentTitle(['Encode settings', subj.name])

  const p = subj.profile
  const details: ProfileDetails | null =
    draft ??
    (p
      ? {
          name: p.name,
          description: p.description,
          result_policy: p.result_policy,
        }
      : null)
  const editedEasy = easy ?? parseEasy(null)

  const save = useMutation({
    mutationFn: async () => {
      if (!details) throw new Error('nothing to save')
      const preview = await previewFlow({ easy: editedEasy })
      return subj.save(preview.graph, 'basic', {
        ...details,
        name: details.name.trim(),
      })
    },
    onSuccess: () => {
      setDraft(null)
      toast.success('Profile saved.')
      subj.invalidate()
    },
    onError: (err: unknown) => toast.failure('Saving the profile failed', err),
  })

  if (subj.isPending) {
    return (
      <PageContainer className="space-y-6" data-testid="profile-loading">
        <PageHeader breadcrumb={[subj.root]} title="Profile" />
        <SkeletonRows rows={4} />
      </PageContainer>
    )
  }
  if (subj.isError || !p || !details) {
    return (
      <PageContainer className="space-y-6" data-testid="profile-page">
        <PageHeader breadcrumb={[subj.root]} title="Profile" />
        <ErrorBox
          title="Cannot load the profile."
          error={subj.error}
          onRetry={() => subj.refetch()}
        />
      </PageContainer>
    )
  }

  const basicLocked = isLocked(subj.lock)
  const nameValid = details.name.trim() !== ''
  const canSave = !basicLocked && nameValid && !save.isPending

  return (
    <PageContainer className="space-y-6 pb-8" data-testid="profile-page">
      <PageHeader
        breadcrumb={[subj.root]}
        title={p.name}
        meta={
          <span
            className="text-caption text-fg-muted"
            data-testid="profile-used-by"
          >
            {usedByText(p.used_by_names)}
          </span>
        }
      />

      <ProfileTierSwitch subject={subj} current="basic" />

      {p.unrepresentable.length > 0 && (
        <UnrepresentableBanner
          tier={p.tier}
          steps={p.unrepresentable}
          editHref={subj.paths.flow}
          noun="profile"
        />
      )}

      <FormSection title="Profile" testId="profile-details">
        <Field
          label="Name"
          htmlFor="profile-name"
          error={nameValid ? undefined : 'Give the profile a name.'}
        >
          <input
            id="profile-name"
            type="text"
            value={details.name}
            onChange={(e) => setDraft({ ...details, name: e.target.value })}
            className={`${input} max-w-md`}
            data-testid="profile-name-input"
          />
        </Field>
        <Field label="Description" htmlFor="profile-description">
          <input
            id="profile-description"
            type="text"
            value={details.description}
            onChange={(e) =>
              setDraft({ ...details, description: e.target.value })
            }
            className={`${input} max-w-md`}
            data-testid="profile-description-input"
          />
        </Field>
        <Field
          label="What happens to the original file"
          htmlFor="profile-policy"
        >
          <select
            id="profile-policy"
            value={details.result_policy}
            onChange={(e) =>
              setDraft({
                ...details,
                result_policy: e.target.value as ResultPolicy,
              })
            }
            className={`${select} max-w-md`}
            data-testid="profile-policy"
          >
            {POLICY_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </Field>
      </FormSection>

      <FormSection
        title="How much to shrink"
        description={`Basic editor. Saving it makes this profile ${tierLabel('basic')}.`}
        testId="profile-basic"
      >
        {basicLocked && (
          <ReadOnlyReason
            tier="basic"
            steps={subj.lock ?? []}
            editHref={subj.paths.flow}
            testId="profile-basic-locked"
          />
        )}
        <Banner tone="warning" testId="profile-basic-warning">
          Saving here replaces the steps this profile runs with the sliders
          below. A profile does not keep its slider values, so set them before
          you save.
        </Banner>
        {/* A disabled fieldset turns every slider and checkbox off at once. */}
        <fieldset disabled={basicLocked} className="contents">
          <QualitySlider
            value={editedEasy.quality}
            onChange={(quality) => setEasy({ ...editedEasy, quality })}
          />
          <RemoveSubtitlesCheckbox
            checked={editedEasy.remove_subtitles}
            onChange={(remove_subtitles) =>
              setEasy({ ...editedEasy, remove_subtitles })
            }
          />
          <AudioSlider
            value={editedEasy.audio}
            onChange={(audio) => setEasy({ ...editedEasy, audio })}
          />
        </fieldset>
      </FormSection>

      <FlowPreviewPanel easy={editedEasy} open />

      <FormActions>
        <button
          type="button"
          onClick={() => subj.requestSave(() => save.mutate())}
          disabled={!canSave}
          className={btnPrimary}
          data-testid="profile-save"
        >
          {save.isPending ? 'Saving…' : 'Save profile'}
        </button>
        <Link to="/encode" className={btn} data-testid="profile-back">
          Back to Encode settings
        </Link>
      </FormActions>

      <ProfileSaveConfirm pending={subj.pendingSave} />
    </PageContainer>
  )
}
