// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The flow canvas page (task 15, tier 3): the library's rule graph as a
 * React Flow canvas, reachable from the rules page tier selector. Read
 * only first — an explicit "Edit flow" unlocks editing, so nobody breaks a
 * generated flow by accident. Below the `md` breakpoint (768px, task 22) the
 * page is view only: no palette, no inspector, no edit controls, just the
 * notice and the canvas. Editing state is `editingRequested && wide`, so
 * resizing below md degrades to viewing and the unsaved guard still warns.
 * The server owns validation: every change
 * re-runs POST /api/v1/flows/validate, the returned field errors render on
 * the nodes, and Save stays disabled with the reason shown while the graph
 * is invalid. Saving writes the whole graph with source=advanced.
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ApiError,
  getFlowStepTypes,
  getLibrary,
  getLibraryFlow,
  putLibraryFlow,
  validateFlow,
  type FlowFieldError,
  type FlowGraph,
} from './api'
import ErrorBox from './ErrorBox'
import FlowCanvas from './FlowCanvas'
import FlowInspector from './FlowInspector'
import FlowPalette from './FlowPalette'
import {
  addStep,
  mapNodeErrors,
  NO_SELECTION,
  type CanvasSelection,
} from './flowCanvasModel'
import PageContainer from './PageContainer'
import ConfirmDialog from './ConfirmDialog'
import { SkeletonRows } from './skeleton'
import { useToast } from './toastContext'
import { useUnsavedGuard } from './useUnsavedGuard'
import { useMediaQuery } from './useMediaQuery'
import { btn, btnGhost, btnPrimary } from './uiControls'

/** Every change re-validates; the debounce keeps typing in the inspector
 * from firing a request per keystroke. */
const VALIDATE_DEBOUNCE_MS = 250

interface Validity {
  version: number
  valid: boolean
  /** Graph-level reason Save is blocked (null when only nodes are marked). */
  message: string | null
}

function parseFieldErrors(raw: string | undefined): FlowFieldError[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as { errors?: FlowFieldError[] }
    return parsed.errors ?? []
  } catch {
    return []
  }
}

