// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen (task 07, restyled in task 41): pick a folder, move a
 * slider, tick a box, press "Add library". Everything else is a sensible
 * default. No technical terms on this screen — the Exact settings panel on
 * the edit page is the only place codec names appear.
 */
import { useId, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { createLibrary, type Library } from './api'
import Checkbox from './Checkbox'
import { initialEasyValue, toEasyInput, type EasyFormValue } from './easyModel'
import ErrorBox from './ErrorBox'
import {
  AudioSlider,
  pathField,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'
import { FormActions, FormSection } from './FormSection'
import FolderPickerModal from './FolderPickerModal'
import FlowPreviewPanel from './FlowPreviewPanel'
import { btn, btnGhost, btnPrimary } from './uiControls'
import { useToast } from './toastContext'

export default function EasyLibraryForm({
  initial,
  onSaved,
  onCancel,
  submitLabel = 'Add library',
}: {
  initial?: Partial<EasyFormValue>
  onSaved?: (lib: Library) => void
  /** When given, the action bar shows a Cancel button. */
  onCancel?: () => void
  submitLabel?: string
}) {
  const toast = useToast()
  const helpId = useId()
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
    onSuccess: (lib) => {
      toast.success('Folder added. The first scan is running.')
      onSaved?.(lib)
    },
    onError: (err: unknown) => toast.failure('Adding the folder failed', err),
  })

  const hasPath = Boolean(value.path)

  return (
    <div className="flex flex-col gap-6" data-testid="easy-form">
      <FormSection
        title="Folder"
        description="The folder to watch. Subfolders are included."
      >
        <div className="flex gap-2">
          {hasPath ? (
            <p
              className={`${pathField} text-fg`}
              title={value.path}
              data-testid="easy-folder"
            >
              {value.path}
            </p>
          ) : (
            <p
              className={`${pathField} text-fg-muted`}
              data-testid="easy-folder-empty"
            >
              No folder chosen yet
            </p>
          )}
          <button
            type="button"
            onClick={() => setPicking(true)}
            className={`${btn} shrink-0`}
            data-testid="easy-choose-folder"
          >
            Choose…
          </button>
        </div>
      </FormSection>

      <FormSection
        title="How much to shrink"
        description="You can change this later in Rules."
      >
        <QualitySlider
          value={value.quality}
          onChange={(quality) => set({ quality })}
        />
        <AudioSlider value={value.audio} onChange={(audio) => set({ audio })} />
        <RemoveSubtitlesCheckbox
          checked={value.remove_subtitles}
          onChange={(remove_subtitles) => set({ remove_subtitles })}
        />
      </FormSection>

      {/* The generated rule in plain language (task 14): one line per
          step, the bridge that teaches the step stack's vocabulary. */}
      <FlowPreviewPanel
        easy={{
          quality: value.quality,
          remove_subtitles: value.remove_subtitles,
          audio: value.audio,
        }}
      />

      <FormSection title="Watching">
        <div data-testid="easy-watch">
          <Checkbox
            checked={value.watch}
            onChange={(watch) => set({ watch })}
            label="Watch this folder for new files"
            help="New files are picked up automatically."
            testId="easy-watch-checkbox"
          />
        </div>
      </FormSection>

      {create.isError && (
        <ErrorBox
          title="Adding the folder failed."
          error={create.error}
          testId="easy-error"
        />
      )}

      <FormActions sticky>
        <button
          type="button"
          onClick={() => create.mutate()}
          disabled={!hasPath || create.isPending}
          aria-describedby={hasPath ? undefined : helpId}
          className={btnPrimary}
          data-testid="easy-start"
        >
          {create.isPending ? 'Adding…' : submitLabel}
        </button>
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className={btnGhost}
            data-testid="easy-cancel"
          >
            Cancel
          </button>
        )}
        {!hasPath && (
          <p id={helpId} className="basis-full text-caption text-fg-muted">
            Pick a folder first.
          </p>
        )}
      </FormActions>

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
