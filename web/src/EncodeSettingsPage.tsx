// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Encode settings (task 10): the library defaults — preferred languages,
 * the default video format for new libraries and what happens to the
 * original file by default. Split out of the old SettingsPage; the
 * server-level settings live at /app.
 */
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getSettings, updateSettings, type SettingsUpdate } from './api'
import ErrorBox from './ErrorBox'
import PageContainer from './PageContainer'
import { buildUpdate, toForm, type FormState } from './settingsModel'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'

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

export default function EncodeSettingsPage() {
  const query = useQuery({ queryKey: ['settings'], queryFn: getSettings })
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

  if (query.isPending) {
    return (
      <PageContainer className="space-y-6" data-testid="encode-settings-page">
        <h1 className="text-2xl font-bold">Encode settings</h1>
        <SkeletonRows rows={3} />
      </PageContainer>
    )
  }
  if (query.isError || !query.data) {
    return (
      <PageContainer className="space-y-6" data-testid="encode-settings-page">
        <h1 className="text-2xl font-bold">Encode settings</h1>
        <ErrorBox title="Cannot load the settings." error={query.error} />
      </PageContainer>
    )
  }
  if (form === null) {
    return (
      <PageContainer className="space-y-6" data-testid="encode-settings-page">
        <h1 className="text-2xl font-bold">Encode settings</h1>
        <SkeletonRows rows={3} />
      </PageContainer>
    )
  }

  const set = (patch: Partial<FormState>) => setForm({ ...form, ...patch })

  const savePlain = () => save.mutate(buildUpdate(form, {}))

  return (
    <PageContainer className="space-y-6" data-testid="encode-settings-page">
      <h1 className="text-2xl font-bold">Encode settings</h1>
      {save.isError && (
        <ErrorBox
          title="Saving the settings failed."
          error={save.error}
          testId="settings-error"
        />
      )}

      <section className="rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <h2 className="font-semibold">Library defaults</h2>
        <label className="mt-4 grid gap-1">
          <span className="text-sm text-slate-300">Keep my languages</span>
          <input
            type="text"
            value={form.preferred_languages}
            onChange={(e) => set({ preferred_languages: e.target.value })}
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
            data-testid="preferred-languages"
          />
          <span className="text-xs text-slate-500">
            Two-letter codes, comma separated (for example eng, deu).
          </span>
        </label>
        <label className="mt-3 grid gap-1">
          <span className="text-sm text-slate-300">
            Video format for new libraries
          </span>
          <select
            value={form.default_video_codec}
            onChange={(e) => set({ default_video_codec: e.target.value })}
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
            data-testid="default-video-codec"
          >
            {codecs.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </label>
        <label className="mt-3 grid gap-1">
          <span className="text-sm text-slate-300">
            What happens to the original file
          </span>
          <select
            value={form.default_result_policy}
            onChange={(e) => set({ default_result_policy: e.target.value })}
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
            data-testid="default-result-policy"
          >
            {policies.map((p) => (
              <option key={p.value} value={p.value}>
                {p.label}
              </option>
            ))}
          </select>
        </label>
      </section>

      <button
        type="button"
        onClick={savePlain}
        disabled={save.isPending}
        className="rounded-lg bg-emerald-600 px-4 py-2 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
        data-testid="save-settings"
      >
        {save.isPending ? 'Saving…' : 'Save settings'}
      </button>
    </PageContainer>
  )
}
