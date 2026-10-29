// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library edit page (task 07). When the library was produced by the easy
 * screen its sliders reopen; a collapsed Advanced panel exposes the raw
 * Target fields (codec, reencode_if_same_codec, container, languages) —
 * the only place in the UI where codec names appear. Saving in advanced
 * mode sends a full target (which clears the easy state server-side);
 * saving in easy mode sends the easy object.
 */
import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { getLibrary, updateLibrary, type LibraryBody, type Target } from './api'
import {
  AudioSlider,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'
import { parseEasy } from './easyModel'
import FolderPickerModal from './FolderPickerModal'
import PageContainer from './PageContainer'

const CODEC_CHOICES = [
  { value: 'hevc', label: 'HEVC (H.265) — recommended' },
  { value: 'av1', label: 'AV1 — smallest files, newer devices only' },
  { value: 'h264', label: 'H.264 — for very old devices' },
  { value: 'copy', label: 'Copy — never re-encode the video' },
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

  const library = useQuery({
    queryKey: ['library', id],
    queryFn: () => getLibrary(id),
  })
  // The details element is the source of truth for "advanced mode": its
  // open state decides what Save sends (easy object vs. full target).
  const advancedRef = useRef<HTMLDetailsElement | null>(null)
  const [advancedShown, setAdvancedShown] = useState(false)
  const [name, setName] = useState<string | null>(null)
  const [watch, setWatch] = useState<boolean | null>(null)
  const [easy, setEasy] = useState<ReturnType<typeof parseEasy> | null>(null)
  const [target, setTarget] = useState<Target | null>(null)
  const [picking, setPicking] = useState(false)
  const [path, setPath] = useState<string | null>(null)

  const save = useMutation({
    mutationFn: (body: LibraryBody) => updateLibrary(id, body),
    onSuccess: () => {
      void client.invalidateQueries({ queryKey: ['library'] })
      void client.invalidateQueries({ queryKey: ['libraries'] })
      void navigate(`/libraries/${id}`)
    },
  })

  if (library.isPending) {
    return <p className="text-slate-400">Loading library…</p>
  }
  if (library.isError || !library.data) {
    return <p className="text-red-400">Cannot load this library.</p>
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

  const submit = () => {
    const body: LibraryBody = {
      name: nameState,
      path: pathState,
      watch: watchState,
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
    <PageContainer className="space-y-6" data-testid="library-edit">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Edit “{lib.name}”</h1>
        <Link
          to={`/libraries/${lib.id}`}
          className="text-sm text-slate-400 underline hover:text-slate-200"
          data-testid="library-edit-back"
        >
          Back to files
        </Link>
      </div>

      <section className="grid gap-3 rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
        <label className="grid gap-1">
          <span className="text-sm font-medium text-slate-300">Name</span>
          <input
            type="text"
            value={nameState}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded-lg bg-slate-800 px-3 py-2"
            data-testid="library-edit-name"
          />
        </label>
        <div className="grid gap-1">
          <span className="text-sm font-medium text-slate-300">Folder</span>
          <div className="flex items-center gap-2">
            <p
              className="min-w-0 flex-1 truncate rounded-lg bg-slate-800 px-3 py-2 text-sm"
              data-testid="library-edit-path"
            >
              {pathState}
            </p>
            <button
              type="button"
              onClick={() => setPicking(true)}
              className="shrink-0 rounded-lg bg-slate-800 px-3 py-2 text-sm hover:bg-slate-700"
              data-testid="library-edit-choose-folder"
            >
              Change…
            </button>
          </div>
        </div>
        <label className="flex items-center gap-3">
          <input
            type="checkbox"
            checked={watchState}
            onChange={(e) => setWatch(e.target.checked)}
            className="h-4 w-4 accent-emerald-500"
            data-testid="library-edit-watch"
          />
          <span className="text-sm text-slate-300">
            Watch this folder for new files
          </span>
        </label>
      </section>

      {hasEasy && !advancedShown && (
        <section
          className="grid gap-6 rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800"
          data-testid="easy-edit"
        >
          <QualitySlider
            value={easyState.quality}
            onChange={(quality) => setEasy({ ...easyState, quality })}
          />
          <RemoveSubtitlesCheckbox
            checked={easyState.remove_subtitles}
            onChange={(remove_subtitles) =>
              setEasy({ ...easyState, remove_subtitles })
            }
          />
          <AudioSlider
            value={easyState.audio}
            onChange={(audio) => setEasy({ ...easyState, audio })}
          />
        </section>
      )}

      <section className="rounded-2xl bg-slate-900 ring-1 ring-slate-800">
        <details
          ref={advancedRef}
          open={!hasEasy}
          onToggle={(e) =>
            setAdvancedShown((e.target as HTMLDetailsElement).open)
          }
          data-testid="advanced-panel"
        >
          <summary className="cursor-pointer px-6 py-4 font-semibold text-slate-300">
            Advanced
            <span className="ml-2 text-xs font-normal text-slate-500">
              Fine-tune codecs, containers and languages
            </span>
          </summary>
          <div className="grid gap-4 px-6 pb-6" data-testid="advanced-fields">
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Video codec</span>
              <select
                value={targetState.video.codec}
                onChange={(e) =>
                  setTarget({
                    ...targetState,
                    video: { ...targetState.video, codec: e.target.value },
                  })
                }
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
                data-testid="advanced-codec"
              >
                {CODEC_CHOICES.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex items-center gap-3">
              <input
                type="checkbox"
                checked={targetState.video.reencode_if_same_codec}
                onChange={(e) =>
                  setTarget({
                    ...targetState,
                    video: {
                      ...targetState.video,
                      reencode_if_same_codec: e.target.checked,
                    },
                  })
                }
                className="h-4 w-4 accent-emerald-500"
                data-testid="advanced-reencode"
              />
              <span className="text-sm text-slate-300">
                Re-encode video even when it already matches the target codec
                (careful: every scan then rewrites the files)
              </span>
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">
                Video quality (1–5)
              </span>
              <input
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
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
                data-testid="advanced-quality"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Container</span>
              <select
                value={targetState.container}
                onChange={(e) =>
                  setTarget({ ...targetState, container: e.target.value })
                }
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
                data-testid="advanced-container"
              >
                <option value="mkv">mkv</option>
                <option value="mp4">mp4</option>
              </select>
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Audio mode</span>
              <select
                value={targetState.audio.mode}
                onChange={(e) =>
                  setTarget({
                    ...targetState,
                    audio: { ...targetState.audio, mode: e.target.value },
                  })
                }
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
                data-testid="advanced-audio-mode"
              >
                <option value="keep_all">keep_all</option>
                <option value="keep_preferred">keep_preferred</option>
                <option value="keep_best">keep_best</option>
                <option value="compact">compact</option>
              </select>
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">
                Preferred languages (comma separated)
              </span>
              <input
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
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
                data-testid="advanced-languages"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-sm text-slate-300">Subtitle mode</span>
              <select
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
                className="w-full rounded-lg bg-slate-800 px-3 py-2"
                data-testid="advanced-subtitles-mode"
              >
                <option value="keep_all">keep_all</option>
                <option value="keep_preferred">keep_preferred</option>
                <option value="remove">remove</option>
              </select>
            </label>
            {hasEasy && (
              <p className="text-xs text-slate-500">
                Saving with the Advanced panel open stores these exact values
                and leaves the simple sliders behind for this library.
              </p>
            )}
          </div>
        </details>
      </section>

      {save.isError && (
        <p
          className="text-sm text-red-400"
          role="status"
          data-testid="library-edit-error"
        >
          {(save.error as Error).message}
        </p>
      )}

      <div className="flex gap-2">
        <button
          type="button"
          onClick={submit}
          disabled={save.isPending}
          className="rounded-lg bg-emerald-600 px-6 py-2.5 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
          data-testid="library-edit-save"
        >
          {save.isPending ? 'Saving…' : 'Save changes'}
        </button>
        <button
          type="button"
          onClick={() => void navigate(`/libraries/${lib.id}`)}
          className="rounded-lg px-4 py-2.5 text-sm text-slate-300 hover:bg-slate-800"
          data-testid="library-edit-cancel"
        >
          Cancel
        </button>
      </div>

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
