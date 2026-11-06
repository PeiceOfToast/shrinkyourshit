// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Encode settings (task 10, restyled task 49, spec 5.11): the library
 * defaults — preferred languages, the default video format for new libraries
 * and what happens to the original file by default — then the Intel Quick
 * Sync extras. Split out of the old SettingsPage; the server-level settings
 * live at /app.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getNodes,
  getSettings,
  updateSettings,
  type RuleTier,
  type SettingsUpdate,
} from './api'
import { TIER_OPTIONS, tierLabel } from './tierModel'
import { ProfilesSection } from './ProfilesPage'
import Banner from './Banner'
import ErrorBox from './ErrorBox'
import { Field, FormActions, FormSection } from './FormSection'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { buildUpdate, toForm, type FormState } from './settingsModel'
import { SkeletonCard } from './skeleton'
import Switch from './Switch'
import { useToast } from './toastContext'
import { btnPrimary, input, select } from './uiControls'

const policies = [
  {
    value: 'replace_if_smaller',
    label: 'Replace the original if the copy is smaller',
  },
  { value: 'replace', label: 'Always replace the original' },
  { value: 'keep_both', label: 'Keep both files' },
]

const codecs = [
  { value: 'hevc', label: 'HEVC — plays almost everywhere (recommended)' },
  { value: 'av1', label: 'AV1 — smallest files, newer devices only' },
  { value: 'h264', label: 'H.264 — for very old devices' },
]

/** One on/off setting: the switch, then its plain-language help. */
function SwitchRow({
  name,
  ariaLabel,
  help,
  checked,
  onChange,
  testId,
  disabled,
}: {
  name: string
  /** The accessible name; defaults to the visible name. */
  ariaLabel?: string
  help: string
  checked: boolean
  onChange: (on: boolean) => void
  testId: string
  disabled: boolean
}) {
  return (
    <div className="flex flex-col">
      <Switch
        checked={checked}
        onChange={onChange}
        label={ariaLabel ?? name}
        visibleLabel={name}
        testId={testId}
        disabled={disabled}
      />
      {/* 44 px track plus the 12 px gap: the help lines up under the name. */}
      <p className="pl-14 text-caption text-fg-muted">{help}</p>
    </div>
  )
}

