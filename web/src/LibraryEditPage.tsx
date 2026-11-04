// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library edit page (task 07, restyled in task 41). When the library was
 * produced by the easy screen its sliders reopen; a collapsed "Exact
 * settings" panel exposes the raw Target fields (codec,
 * reencode_if_same_codec, container, languages) — the only place in the UI
 * where codec names appear. Saving in exact mode sends a full target (which
 * clears the easy state server-side); saving in easy mode sends the easy
 * object. The option labels are plain language; the values are unchanged.
 *
 * Interim (task 41, spec 5.7): both panels stay. Task 47 removes the exact
 * panel once the owner signs off.
 */
import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate, useParams } from 'react-router-dom'
import { getLibrary, updateLibrary, type LibraryBody, type Target } from './api'
import Checkbox from './Checkbox'
import {
  AudioSlider,
  pathField,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'
import { parseEasy } from './easyModel'
import ErrorBox from './ErrorBox'
import { Field, FormActions, FormSection } from './FormSection'
import FolderPickerModal from './FolderPickerModal'
import KindPicker from './KindPicker'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { btn, btnGhost, btnPrimary, input, select } from './uiControls'

const CODEC_CHOICES = [
  { value: 'hevc', label: 'HEVC (H.265) — recommended' },
  { value: 'av1', label: 'AV1 — smallest files, newer devices only' },
  { value: 'h264', label: 'H.264 — for very old devices' },
  { value: 'copy', label: 'Copy — never re-encode the video' },
]

/** Human labels for the stored values (the values themselves are unchanged). */
const CONTAINER_CHOICES = [
  { value: 'mkv', label: 'MKV' },
  { value: 'mp4', label: 'MP4' },
]
const AUDIO_MODE_CHOICES = [
  { value: 'keep_all', label: 'Keep every track' },
  { value: 'keep_preferred', label: 'Keep my languages' },
  { value: 'keep_best', label: 'Keep the best track' },
  { value: 'compact', label: 'Keep the best track, make it small' },
]
const SUBTITLE_MODE_CHOICES = [
  { value: 'keep_all', label: 'Keep every track' },
  { value: 'keep_preferred', label: 'Keep my languages' },
  { value: 'remove', label: 'Remove all' },
]

function defaultTarget(lib: { target: Target }): Target {
  return {
    video: {
      codec: lib.target.video.codec || 'hevc',
      quality: lib.target.video.quality || 3,
      reencode_if_same_codec: Boolean(lib.target.video.reencode_if_same_codec),
    },
    audio: {
      mode: lib.target.audio.mode || 'keep_all',
      preferred_languages: lib.target.audio.preferred_languages ?? ['eng'],
      compact_codec: lib.target.audio.compact_codec || 'aac',
    },
    subtitles: {
      mode: lib.target.subtitles.mode || 'keep_all',
      preferred_languages: lib.target.subtitles.preferred_languages ?? ['eng'],
      extract_removed: Boolean(lib.target.subtitles.extract_removed),
    },
    container: lib.target.container || 'mkv',
  }
}

export default function LibraryEditPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const client = useQueryClient()
  const toast = useToast()

  const library = useQuery({
    queryKey: ['library', id],
    queryFn: () => getLibrary(id),
  })
  // The details element is the source of truth for "exact mode": its open
  // state decides what Save sends (easy object vs. full target).
  const advancedRef = useRef<HTMLDetailsElement | null>(null)
  const [advancedShown, setAdvancedShown] = useState(false)
  const [name, setName] = useState<string | null>(null)
  const [watch, setWatch] = useState<boolean | null>(null)
  const [easy, setEasy] = useState<ReturnType<typeof parseEasy> | null>(null)
  const [target, setTarget] = useState<Target | null>(null)
  const [kinds, setKinds] = useState<string[] | null>(null)
  const [picking, setPicking] = useState(false)
  const [path, setPath] = useState<string | null>(null)

  const save = useMutation({
    mutationFn: (body: LibraryBody) => updateLibrary(id, body),
    onSuccess: () => {
      toast.success('Library saved.')
      void client.invalidateQueries({ queryKey: ['library'] })
      void client.invalidateQueries({ queryKey: ['libraries'] })
      void navigate(`/libraries/${id}`)
    },
    onError: (err: unknown) => toast.failure('Saving the library failed', err),
  })

  const libraries = { label: 'Libraries', to: '/libraries' }

  if (library.isPending) {
    return (
      <PageContainer data-testid="library-edit-loading">
        <PageHeader breadcrumb={[libraries]} title="Edit library" />
        <SkeletonRows rows={4} />
      </PageContainer>
    )
  }
  if (library.isError || !library.data) {
    return (
      <PageContainer data-testid="library-edit">
        <PageHeader breadcrumb={[libraries]} title="Edit library" />
        <ErrorBox title="Cannot load this library." error={library.error} />
      </PageContainer>
    )
  }
  const lib = library.data
  const hasEasy = lib.easy != null
  // Uncontrolled details: the initial open value only applies to the first
  // render; afterwards the user's toggling wins (and the ref reports it).
  const advancedInUse = !hasEasy || (advancedRef.current?.open ?? false)

  const easyState = easy ?? parseEasy(lib.easy)
  const targetState = target ?? defaultTarget(lib)
  const nameState = name ?? lib.name
  const watchState = watch ?? lib.watch
  const pathState = path ?? lib.path
  const kindsState =
    kinds ?? (lib.file_kinds?.length ? lib.file_kinds : ['video'])

  const submit = () => {
    const body: LibraryBody = {
      name: nameState,
      path: pathState,
      watch: watchState,
      file_kinds: kindsState,
    }
    if (advancedInUse) {
      body.target = targetState
    } else {
      body.easy = {
        quality: easyState.quality,
        remove_subtitles: easyState.remove_subtitles,
        audio: easyState.audio,
      }
    }
    save.mutate(body)
  }

  return (
    <PageContainer data-testid="library-edit">
      <PageHeader
        breadcrumb={[
          libraries,
          {
            label: lib.name,
            to: `/libraries/${lib.id}`,
            testId: 'library-edit-back',
          },
        ]}
        title="Edit library"
      />

      <FormSection title="Library">
        <Field label="Name" htmlFor="library-edit-name">
          <input
            id="library-edit-name"
            type="text"
            value={nameState}
            onChange={(e) => setName(e.target.value)}
            className={`${input} w-full`}
            data-testid="library-edit-name"
          />
        </Field>
        <div className="flex flex-col gap-2">
          <span className="text-body font-medium text-fg">Folder</span>
          <div className="flex gap-2">
            <p
              className={`${pathField} text-fg`}
              title={pathState}
              data-testid="library-edit-path"
            >
              {pathState}
            </p>
            <button
              type="button"
              onClick={() => setPicking(true)}
              className={`${btn} shrink-0`}
              data-testid="library-edit-choose-folder"
            >
              Change…
            </button>
          </div>
        </div>
        <div data-testid="library-edit-watch-row">
          <Checkbox
            checked={watchState}
            onChange={(next) => setWatch(next)}
            label="Watch this folder for new files"
            testId="library-edit-watch"
          />
        </div>
      </FormSection>

      <FormSection title="File types">
        <KindPicker
          value={kindsState}
          onChange={setKinds}
          testIdPrefix="library-edit-kind"
        />
        <p className="text-caption text-fg-muted">
          Files of other types stay untouched.
        </p>
      </FormSection>

      {hasEasy && !advancedShown && (
        <FormSection title="How much to shrink" testId="easy-edit">
          <QualitySlider
            value={easyState.quality}
            onChange={(quality) => setEasy({ ...easyState, quality })}
          />
          <AudioSlider
            value={easyState.audio}
            onChange={(audio) => setEasy({ ...easyState, audio })}
          />
          <RemoveSubtitlesCheckbox
            checked={easyState.remove_subtitles}
            onChange={(remove_subtitles) =>
              setEasy({ ...easyState, remove_subtitles })
            }
          />
        </FormSection>
      )}

      <FormSection
        title="Exact settings"
        description="Replaces the sliders with exact values for this library."
      >
        {/* Native details: the browser supplies the keyboard toggle, and the
            open state is read through the ref on save. */}
        <details
          ref={advancedRef}
          open={!hasEasy}
          onToggle={(e) =>
            setAdvancedShown((e.target as HTMLDetailsElement).open)
          }
          className="group"
          data-testid="advanced-panel"
        >
          <summary className="flex min-h-11 cursor-pointer list-none items-center justify-between gap-3 text-body font-semibold text-fg [&::-webkit-details-marker]:hidden">
            Show the exact values
          </summary>
          <div
            className="mt-4 flex flex-col gap-4"
            data-testid="advanced-fields"
          >
            <Field label="Video codec" htmlFor="advanced-codec">
              <select
                id="advanced-codec"
                value={targetState.video.codec}
                onChange={(e) =>
                  setTarget({
                    ...targetState,
                    video: { ...targetState.video, codec: e.target.value },
                  })
                }
                className={`${select} max-w-xs`}
                data-testid="advanced-codec"
              >
                {CODEC_CHOICES.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </Field>
            <Checkbox
              checked={targetState.video.reencode_if_same_codec}
              onChange={(next) =>
                setTarget({
                  ...targetState,
                  video: {
                    ...targetState.video,
                    reencode_if_same_codec: next,
                  },
                })
              }
              label="Re-encode video even when it already matches the target codec"
              help="Careful: every scan then rewrites the files."
              testId="advanced-reencode"
            />
            <Field label="Video quality (1–5)" htmlFor="advanced-quality">
              <input
                id="advanced-quality"
                type="number"
                min={1}
                max={5}
                value={targetState.video.quality}
                onChange={(e) =>
                  setTarget({
                    ...targetState,
                    video: {
                      ...targetState.video,
                      quality: Number(e.target.value),
                    },
                  })
                }
                className={`${input} w-32`}
                data-testid="advanced-quality"
              />
            </Field>
            <Field label="Container" htmlFor="advanced-container">
              <select
                id="advanced-container"
                value={targetState.container}
                onChange={(e) =>
                  setTarget({ ...targetState, container: e.target.value })
                }
                className={`${select} max-w-xs`}
                data-testid="advanced-container"
              >
                {CONTAINER_CHOICES.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Audio tracks" htmlFor="advanced-audio-mode">
              <select
                id="advanced-audio-mode"
                value={targetState.audio.mode}
                onChange={(e) =>
                  setTarget({
                    ...targetState,
                    audio: { ...targetState.audio, mode: e.target.value },
                  })
                }
                className={`${select} max-w-xs`}
                data-testid="advanced-audio-mode"
              >
                {AUDIO_MODE_CHOICES.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label="Preferred languages (comma separated)"
              htmlFor="advanced-languages"
            >
              <input
                id="advanced-languages"
                type="text"
                value={targetState.audio.preferred_languages.join(', ')}
                onChange={(e) => {
                  const langs = e.target.value
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean)
                  setTarget({
                    ...targetState,
                    audio: { ...targetState.audio, preferred_languages: langs },
                    subtitles: {
                      ...targetState.subtitles,
                      preferred_languages: langs,
                    },
                  })
                }}
                className={`${input} w-full`}
                data-testid="advanced-languages"
              />
            </Field>
            <Field label="Subtitles" htmlFor="advanced-subtitles-mode">
              <select
                id="advanced-subtitles-mode"
                value={targetState.subtitles.mode}
                onChange={(e) =>
                  setTarget({
                    ...targetState,
                    subtitles: {
                      ...targetState.subtitles,
                      mode: e.target.value,
                    },
                  })
                }
                className={`${select} max-w-xs`}
                data-testid="advanced-subtitles-mode"
              >
                {SUBTITLE_MODE_CHOICES.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </Field>
            {hasEasy && (
              <p className="text-caption text-fg-muted">
                Saving with this panel open stores these exact values and leaves
                the simple sliders behind for this library.
              </p>
            )}
          </div>
        </details>
      </FormSection>

      {save.isError && (
        <ErrorBox
          title="Saving the library failed."
          error={save.error}
          testId="library-edit-error"
        />
      )}

      <FormActions sticky>
        <button
          type="button"
          onClick={submit}
          disabled={save.isPending}
          className={btnPrimary}
          data-testid="library-edit-save"
        >
          {save.isPending ? 'Saving…' : 'Save changes'}
        </button>
        <button
          type="button"
          onClick={() => void navigate(`/libraries/${lib.id}`)}
          className={btnGhost}
          data-testid="library-edit-cancel"
        >
          Cancel
        </button>
      </FormActions>

      {picking && (
        <FolderPickerModal
          initialPath={pathState}
          onClose={() => setPicking(false)}
          onPick={(p) => {
            setPath(p)
            setPicking(false)
          }}
        />
      )}
    </PageContainer>
  )
}
