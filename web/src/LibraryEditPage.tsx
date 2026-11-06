// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Library edit page (task 07, restyled in task 41, properties only in task
 * 47, spec 5.7). The page edits what the library is: name, folder, the
 * watch flag, the file types and the encode profile it follows (task 73).
 * The steps belong to the profile, edited in Encode settings; this page
 * links there. Remove library sits in the Danger zone.
 *
 * The API accepts a profile-only save. Libraries saved before task 73 still
 * carry a stored rule, so this page also resends the rule the library has,
 * unchanged: the easy object when the easy tier owns the flow, the stored
 * target otherwise. A save therefore never changes a legacy rule.
 */
import { useDocumentTitle } from './useDocumentTitle'
import { useId, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate, useParams } from 'react-router-dom'
import {
  deleteLibrary,
  getLibrary,
  updateLibrary,
  type LibraryBody,
} from './api'
import Checkbox from './Checkbox'
import ConfirmDialog from './ConfirmDialog'
import { pathField } from './easyControls'
import { parseEasy } from './easyModel'
import ErrorBox from './ErrorBox'
import { Field, FormActions, FormSection } from './FormSection'
import FolderPickerModal from './FolderPickerModal'
import KindPicker from './KindPicker'
import { ProfileSelect, ProfileSummary } from './ProfileChoice'
import { useProfiles } from './queries'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { btn, btnDangerGhost, btnGhost, btnPrimary, input } from './uiControls'

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
  const [profile, setProfile] = useState<string | null>(null)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const profileSelectId = useId()
  const profiles = useProfiles()
  const profileList = profiles.data ?? []

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
  const profileState = profile ?? lib.profile_id ?? ''
  const chosenProfile = profileList.find((p) => p.id === profileState)

  const submit = () => {
    const body: LibraryBody = {
      name: nameState,
      path: pathState,
      watch: watchState,
      file_kinds: kindsState,
      profile_id: profileState,
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

      <FormSection
        title="Encode profile"
        description="The profile says how files are shrunk. It is shared with every library that uses it."
        testId="library-edit-rules"
      >
        {!profileState && (
          <p
            className="text-body text-warning"
            data-testid="library-edit-no-profile"
          >
            This library has no encode profile yet, so it does not shrink
            anything. Choose one below.
          </p>
        )}
        {profiles.isError ? (
          <p className="text-body text-danger">
            The profiles could not be loaded. Reload the page to try again.
          </p>
        ) : (
          <ProfileSelect
            id={profileSelectId}
            value={profileState}
            onChange={setProfile}
            profiles={profileList}
            testId="library-edit-profile"
          />
        )}
        {chosenProfile && (
          <div data-testid="library-edit-rules-summary">
            <ProfileSummary
              profile={chosenProfile}
              testId="library-edit-profile-summary"
            />
          </div>
        )}
        <div>
          <Link
            to={profileState ? `/encode/profiles/${profileState}` : '/encode'}
            className={btn}
            data-testid="library-edit-rules-link"
          >
            Edit profiles in Encode settings
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
