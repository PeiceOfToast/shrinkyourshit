// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen (task 07): pick a folder, move a slider, tick a box,
 * press Start. Everything else is a sensible default. No technical terms
 * on this screen — the Advanced panel on the edit page is the only place
 * codec names appear.
 */
import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { createLibrary, type Library } from './api'
import { initialEasyValue, toEasyInput, type EasyFormValue } from './easyModel'
import {
  AudioSlider,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'
import FolderPickerModal from './FolderPickerModal'

export default function EasyLibraryForm({
  initial,
  onSaved,
  submitLabel = 'Start',
}: {
  initial?: Partial<EasyFormValue>
  onSaved?: (lib: Library) => void
  submitLabel?: string
}) {
  const [value, setValue] = useState<EasyFormValue>({
    ...initialEasyValue,
    ...initial,
  })
  const [picking, setPicking] = useState(false)
  const set = (patch: Partial<EasyFormValue>) =>
    setValue((v) => ({ ...v, ...patch }))

  const create = useMutation({
    mutationFn: () =>
      createLibrary({
        path: value.path,
        easy: toEasyInput(value),
        watch: value.watch,
      }),
    onSuccess: (lib) => onSaved?.(lib),
  })

  return (
    <div className="space-y-6" data-testid="easy-form">
      <section className="grid gap-3 rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <span className="text-sm font-medium text-slate-300">Folder</span>
        {value.path ? (
          <p
            className="min-w-0 break-words rounded-lg bg-slate-800 px-3 py-2 text-sm"
            data-testid="easy-folder"
          >
            {value.path}
          </p>
        ) : (
          <p
            className="rounded-lg bg-slate-800 px-3 py-2 text-sm text-slate-500"
            data-testid="easy-folder-empty"
          >
            No folder chosen yet.
          </p>
        )}
        <button
          type="button"
          onClick={() => setPicking(true)}
          className="justify-self-start rounded-lg bg-slate-800 px-4 py-2 text-sm font-semibold hover:bg-slate-700"
          data-testid="easy-choose-folder"
        >
          {value.path ? 'Choose a different folder' : 'Choose a folder'}
        </button>
      </section>

      <section className="grid gap-6 rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <QualitySlider
          value={value.quality}
          onChange={(quality) => set({ quality })}
        />
        <RemoveSubtitlesCheckbox
          checked={value.remove_subtitles}
          onChange={(remove_subtitles) => set({ remove_subtitles })}
        />
        <AudioSlider value={value.audio} onChange={(audio) => set({ audio })} />
        <label className="flex items-center gap-3" data-testid="easy-watch">
          <input
            type="checkbox"
            checked={value.watch}
            onChange={(e) => set({ watch: e.target.checked })}
            className="h-4 w-4 accent-emerald-500"
            data-testid="easy-watch-checkbox"
          />
          <span>
            <span className="text-sm font-medium text-slate-300">
              Watch this folder for new files
            </span>
            <span className="block text-xs text-slate-500">
              New files are picked up automatically.
            </span>
          </span>
        </label>
      </section>

      {create.isError && (
        <p
          className="text-sm text-red-400"
          role="status"
          data-testid="easy-error"
        >
          {(create.error as Error).message}
        </p>
      )}

      <button
        type="button"
        onClick={() => create.mutate()}
        disabled={!value.path || create.isPending}
        className="rounded-lg bg-emerald-600 px-6 py-3 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
        data-testid="easy-start"
      >
        {create.isPending ? 'Starting…' : submitLabel}
      </button>

      {picking && (
        <FolderPickerModal
          initialPath={value.path || undefined}
          onClose={() => setPicking(false)}
          onPick={(path) => {
            set({ path })
            setPicking(false)
          }}
        />
      )}
    </div>
  )
}