export default function FlowCanvasPage() {
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

  const [graph, setGraphState] = useState<FlowGraph | null>(null)
  const [baseline, setBaseline] = useState<string | null>(null)
  const [version, setVersion] = useState(0)
  const [editingRequested, setEditingRequested] = useState(false)
  const wide = useMediaQuery('(min-width: 768px)')
  const editing = editingRequested && wide
  const [selection, setSelection] = useState<CanvasSelection>(NO_SELECTION)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [validity, setValidity] = useState<Validity | null>(null)
  const [confirmDiscard, setConfirmDiscard] = useState(false)

  // First load: the graph is exactly the object the API returned.
  useEffect(() => {
    if (graph || !flow.data) return
    setGraphState(flow.data.graph)
    setBaseline(JSON.stringify(flow.data.graph))
  }, [flow.data, graph])

  const applyGraph = useCallback((next: FlowGraph) => {
    setGraphState(next)
    setVersion((v) => v + 1)
    // A selection that no longer exists (its step or connection was
    // removed) resets, so the inspector never shows a ghost.
    setSelection((sel) => ({
      nodeId:
        sel.nodeId && next.nodes.some((n) => n.id === sel.nodeId)
          ? sel.nodeId
          : null,
      edge:
        sel.edge &&
        next.edges.some(
          (e) => e.from === sel.edge!.from && e.to === sel.edge!.to,
        )
          ? sel.edge
          : null,
    }))
  }, [])

  // The server decides whether the graph is valid — the client never
  // does. Errors land on the nodes verbatim.
  useEffect(() => {
    if (!graph) return
    const v = version
    let cancelled = false
    const timer = window.setTimeout(async () => {
      try {
        await validateFlow(graph)
        if (cancelled) return
        setErrors({})
        setValidity({ version: v, valid: true, message: null })
      } catch (err) {
        if (cancelled) return
        if (err instanceof ApiError && err.code === 'invalid_flow') {
          const fieldErrors = parseFieldErrors(err.raw)
          setErrors(mapNodeErrors(fieldErrors, graph.nodes))
          const graphMessage = fieldErrors.find(
            (e) => !/^nodes\[\d+\]/.test(e.path),
          )?.message
          setValidity({
            version: v,
            valid: false,
            message: graphMessage ?? null,
          })
        } else {
          setErrors({})
          setValidity({
            version: v,
            valid: false,
            message: 'The flow cannot be checked right now.',
          })
        }
      }
    }, VALIDATE_DEBOUNCE_MS)
    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [graph, version])

  const dirty =
    !!graph && baseline !== null && JSON.stringify(graph) !== baseline

  const invalidate = () => {
    void client.invalidateQueries({ queryKey: ['library', id] })
    void client.invalidateQueries({ queryKey: ['library-flow', id] })
    void client.invalidateQueries({ queryKey: ['libraries'] })
  }

  const save = useMutation({
    mutationFn: () => {
      if (!graph) throw new Error('nothing to save')
      return putLibraryFlow(id, { source: 'advanced', graph })
    },
    onSuccess: (resp) => {
      setGraphState(resp.graph)
      setBaseline(JSON.stringify(resp.graph))
      toast.success('Flow saved. This library now uses the visual editor.')
      invalidate()
    },
    onError: (err: unknown) => {
      if (err instanceof ApiError && err.code === 'invalid_flow' && graph) {
        setErrors(mapNodeErrors(parseFieldErrors(err.raw), graph.nodes))
      }
      toast.failure('Saving the flow failed', err)
    },
  })

  const guard = useUnsavedGuard(dirty)

  const paletteItems = useMemo(() => palette.data?.items ?? [], [palette.data])
  const addToGraph = useCallback(
    (type: string) => {
      if (!graph) return
      const info = paletteItems.find((p) => p.type === type)
      if (!info) return
      applyGraph(addStep(graph, info).graph)
    },
    [graph, paletteItems, applyGraph],
  )

  const discard = () => {
    setConfirmDiscard(false)
    if (!baseline) return
    applyGraph(JSON.parse(baseline) as FlowGraph)
  }

  const loading =
    library.isPending || flow.isPending || palette.isPending || !graph
  if (loading) {
    return (
      <PageContainer className="space-y-6" data-testid="flow-loading">
        <h1 className="text-2xl font-bold">Flow</h1>
        <SkeletonRows rows={4} />
      </PageContainer>
    )
  }
  const lib = library.data
  if (
    library.isError ||
    flow.isError ||
    palette.isError ||
    !flow.data ||
    !lib
  ) {
    return (
      <PageContainer className="space-y-6" data-testid="flow-canvas-page">
        <ErrorBox
          title="Cannot load the flow."
          error={library.error ?? flow.error ?? palette.error}
        />
      </PageContainer>
    )
  }

  const checking = !validity || validity.version !== version
  const canSave =
    editing && dirty && !checking && validity!.valid && !save.isPending
  const saveReason = !editing
    ? null
    : save.isPending
      ? null
      : checking
        ? 'Checking the flow…'
        : !validity!.valid
          ? (validity!.message ?? 'Fix the steps marked in red.')
          : !dirty
            ? 'No changes to save.'
            : null

  return (
    <PageContainer className="space-y-4 pb-8" data-testid="flow-canvas-page">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-2xl font-bold">Flow “{lib.name}”</h1>
          <p className="flex flex-wrap items-center gap-2 text-sm text-slate-500">
            <span data-testid="flow-summary">
              {graph!.nodes.length} steps · {graph!.edges.length} connections
            </span>
            <span
              className="rounded-full bg-slate-800 px-2 py-0.5 text-xs text-slate-300"
              data-testid="flow-source"
            >
              {flow.data.source}
            </span>
            {dirty && (
              <span className="text-xs text-amber-400" data-testid="flow-dirty">
                Unsaved changes
              </span>
            )}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {wide &&
            (editing ? (
              <>
                <button
                  type="button"
                  onClick={() => save.mutate()}
                  disabled={!canSave}
                  title={saveReason ?? undefined}
                  className={btnPrimary}
                  data-testid="flow-save"
                >
                  {save.isPending ? 'Saving…' : 'Save flow'}
                </button>
                {dirty ? (
                  <button
                    type="button"
                    onClick={() => setConfirmDiscard(true)}
                    className={btn}
                    data-testid="flow-discard"
                  >
                    Discard changes
                  </button>
                ) : (
                  <button
                    type="button"
                    onClick={() => setEditingRequested(false)}
                    className={btn}
                    data-testid="flow-done"
                  >
                    Done editing
                  </button>
                )}
              </>
            ) : (
              <button
                type="button"
                onClick={() => setEditingRequested(true)}
                className={btnPrimary}
                data-testid="flow-edit"
              >
                Edit flow
              </button>
            ))}
          <Link
            to={`/libraries/${id}/rules`}
            className={btnGhost}
            data-testid="flow-back"
          >
            Back to rules
          </Link>
        </div>
      </div>

      {wide && !editing && (
        <div
          className="flex flex-wrap items-center gap-3 rounded-2xl bg-slate-900 px-4 py-3 ring-1 ring-slate-800"
          data-testid="flow-readonly-banner"
        >
          <p className="min-w-[16rem] flex-1 text-sm text-slate-300">
            You are viewing the flow this library runs. Nothing can be changed
            here.
            {flow.data.source === 'easy' &&
              ' The rules come from the easy settings on the rules page.'}
          </p>
          <button
            type="button"
            onClick={() => setEditingRequested(true)}
            className={btn}
            data-testid="flow-edit-banner"
          >
            Edit flow
          </button>
        </div>
      )}

      {!wide && (
        <div
          className="rounded-xl bg-slate-900 p-3 text-sm text-slate-300 ring-1 ring-slate-800"
          data-testid="flow-mobile-notice"
        >
          Viewing only on a phone. Editing the flow needs a desktop browser.
        </div>
      )}

      {editing && flow.data.source === 'easy' && (
        <p
          className="rounded-lg bg-amber-500/10 px-4 py-3 text-sm text-amber-300"
          data-testid="flow-edit-warning"
        >
          Saving here replaces the generated rules with your own flow. The easy
          sliders will no longer drive this library.
        </p>
      )}

      {editing && saveReason && (
        <p className="text-sm text-amber-400" data-testid="flow-save-reason">
          {saveReason}
        </p>
      )}
      {errors['graph'] && (
        <p className="text-sm text-rose-400" data-testid="flow-graph-error">
          {errors['graph']}
        </p>
      )}

      <div className="flex flex-col gap-4 lg:flex-row">
        {wide && editing && (
          <div className="shrink-0 lg:w-56">
            <FlowPalette palette={paletteItems} onAdd={addToGraph} />
          </div>
        )}
        <div className="min-w-0 flex-1 space-y-3">
          <div className="h-[55vh] min-h-[24rem] lg:h-[68vh]">
            <FlowCanvas
              graph={graph!}
              palette={paletteItems}
              errors={errors}
              editing={editing}
              selection={selection}
              onSelect={setSelection}
              onChange={applyGraph}
            />
          </div>
        </div>
        {wide && (
          <div className="shrink-0 lg:w-80">
            <div className="rounded-2xl bg-slate-950/60 p-4 ring-1 ring-slate-800">
              <FlowInspector
                graph={graph!}
                palette={paletteItems}
                selection={selection}
                editing={editing}
                onChange={applyGraph}
              />
            </div>
          </div>
        )}
      </div>

      {confirmDiscard && (
        <ConfirmDialog
          title="Discard changes?"
          confirmLabel="Discard changes"
          cancelLabel="Keep editing"
          testId="flow-discard-dialog"
          confirmTestId="flow-discard-confirm"
          cancelTestId="flow-discard-cancel"
          onConfirm={discard}
          onClose={() => setConfirmDiscard(false)}
        >
          <p>
            The edits you made to the flow are not saved. Discarding brings back
            the flow the library currently runs.
          </p>
        </ConfirmDialog>
      )}

      {guard.pending && (
        <ConfirmDialog
          title="Leave with unsaved changes?"
          confirmLabel="Leave without saving"
          cancelLabel="Stay on the canvas"
          testId="flow-guard-dialog"
          confirmTestId="flow-guard-leave"
          cancelTestId="flow-guard-stay"
          onConfirm={guard.leave}
          onClose={guard.stay}
        >
          <p>The changes you made to the flow are not saved.</p>
        </ConfirmDialog>
      )}
    </PageContainer>
  )
}
