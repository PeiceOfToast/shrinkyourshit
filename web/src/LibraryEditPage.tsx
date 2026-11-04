// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library edit page (task 07, restyled in task 41, properties only in task
 * 47, spec 5.7). The page edits what the library is: name, folder, the
 * watch flag and the file types. Its rules are a summary with a link to the
 * Rules page, and Remove library sits in the Danger zone. The sliders and the
 * exact-values panel moved to the Rules page (Easy and Steps tiers).
 *
 * The API needs a rule on every update (`target` or `easy`, see
 * libraryParams). This page resends the rule the library already has,
 * unchanged, so a save never changes the rules: the easy object when the easy
 * tier owns the flow, the stored target otherwise. A stored target that
 * matches the stored one keeps the flow; an easy save regenerates it.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  deleteLibrary,
  getLibrary,
  updateLibrary,
  type Library,
  type LibraryBody,
} from './api'
import Checkbox from './Checkbox'
import ConfirmDialog from './ConfirmDialog'
import { pathField } from './easyControls'
import { parseEasy, qualityLabel } from './easyModel'
import ErrorBox from './ErrorBox'
import { Field, FormActions, FormSection } from './FormSection'
import FolderPickerModal from './FolderPickerModal'
import KindPicker from './KindPicker'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { btn, btnDangerGhost, btnGhost, btnPrimary, input } from './uiControls'

/** Plain-language audio part of the rules summary (spec 5.7). */
const AUDIO_SUMMARY: Record<number, string> = {
  1: 'keep every audio track',
  2: 'keep audio in my languages',
  3: 'keep the best audio track',
  4: 'keep the best audio track, made small',
}

/** One read-only line describing the rules, e.g. "Easy · Balanced · keep every audio track". */
function rulesSummary(lib: Library): string {
  if (lib.flow_source === 'easy') {
    const easy = parseEasy(lib.easy)
    const parts = [
      'Easy',
      qualityLabel(easy.quality),
      AUDIO_SUMMARY[easy.audio] ?? AUDIO_SUMMARY[1],
    ]
    if (easy.remove_subtitles) parts.push('remove subtitles')
    return parts.join(' · ')
  }
  if (lib.flow_source === 'advanced') {
    return 'Advanced · a visual flow'
  }
  return 'Steps · an ordered list'
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
  useDocumentTitle(['Edit library', library.data?.name])
  const [name, setName] = useState<string | null>(null)
  const [watch, setWatch] = useState<boolean | null>(null)
  const [kinds, setKinds] = useState<string[] | null>(null)
  const [picking, setPicking] = useState(false)
  const [path, setPath] = useState<string | null>(null)
  const [confirmRemove, setConfirmRemove] = useState(false)

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

  const remove = useMutation({
    mutationFn: () => deleteLibrary(id),
    onSuccess: () => {
      setConfirmRemove(false)
      toast.success('Library removed. The files on disk are untouched.')
      client.removeQueries({ queryKey: ['library', id] })
      void client.invalidateQueries({ queryKey: ['libraries'] })
      void client.invalidateQueries({ queryKey: ['stats'] })
      void navigate('/libraries')
    },
    onError: (err: unknown) =>
      toast.failure('Removing the library failed', err),
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
    // Resend the rule the library has, unchanged (see the file comment).
    if (lib.flow_source === 'easy' && lib.easy != null) {
      body.easy = parseEasy(lib.easy)
    } else {
      body.target = lib.target
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

      <FormSection title="Rules" testId="library-edit-rules">
        <p
          className="text-body text-fg"
          data-testid="library-edit-rules-summary"
        >
          {rulesSummary(lib)}
        </p>
        <div>
          <Link
            to={`/libraries/${lib.id}/rules`}
            className={btn}
            data-testid="library-edit-rules-link"
          >
            Change rules
          </Link>
        </div>
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

      <FormSection
        title="Danger zone"
        description="Removing a library forgets its history. Files on disk are not touched."
        testId="library-edit-danger"
      >
        <div>
          <button
            type="button"
            onClick={() => setConfirmRemove(true)}
            className={btnDangerGhost}
            data-testid={`library-delete-${lib.id}`}
          >
            Remove library
          </button>
        </div>
      </FormSection>

      {confirmRemove && (
        <ConfirmDialog
          title={`Remove “${lib.name}”?`}
          confirmLabel="Remove library"
          busy={remove.isPending}
          onConfirm={() => remove.mutate()}
          onClose={() => setConfirmRemove(false)}
          testId="confirm-delete"
          confirmTestId="confirm-delete-yes"
          cancelTestId="confirm-delete-cancel"
        >
          <p>
            This removes the library and its job records from the database and
            forgets what ShrinkYourShit did there. The files in{' '}
            <span className="text-fg">{lib.path}</span> are not touched —
            nothing on disk is deleted or renamed.
          </p>
        </ConfirmDialog>
      )}

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
