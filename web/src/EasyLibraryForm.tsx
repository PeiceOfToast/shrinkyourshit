// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen (task 07, restyled in task 41): pick a folder, move a
 * slider, tick a box, press "Add library". Everything else is a sensible
 * default. No technical terms on this screen — the Exact settings panel on
 * the edit page is the only place codec names appear.
 */
import { useId, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import {
  browseStorage,
  createLibrary,
  getStorages,
  type Library,
  type Storage,
} from './api'
import Checkbox from './Checkbox'
import { initialEasyValue, toEasyInput, type EasyFormValue } from './easyModel'
import {
  checkTypedPath,
  freeSpaceText,
  storageBrowseKey,
  storageContaining,
  storagesKey,
} from './storageModel'
import StorageBrowser from './StorageBrowser'
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
import { btn, btnGhost, btnPrimary, input, link, select } from './uiControls'
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
  const [typing, setTyping] = useState('')
  const [typingError, setTypingError] = useState('')
  /** null means "follow the path": the share holding the chosen folder. */
  const [pickedShareId, setPickedShareId] = useState<string | null>(null)
  const set = (patch: Partial<EasyFormValue>) =>
    setValue((v) => ({ ...v, ...patch }))

  // Storages the folder can come from (task 69). A failed read hides the
  // storage choice and leaves the server folder picker, so the form still works.
  const shareQuery = useQuery({
    queryKey: storagesKey,
    queryFn: getStorages,
    select: (list) => list.items,
    retry: false,
  })
  const shares: Storage[] = shareQuery.data ?? []
  const containing = storageContaining(value.path, shares)
  const pickedId = pickedShareId ?? containing?.id ?? ''
  const share = shares.find((s) => s.id === pickedId)

  // Free space for the chosen share, read from its top folder (the same
  // listing the browser opens, so the cache is shared).
  const spaceId = share?.id ?? ''
  const spaceMount = share?.mountpoint ?? ''
  const spaceQuery = useQuery({
    queryKey: storageBrowseKey(spaceId, spaceMount),
    queryFn: () => browseStorage(spaceId, spaceMount),
    enabled: Boolean(share?.mounted),
    retry: false,
  })
  const space = spaceQuery.data
    ? freeSpaceText(spaceQuery.data.free_bytes, spaceQuery.data.total_bytes)
    : ''

  const useTypedPath = () => {
    const checked = checkTypedPath(typing, shares)
    if (!checked.ok) {
      setTypingError(checked.message)
      return
    }
    setTypingError('')
    setPickedShareId(checked.storage.id)
    set({ path: checked.path })
    setTyping('')
  }

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
        {shares.length > 0 && (
          <div className="flex flex-col gap-2">
            <label htmlFor={`${helpId}-share`} className="text-body text-fg">
              Storage
            </label>
            <select
              id={`${helpId}-share`}
              value={pickedId}
              onChange={(e) => {
                setPickedShareId(e.target.value)
                setTypingError('')
              }}
              className={select}
              data-testid="easy-storage-select"
            >
              <option value="">This server</option>
              {shares.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                  {s.mounted ? '' : ' (not mounted)'}
                </option>
              ))}
            </select>
            {share && (
              <p className="flex flex-wrap items-center gap-2 text-caption text-fg-muted">
                <span
                  className="rounded-control bg-raised px-2 py-0.5"
                  data-testid="easy-storage-badge"
                >
                  {share.mounted ? 'Mounted' : 'Not mounted'}
                </span>
                {space && <span data-testid="easy-storage-space">{space}</span>}
                {!share.mounted && (
                  <span>
                    Mount it on the{' '}
                    <a href="/storage" className={link}>
                      Storage page
                    </a>{' '}
                    before you add this folder.
                  </span>
                )}
              </p>
            )}
          </div>
        )}
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
            disabled={share !== undefined && !share.mounted}
            className={`${btn} shrink-0`}
            data-testid="easy-choose-folder"
          >
            Choose…
          </button>
        </div>
        <details
          className="text-body text-fg-muted"
          data-testid="easy-typed-path"
        >
          <summary className="inline-flex min-h-11 cursor-pointer items-center hover:text-fg">
            Type the path instead
          </summary>
          <div className="mt-2 flex flex-col gap-2">
            <label htmlFor={`${helpId}-typed`} className="text-caption">
              Full path, inside a mounted storage, for example
              /storage/nas/Movies
            </label>
            <div className="flex gap-2">
              <input
                id={`${helpId}-typed`}
                value={typing}
                onChange={(e) => {
                  setTyping(e.target.value)
                  setTypingError('')
                }}
                className={input}
                data-testid="easy-typed-path-input"
              />
              <button
                type="button"
                onClick={useTypedPath}
                className={`${btn} shrink-0`}
                data-testid="easy-use-typed-path"
              >
                Use this path
              </button>
            </div>
            {typingError && (
              <p
                role="alert"
                className="text-caption text-danger"
                data-testid="easy-typed-path-error"
              >
                {typingError}
              </p>
            )}
          </div>
        </details>
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

      {picking && share && (
        <StorageBrowser
          storage={share}
          initialPath={value.path || undefined}
          onClose={() => setPicking(false)}
          onPick={(path) => {
            set({ path })
            setPicking(false)
          }}
        />
      )}
      {picking && !share && (
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
