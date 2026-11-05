// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The rules page (task 14): the three tiers on one engine. The tier
 * selector picks the editor — Easy (the sliders), Steps (the step stack)
 * or Advanced (the visual canvas, task 15, on its own route). Steps save
 * through the shared validator first, so a broken row is marked on the row
 * itself. Going backwards from the canvas or the stack to a lower tier
 * warns that the manual graph structure will be replaced on save.
 *
 * Restyled in task 45 (spec 5.8): one PageHeader with a breadcrumb, a
 * neutral tier selector that marks the tier in use, a caption that explains
 * the three tiers, Banners for the replace warnings, and "Save rules" on
 * every tier.
 */
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import {
  getFlowStepTypes,
  getPlugins,
  getLibrary,
  getLibraryFlow,
  putLibraryFlow,
  updateLibrary,
  validateFlow,
  type FlowGraph,
} from './api'
import { ApiError } from './api'
import Badge from './Badge'
import Banner from './Banner'
import Card from './Card'
import {
  AudioSlider,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'
import { parseEasy } from './easyModel'
import { mapErrors, orderedSteps } from './flowModel'
import { FormActions, FormSection } from './FormSection'
import ErrorBox from './ErrorBox'
import FlowPreviewPanel from './FlowPreviewPanel'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import SegmentedControl from './SegmentedControl'
import { SkeletonRows } from './skeleton'
import StepStackEditor from './StepStackEditor'
import { isLocked } from './tierModel'
import { ReadOnlyReason } from './TierNotices'
import { useDocumentTitle } from './useDocumentTitle'
import { useToast } from './toastContext'
import { btnPrimary } from './uiControls'

type Tier = 'easy' | 'steps' | 'advanced'

const TIERS: Array<{ id: Tier; label: string }> = [
  { id: 'easy', label: 'Easy' },
  { id: 'steps', label: 'Steps' },
  { id: 'advanced', label: 'Advanced' },
]

function tierOf(source: string): Tier {
  if (source === 'easy') return 'easy'
  if (source === 'advanced') return 'advanced'
  return 'steps'
}

export default function RulesPage() {
  const { id = '' } = useParams()
  const client = useQueryClient()
  const toast = useToast()

  const library = useQuery({
    queryKey: ['library', id],
    queryFn: () => getLibrary(id),
  })
  const flow = useQuery({
    queryKey: ['library-flow', id],
    queryFn: () => getLibraryFlow(id),
  })
  const palette = useQuery({
    queryKey: ['flow-step-types'],
    queryFn: getFlowStepTypes,
    staleTime: Infinity,
  })
  const plugins = useQuery({
    queryKey: ['plugins'],
    queryFn: getPlugins,
    staleTime: Infinity,
  })
  // What each lower editor can change (task 62). An editor whose tier cannot
  // express a step is read only: its Save is off and the reason is shown.
  const basicLock = useQuery({
    queryKey: ['library-flow', id, 'basic'],
    queryFn: () => getLibraryFlow(id, 'basic'),
  })
  const moderateLock = useQuery({
    queryKey: ['library-flow', id, 'moderate'],
    queryFn: () => getLibraryFlow(id, 'moderate'),
  })

  const [tier, setTier] = useState<Tier | null>(null)
  const [graph, setGraph] = useState<FlowGraph | null>(null)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [easy, setEasy] = useState<ReturnType<typeof parseEasy> | null>(null)

  useDocumentTitle(['Rules', library.data?.name])

  const activeTier: Tier = tier ?? tierOf(library.data?.flow_source ?? '')

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ['library', id] })
    void client.invalidateQueries({ queryKey: ['library-flow', id] })
    void client.invalidateQueries({ queryKey: ['libraries'] })
  }

  const saveFlow = useMutation({
    mutationFn: async () => {
      // The edited graph when the user touched the stack, the stored one
      // otherwise — a save without edits still saves what is there.
      const g = graph ?? flow.data?.graph
      if (!g) throw new Error('nothing to save')
      try {
        await validateFlow(g)
      } catch (err) {
        if (err instanceof ApiError && err.code === 'invalid_flow') {
          const parsed = JSON.parse(err.raw ?? '{}') as {
            errors?: Array<{ path: string; message: string }>
          }
          setErrors(mapErrors(parsed.errors ?? [], orderedSteps(g)))
          throw new Error('The flow has errors — fix the marked steps.')
        }
        throw err
      }
      setErrors({})
      return putLibraryFlow(id, { source: 'steps', graph: g })
    },
    onSuccess: () => {
      toast.success('Steps saved.')
      invalidate()
    },
    onError: (err: unknown) => {
      if (
        !(err instanceof Error) ||
        !err.message.startsWith('The flow has errors')
      ) {
        toast.failure('Saving the steps failed', err)
      }
    },
  })

  const saveEasy = useMutation({
    mutationFn: () => {
      const e = easy ?? parseEasy(library.data?.easy ?? null)
      return updateLibrary(id, {
        name: library.data?.name ?? '',
        path: library.data?.path ?? '',
        watch: library.data?.watch ?? false,
        easy: e,
      })
    },
    onSuccess: () => {
      toast.success('Easy settings saved. The manual steps were replaced.')
      invalidate()
    },
    onError: (err: unknown) =>
      toast.failure('Saving the easy settings failed', err),
  })

  const libraryCrumb = { label: 'Libraries', to: '/libraries' }

  const loading =
    library.isPending ||
    flow.isPending ||
    palette.isPending ||
    basicLock.isPending ||
    moderateLock.isPending
  if (loading) {
    return (
      <PageContainer className="space-y-6" data-testid="rules-loading">
        <PageHeader breadcrumb={[libraryCrumb]} title="Rules" />
        <SkeletonRows rows={4} />
      </PageContainer>
    )
  }
  if (
    library.isError ||
    !library.data ||
    flow.isError ||
    palette.isError ||
    basicLock.isError ||
    moderateLock.isError ||
    !flow.data ||
    !basicLock.data ||
    !moderateLock.data
  ) {
    return (
      <PageContainer className="space-y-6" data-testid="rules">
        <PageHeader breadcrumb={[libraryCrumb]} title="Rules" />
        <ErrorBox
          title="Cannot load the rules."
          error={
            library.error ??
            flow.error ??
            palette.error ??
            basicLock.error ??
            moderateLock.error
          }
          onRetry={() => {
            void library.refetch()
            void flow.refetch()
            void palette.refetch()
            void basicLock.refetch()
            void moderateLock.refetch()
          }}
        />
      </PageContainer>
    )
  }

  const lib = library.data
  const paletteItems = palette.data.items
  const enabledStepPlugins = (plugins.data?.items ?? []).filter(
    (p) => p.type === 'step' && p.enabled,
  )
  const manualFlow = lib.flow_source !== 'easy'
  const easyLocked = isLocked(basicLock.data.unrepresentable)
  const stepsLocked = isLocked(moderateLock.data.unrepresentable)
  const editedEasy = easy ?? parseEasy(lib.easy)
  const inUse = tierOf(flow.data.source)

  return (
    <PageContainer className="space-y-6" data-testid="rules">
      <PageHeader
        breadcrumb={[
          libraryCrumb,
          {
            label: lib.name,
            to: `/libraries/${lib.id}`,
            testId: 'rules-back',
          },
        ]}
        title="Rules"
        subtitle="Pick one way to describe what happens to each file. Easy is enough for most libraries."
      />

      <div className="flex flex-col gap-2" data-testid="tier-selector">
        <SegmentedControl
          label="Rule editor"
          value={activeTier}
          onChange={(id) => setTier(id as Tier)}
          options={TIERS.map((t) => ({
            id: t.id,
            label: t.label,
            testId: `tier-${t.id}`,
            to: t.id === 'advanced' ? `/libraries/${lib.id}/flow` : undefined,
            badge:
              inUse === t.id ? (
                <Badge tone="success" testId={`tier-${t.id}-in-use`}>
                  In use
                </Badge>
              ) : undefined,
          }))}
        />
        <p className="text-caption text-fg-muted">
          Easy: three sliders. Steps: an ordered list. Advanced: a visual graph.
          All three produce the same kind of rules.
        </p>
      </div>

      {activeTier === 'easy' && (
        <>
          {easyLocked && (
            <ReadOnlyReason
              tier="basic"
              steps={basicLock.data.unrepresentable ?? []}
              libraryId={lib.id}
              testId="easy-locked-reason"
            />
          )}
          {manualFlow && !easyLocked && (
            <Banner tone="warning" testId="easy-replace-warning">
              Saving here replaces the manual steps this library currently uses
              with the simple sliders.
            </Banner>
          )}
          <FormSection title="How much to shrink">
            {/* A disabled fieldset turns every slider and checkbox off at once. */}
            <fieldset
              disabled={easyLocked}
              className="contents"
              data-testid="easy-fieldset"
            >
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
              onClick={() => saveEasy.mutate()}
              disabled={saveEasy.isPending || easyLocked}
              className={btnPrimary}
              data-testid="rules-easy-save"
            >
              {saveEasy.isPending ? 'Saving…' : 'Save rules'}
            </button>
          </FormActions>
        </>
      )}

      {activeTier !== 'easy' && (
        <>
          {activeTier === 'advanced' && (
            <>
              <Card testId="advanced-gateway">
                <div className="flex flex-col items-start gap-4">
                  <p className="text-body text-fg">
                    This library uses the visual flow editor: the rules are a
                    graph on a canvas. Open it to see every step and edit it.
                  </p>
                  <Link
                    to={`/libraries/${lib.id}/flow`}
                    className={btnPrimary}
                    data-testid="advanced-open-canvas"
                  >
                    Open the flow editor
                  </Link>
                </div>
              </Card>
              <Banner tone="warning">
                Switching to Steps or Easy below saves that editor’s rules
                instead — the manual graph structure will be lost.
              </Banner>
            </>
          )}
          {stepsLocked && (
            <ReadOnlyReason
              tier="moderate"
              steps={moderateLock.data.unrepresentable ?? []}
              libraryId={lib.id}
              testId="steps-locked-reason"
            />
          )}
          {activeTier === 'steps' &&
            flow.data.source === 'advanced' &&
            !stepsLocked && (
              <Banner tone="warning" testId="steps-replace-warning">
                This library currently uses the visual flow editor. Saving here
                replaces the manual graph structure with the ordered steps
                below.
              </Banner>
            )}
          <Card
            title="Steps, in order"
            description="Each file goes through these steps from top to bottom."
          >
            <fieldset disabled={stepsLocked} className="contents">
              <StepStackEditor
                graph={graph ?? flow.data.graph}
                palette={paletteItems}
                plugins={enabledStepPlugins}
                errors={errors}
                onChange={setGraph}
              />
            </fieldset>
          </Card>
          <FormActions>
            <button
              type="button"
              onClick={() => saveFlow.mutate()}
              disabled={saveFlow.isPending || stepsLocked}
              className={btnPrimary}
              data-testid="rules-steps-save"
            >
              {saveFlow.isPending ? 'Saving…' : 'Save rules'}
            </button>
          </FormActions>
        </>
      )}
    </PageContainer>
  )
}