export default function EncodeSettingsPage() {
  useDocumentTitle(['Encode settings'])
  const query = useQuery({ queryKey: ['settings'], queryFn: getSettings })
  const nodes = useQuery({ queryKey: ['nodes'], queryFn: getNodes })
  const client = useQueryClient()
  const toast = useToast()
  const [form, setForm] = useState<FormState | null>(null)

  useEffect(() => {
    if (query.data && form === null) {
      setForm(toForm(query.data))
    }
  }, [query.data, form])

  const save = useMutation({
    mutationFn: (body: SettingsUpdate) => updateSettings(body),
    onSuccess: (updated) => {
      client.setQueryData(['settings'], updated)
      setForm(toForm(updated))
      toast.success('Settings saved.')
    },
    onError: (err: unknown) => toast.failure('Saving settings failed', err),
  })

  const header = (
    <PageHeader
      title="Encode settings"
      subtitle="Defaults for new libraries, and the encode profiles that libraries pick from."
    />
  )

  if (query.isPending) {
    return (
      <PageContainer data-testid="encode-settings-page">
        {header}
        <SkeletonCard className="h-64" />
      </PageContainer>
    )
  }
  if (query.isError || !query.data) {
    return (
      <PageContainer data-testid="encode-settings-page">
        {header}
        <ErrorBox
          title="Cannot load the settings."
          error={query.error}
          onRetry={() => void query.refetch()}
        />
      </PageContainer>
    )
  }
  if (form === null) {
    return (
      <PageContainer data-testid="encode-settings-page">
        {header}
        <SkeletonCard className="h-64" />
      </PageContainer>
    )
  }

  const set = (patch: Partial<FormState>) => setForm({ ...form, ...patch })

  // Validation before submit (task 62): the tier must be one the server
  // knows, and Save stays off until it is. The select only offers valid
  // values, so this guards a stale or hand-edited form.
  const tierValid = TIER_OPTIONS.some((t) => t.id === form.default_rule_tier)
  const savePlain = () => {
    if (!tierValid) return
    save.mutate(buildUpdate(form, {}))
  }

  // QSV is only useful on a node that reports it. Until the nodes load,
  // say nothing rather than guess.
  const anyNodeHasQSV = (nodes.data?.items ?? []).some((n) =>
    (n.capabilities?.encoders ?? []).some((e) => e.name === 'qsv'),
  )
  const noNodeHasQSV = nodes.isSuccess && !anyNodeHasQSV

  return (
    <PageContainer data-testid="encode-settings-page">
      {header}

      <FormSection title="Library defaults">
        <Field
          label="Keep my languages"
          htmlFor="preferred-languages"
          help="Three-letter language codes, comma separated (for example eng, deu)."
        >
          <input
            id="preferred-languages"
            type="text"
            value={form.preferred_languages}
            onChange={(e) => set({ preferred_languages: e.target.value })}
            className={`${input} max-w-xs`}
            data-testid="preferred-languages"
          />
        </Field>
        <Field
          label="Video format for new libraries"
          htmlFor="default-video-codec"
        >
          <select
            id="default-video-codec"
            value={form.default_video_codec}
            onChange={(e) => set({ default_video_codec: e.target.value })}
            className={select}
            data-testid="default-video-codec"
          >
            {codecs.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </Field>
        <Field
          label="What happens to the original file"
          htmlFor="default-result-policy"
        >
          <select
            id="default-result-policy"
            value={form.default_result_policy}
            onChange={(e) => set({ default_result_policy: e.target.value })}
            className={select}
            data-testid="default-result-policy"
          >
            {policies.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </Field>
      </FormSection>

      <FormSection
        title="Rule tiers"
        description="How new libraries are described, and whether raw ffmpeg arguments may run."
        testId="rule-tiers-section"
      >
        <Field
          label="Editor for new libraries"
          htmlFor="default-rule-tier"
          help="Basic is the sliders, Moderate is the rule tree, Advanced is the visual graph. A library keeps its own choice in its Rules."
          error={tierValid ? undefined : 'Pick Basic, Moderate or Advanced.'}
        >
          <select
            id="default-rule-tier"
            value={form.default_rule_tier}
            onChange={(e) => set({ default_rule_tier: e.target.value })}
            className={select}
            data-testid="default-rule-tier"
          >
            {TIER_OPTIONS.map((t) => (
              <option key={t.id} value={t.id}>
                {tierLabel(t.id)}
              </option>
            ))}
          </select>
        </Field>
        <SwitchRow
          name="Allow raw ffmpeg arguments"
          help="Lets a graph run the raw arguments step, which passes words the app does not model. Turn it off and flows that hold that step are refused. On by default."
          checked={query.data.allow_raw_args ?? true}
          onChange={(on) => save.mutate({ allow_raw_args: on })}
          testId="allow-raw-args"
          disabled={save.isPending}
        />
      </FormSection>

      <FormSection
        title="Intel Quick Sync extras"
        description="Only used on machines where the hardware check found Quick Sync. Off changes nothing."
      >
        {noNodeHasQSV && (
          <Banner tone="info" testId="qsv-no-nodes">
            None of your nodes has Intel Quick Sync, so these do nothing today.
          </Banner>
        )}
        <SwitchRow
          name="Extra quality"
          ariaLabel="Extra quality on Intel Quick Sync"
          help="Uses look-ahead and extended bitrate control where the machine supports both. A little slower, slightly better files. Does nothing on machines without the feature."
          checked={query.data.qsv_extra_quality}
          onChange={(on) => save.mutate({ qsv_extra_quality: on })}
          testId="qsv-extra-quality"
          disabled={save.isPending}
        />
        <SwitchRow
          name="Low-power mode"
          ariaLabel="Low-power mode on Intel Quick Sync"
          help="Faster and slightly worse, and it draws less power. Does nothing on older hardware."
          checked={query.data.qsv_low_power}
          onChange={(on) => save.mutate({ qsv_low_power: on })}
          testId="qsv-low-power"
          disabled={save.isPending}
        />
      </FormSection>

      {save.isError && (
        <ErrorBox
          title="Saving the settings failed."
          error={save.error}
          testId="settings-error"
        />
      )}
      <FormActions>
        <button
          type="button"
          onClick={savePlain}
          disabled={save.isPending || !tierValid}
          className={btnPrimary}
          data-testid="save-settings"
        >
          {save.isPending ? 'Saving…' : 'Save changes'}
        </button>
      </FormActions>

      {/* A new profile takes the saved default tier (task 72). */}
      <ProfilesSection
        defaultTier={
          TIER_OPTIONS.some((t) => t.id === query.data.default_rule_tier)
            ? (query.data.default_rule_tier as RuleTier)
            : 'basic'
        }
      />
    </PageContainer>
  )
}
