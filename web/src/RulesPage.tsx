// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The rules page (task 14): the three tiers on one engine. The tier
 * selector picks the editor — Easy (the sliders), Steps (the step stack)
 * or Advanced (the visual canvas, task 15, on its own route). Steps save
 * through the shared validator first, so a broken row is marked on the row
 * itself. Going backwards from the canvas or the stack to a lower tier
 * warns that the manual graph structure will be replaced on save.
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
import {
  AudioSlider,
  QualitySlider,
  RemoveSubtitlesCheckbox,
} from './easyControls'
import { parseEasy } from './easyModel'
import { mapErrors, orderedSteps } from './flowModel'
import ErrorBox from './ErrorBox'
import FlowPreviewPanel from './FlowPreviewPanel'
import PageContainer from './PageContainer'
import { SkeletonRows } from './skeleton'
import StepStackEditor from './StepStackEditor'
import { useToast } from './toastContext'

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

  const [tier, setTier] = useState<Tier | null>(null)
  const [graph, setGraph] = useState<FlowGraph | null>(null)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [easy, setEasy] = useState<ReturnType<typeof parseEasy> | null>(null)

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

  const loading = library.isPending || flow.isPending || palette.isPending
  if (loading) {
    return (
      <PageContainer className="space-y-6" data-testid="rules-loading">
        <h1 className="text-2xl font-bold">Rules</h1>
        <SkeletonRows rows={4} />
      </PageContainer>
    )
  }
  if (
    library.isError ||
    !library.data ||
    flow.isError ||
    palette.isError ||
    !flow.data
  ) {
    return (
      <PageContainer className="space-y-6" data-testid="rules">
        <ErrorBox
          title="Cannot load the rules."
          error={library.error ?? flow.error}
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
  const editedEasy = easy ?? parseEasy(lib.easy)

  return (
    <PageContainer className="space-y-6" data-testid="rules">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Rules “{lib.name}”</h1>
        <Link
          to={`/libraries/${lib.id}`}
          className="text-sm text-slate-400 underline hover:text-slate-200"
          data-testid="rules-back"
        >
          Back to files
        </Link>
      </div>

      <div
        className="flex gap-1 rounded-2xl bg-slate-900 p-1 ring-1 ring-slate-800"
        data-testid="tier-selector"
      >
        {TIERS.map((t) =>
          t.id === 'advanced' ? (
            <Link
              key={t.id}
              to={`/libraries/${lib.id}/flow`}
              className={`flex-1 rounded-xl px-4 py-2 text-center text-sm font-semibold ${
                activeTier === 'advanced'
                  ? 'bg-emerald-600 text-white'
                  : 'text-slate-300 hover:bg-slate-800'
              }`}
              data-testid="tier-advanced"
              aria-current={activeTier === 'advanced' ? 'page' : undefined}
            >
              {t.label}
            </Link>
          ) : (
            <button
              key={t.id}
              type="button"
              onClick={() => setTier(t.id)}
              className={`flex-1 rounded-xl px-4 py-2 text-sm font-semibold ${
                activeTier === t.id
                  ? 'bg-emerald-600 text-white'
                  : 'text-slate-300 hover:bg-slate-800'
              }`}
              aria-pressed={activeTier === t.id}
              data-testid={`tier-${t.id}`}
            >
              {t.label}
            </button>
          ),
        )}
      </div>

      {activeTier === 'easy' && (
        <section className="space-y-6">
          {manualFlow && (
            <p
              className="rounded-lg bg-amber-500/10 px-4 py-3 text-sm text-amber-300"
              data-testid="easy-replace-warning"
            >
              Saving here replaces the manual steps this library currently uses
              with the simple sliders.
            </p>
          )}
          <section className="grid gap-6 rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800">
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
          </section>
          <FlowPreviewPanel easy={editedEasy} open />
          <button
            type="button"
            onClick={() => saveEasy.mutate()}
            disabled={saveEasy.isPending}
            className="rounded-lg bg-emerald-600 px-6 py-2.5 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
            data-testid="rules-easy-save"
          >
            {saveEasy.isPending ? 'Saving…' : 'Save easy settings'}
          </button>
        </section>
      )}

      {activeTier !== 'easy' && (
        <section className="space-y-4">
          {activeTier === 'advanced' && (
            <section
              className="space-y-3 rounded-2xl bg-slate-900 p-6 ring-1 ring-slate-800"
              data-testid="advanced-gateway"
            >
              <p className="text-sm text-slate-300">
                This library uses the visual flow editor: the rules are a graph
                on a canvas. Open it to see every step and edit it.
              </p>
              <Link
                to={`/libraries/${lib.id}/flow`}
                className="inline-block rounded-lg bg-emerald-600 px-4 py-2 text-sm font-semibold text-white hover:bg-emerald-500"
                data-testid="advanced-open-canvas"
              >
                Open the flow canvas
              </Link>
              <p className="text-xs text-slate-500">
                Switching to Steps or Easy below saves that editor’s rules
                instead — the manual graph structure will be lost.
              </p>
            </section>
          )}
          {activeTier === 'steps' && flow.data.source === 'advanced' && (
            <p
              className="rounded-lg bg-amber-500/10 px-4 py-3 text-sm text-amber-300"
              data-testid="steps-replace-warning"
            >
              This library currently uses the visual flow editor. Saving here
              replaces the manual graph structure with the ordered steps below.
            </p>
          )}
          <StepStackEditor
            graph={graph ?? flow.data.graph}
            palette={paletteItems}
            plugins={enabledStepPlugins}
            errors={errors}
            onChange={setGraph}
          />
          <button
            type="button"
            onClick={() => saveFlow.mutate()}
            disabled={saveFlow.isPending}
            className="rounded-lg bg-emerald-600 px-6 py-2.5 font-semibold text-white hover:bg-emerald-500 disabled:opacity-50"
            data-testid="rules-steps-save"
          >
            {saveFlow.isPending ? 'Saving…' : 'Save steps'}
          </button>
        </section>
      )}
    </PageContainer>
  )
}
