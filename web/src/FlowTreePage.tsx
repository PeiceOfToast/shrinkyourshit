// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The Moderate rule editor (task 60, tier 2 of the spec): the library's
 * graph as an ordered tree. A condition row owns the steps it gates, drawn
 * indented under it; an action row is a leaf. Rows add, remove and reorder
 * (drag handle, or the up and down buttons and arrow keys on the handle).
 * Fields render from the server's `StepTypes()` params, so this page names
 * no step. Server field errors land on the row that owns the field. Nothing
 * is checked on the client.
 *
 * Saving writes the whole graph with source `steps`, the same as the step
 * stack. The page never changes the library's tier: tier switching is the
 * library's own control (task 62), so a tier change never saves a graph.
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ApiError,
  getFlowStepTypes,
  getLibrary,
  getLibraryFlow,
  getPlugins,
  putLibraryFlow,
  type FlowFieldError,
  type FlowGraph,
  type FlowStep,
} from './api'
import Badge from './Badge'
import Banner from './Banner'
import ConfirmDialog from './ConfirmDialog'
import ErrorBox from './ErrorBox'
import { chipsFor, mapErrors, paletteFor, stepFromPalette } from './flowModel'
import ParamField from './flowParams'
import PageContainer from './PageContainer'
import PageHeader from './PageHeader'
import { SkeletonRows } from './skeleton'
import { isLocked } from './tierModel'
import { ReadOnlyReason } from './TierNotices'
import { useDocumentTitle } from './useDocumentTitle'
import { useToast } from './toastContext'
import { useUnsavedGuard } from './useUnsavedGuard'
import {
  btn,
  btnGhost,
  btnIcon,
  btnPrimary,
  card,
  cardPad,
  linkTouch,
  select,
} from './uiControls'
import {
  chainSteps,
  graphOf,
  isChainGraph,
  MAX_INDENT,
  moveStep,
  setStepParam,
  treeRows,
  type IsCondition,
} from './treeModel'

/** Remove is the one destructive row action: the danger text, not muted. */
const removeIcon = btnIcon
  .replace('text-fg-muted', 'text-danger')
  .replace('hover:text-fg', 'hover:bg-danger/10')

/** Indent per level, in rem. Rows past MAX_INDENT keep the cap. */
const INDENT_REM = 1.5

function parseFieldErrors(raw: string | undefined): FlowFieldError[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as { errors?: FlowFieldError[] }
    return parsed.errors ?? []
  } catch {
    return []
  }
}

export default function FlowTreePage() {
  const { id = '' } = useParams()
  const client = useQueryClient()
  const toast = useToast()
  const libraryCrumb = { label: 'Libraries', to: '/libraries' }

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
  // What the Moderate tier can change (task 62): a step it cannot express
  // (raw args, plugins) locks the whole tree read only.
  const lock = useQuery({
    queryKey: ['library-flow', id, 'moderate'],
    queryFn: () => getLibraryFlow(id, 'moderate'),
  })

  const [graph, setGraph] = useState<FlowGraph | null>(null)
  const [baseline, setBaseline] = useState<string | null>(null)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [expanded, setExpanded] = useState<string | null>(null)
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [addType, setAddType] = useState('')
  const [confirmRevert, setConfirmRevert] = useState(false)

  useDocumentTitle(['Rule tree', library.data?.name])

  // First load: the graph is exactly the object the API returned.
  useEffect(() => {
    if (graph || !flow.data) return
    setGraph(flow.data.graph)
    setBaseline(JSON.stringify(flow.data.graph))
  }, [flow.data, graph])

  const paletteItems = useMemo(() => palette.data?.items ?? [], [palette.data])
  const enabledStepPlugins = useMemo(
    () =>
      (plugins.data?.items ?? []).filter((p) => p.type === 'step' && p.enabled),
    [plugins.data],
  )
  // The palette's flag decides what a condition is; no step name is kept here.
  const isCondition: IsCondition = useCallback(
    (type: string) => paletteFor(paletteItems, type)?.condition ?? false,
    [paletteItems],
  )

  const rows = useMemo(
    () => (graph ? treeRows(graph, isCondition) : []),
    [graph, isCondition],
  )
  const chain = graph ? isChainGraph(graph, isCondition) : false
  const locked = isLocked(lock.data?.unrepresentable)
  const editable = chain && !locked
  const dirty =
    !!graph && baseline !== null && JSON.stringify(graph) !== baseline

  /** Applies an edit: the whole chain is rebuilt from the new step order. */
  const setSteps = (next: FlowStep[]) => {
    if (!graph) return
    setGraph(graphOf(graph.schema, next))
    setErrors({})
  }
  const steps = graph ? chainSteps(graph, isCondition) : []

  const move = (from: number, to: number) => setSteps(moveStep(steps, from, to))
  const remove = (stepID: string) =>
    setSteps(steps.filter((s) => s.id !== stepID))
  const add = (type: string) => {
    const info = paletteFor(paletteItems, type)
    if (!info) return
    setSteps([...steps, stepFromPalette(info, steps)])
  }
  const updateParam = (stepID: string, name: string, value: unknown) =>
    setSteps(setStepParam(steps, stepID, name, value))

  const save = useMutation({
    mutationFn: () => {
      if (!graph) throw new Error('nothing to save')
      return putLibraryFlow(id, { source: 'steps', graph })
    },
    onSuccess: (resp) => {
      setGraph(resp.graph)
      setBaseline(JSON.stringify(resp.graph))
      setErrors({})
      toast.success('Tree saved.')
      void client.invalidateQueries({ queryKey: ['library', id] })
      void client.invalidateQueries({ queryKey: ['library-flow', id] })
      void client.invalidateQueries({ queryKey: ['libraries'] })
    },
    onError: (err: unknown) => {
      // Field errors sit on the row that owns the field, by node index.
      if (err instanceof ApiError && err.code === 'invalid_flow' && graph) {
        setErrors(mapErrors(parseFieldErrors(err.raw), graph.nodes))
      }
      toast.failure('Saving the tree failed', err)
    },
  })

  const guard = useUnsavedGuard(dirty)

  const revert = () => {
    setConfirmRevert(false)
    if (!baseline) return
    setGraph(JSON.parse(baseline) as FlowGraph)
    setErrors({})
  }

  const loading =
    library.isPending ||
    flow.isPending ||
    palette.isPending ||
    lock.isPending ||
    !graph
  if (loading) {
    return (
      <PageContainer className="pb-8" data-testid="tree-loading">
        <PageHeader breadcrumb={[libraryCrumb]} title="Rule tree" />
        <SkeletonRows rows={4} />
      </PageContainer>
    )
  }
  const lib = library.data
  if (
    library.isError ||
    flow.isError ||
    palette.isError ||
    lock.isError ||
    !flow.data ||
    !lock.data ||
    !lib
  ) {
    return (
      <PageContainer className="pb-8" data-testid="tree-page">
        <PageHeader breadcrumb={[libraryCrumb]} title="Rule tree" />
        <ErrorBox
          title="Cannot load the rule tree."
          error={library.error ?? flow.error ?? palette.error}
          onRetry={() => {
            void library.refetch()
            void flow.refetch()
            void palette.refetch()
          }}
        />
      </PageContainer>
    )
  }

  const canSave = editable && dirty && !save.isPending

  return (
    <PageContainer className="pb-8" data-testid="tree-page">
      <PageHeader
        breadcrumb={[
          libraryCrumb,
          { label: lib.name, to: `/libraries/${lib.id}` },
        ]}
        title="Rule tree"
        meta={
          <span
            className="text-caption text-fg-muted tabular-nums"
            data-testid="tree-summary"
          >
            {rows.length} steps
            {dirty ? ' · unsaved changes' : ''}
          </span>
        }
        actions={
          <>
            {editable && dirty && (
              <button
                type="button"
                onClick={() => setConfirmRevert(true)}
                className={btnGhost}
                data-testid="tree-revert"
              >
                Revert
              </button>
            )}
            {editable && (
              <button
                type="button"
                onClick={() => save.mutate()}
                disabled={!canSave}
                className={btnPrimary}
                data-testid="tree-save"
              >
                {save.isPending ? 'Saving…' : 'Save tree'}
              </button>
            )}
          </>
        }
      />

      {flow.data.source === 'easy' && editable && (
        <Banner tone="warning" testId="tree-easy-warning">
          Saving here replaces the generated rules with your own steps. The easy
          sliders will no longer drive this library.
        </Banner>
      )}

      {locked && (
        <ReadOnlyReason
          tier="moderate"
          steps={lock.data?.unrepresentable ?? []}
          libraryId={lib.id}
          testId="tree-locked-reason"
        />
      )}

      {!chain && (
        <Banner tone="warning" testId="tree-readonly-banner">
          This flow branches, which the tree cannot save. Open it in the visual
          editor to change it.{' '}
          <Link
            to={`/libraries/${lib.id}/flow`}
            className={linkTouch}
            data-testid="tree-open-flow"
          >
            Open the visual editor
          </Link>
        </Banner>
      )}

      {errors['graph'] && (
        <p className="text-body text-danger" data-testid="tree-graph-error">
          {errors['graph']}
        </p>
      )}

      <section className={`${card} ${cardPad} space-y-3`} aria-label="Steps">
        {rows.length === 0 && (
          <p className="text-body text-fg-muted" data-testid="tree-empty">
            No steps yet. Add a step to start the rule.
          </p>
        )}
        <ol className="space-y-3" data-testid="tree-rows">
          {rows.map((row, i) => {
            const step = row.step
            const info = paletteFor(paletteItems, step.type)
            const rowError = errors[step.id]
            const indent = Math.min(row.depth, MAX_INDENT)
            const label = info?.label ?? step.type
            return (
              <li
                key={step.id}
                data-testid={`tree-row-${step.id}`}
                data-depth={row.depth}
                style={{ marginLeft: `${indent * INDENT_REM}rem` }}
                className={`rounded-card bg-canvas p-4 ring-1 ${
                  rowError ? 'ring-danger/60' : 'ring-line'
                }`}
              >
                <div className="flex items-center gap-2">
                  {editable ? (
                    <span
                      role="button"
                      tabIndex={0}
                      draggable
                      aria-label={`Reorder ${label}. Drag, or press the up or down arrow key.`}
                      onDragStart={() => setDragIndex(i)}
                      onDragOver={(e) => e.preventDefault()}
                      onDrop={() => {
                        if (dragIndex !== null) move(dragIndex, i)
                        setDragIndex(null)
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'ArrowUp') {
                          e.preventDefault()
                          move(i, i - 1)
                        } else if (e.key === 'ArrowDown') {
                          e.preventDefault()
                          move(i, i + 1)
                        }
                      }}
                      className="flex h-11 w-11 shrink-0 cursor-grab select-none items-center justify-center rounded-control text-fg-subtle focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
                      data-testid={`tree-handle-${step.id}`}
                    >
                      ⠿
                    </span>
                  ) : null}
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-baseline gap-2">
                      <span className="font-semibold text-fg">{label}</span>
                      <span className="truncate font-mono text-caption text-fg-muted">
                        {step.id}
                      </span>
                    </div>
                    {info?.description && (
                      <p
                        className="text-caption text-fg-muted"
                        data-testid={`tree-description-${step.id}`}
                      >
                        {info.description}
                      </p>
                    )}
                  </div>
                  {editable && (
                    <div className="flex shrink-0 items-center gap-1">
                      <button
                        type="button"
                        onClick={() => move(i, i - 1)}
                        disabled={i === 0}
                        className={btnIcon}
                        aria-label={`Move ${step.id} up`}
                        data-testid={`tree-up-${step.id}`}
                      >
                        ↑
                      </button>
                      <button
                        type="button"
                        onClick={() => move(i, i + 1)}
                        disabled={i === rows.length - 1}
                        className={btnIcon}
                        aria-label={`Move ${step.id} down`}
                        data-testid={`tree-down-${step.id}`}
                      >
                        ↓
                      </button>
                      <button
                        type="button"
                        onClick={() =>
                          setExpanded(expanded === step.id ? null : step.id)
                        }
                        className={btnIcon}
                        aria-label={`Edit ${step.id}`}
                        aria-expanded={expanded === step.id}
                        data-testid={`tree-edit-${step.id}`}
                      >
                        ✎
                      </button>
                      <button
                        type="button"
                        onClick={() => remove(step.id)}
                        className={removeIcon}
                        aria-label={`Remove ${step.id}`}
                        data-testid={`tree-remove-${step.id}`}
                      >
                        ✕
                      </button>
                    </div>
                  )}
                </div>

                {info && info.params.length > 0 && (
                  <div
                    className="mt-2 flex flex-wrap gap-2"
                    data-testid={`tree-chips-${step.id}`}
                  >
                    {chipsFor(step, info).map((chip, ci) => (
                      <Badge key={ci}>{chip}</Badge>
                    ))}
                  </div>
                )}

                {row.depth > MAX_INDENT && (
                  <p
                    className="mt-2 text-caption text-fg-muted"
                    data-testid={`tree-cap-${step.id}`}
                  >
                    Nested {row.depth} levels. Drawn at {MAX_INDENT}.
                  </p>
                )}

                {rowError && (
                  <p
                    className="mt-2 text-caption text-danger"
                    data-testid={`tree-error-${step.id}`}
                  >
                    {rowError}
                  </p>
                )}

                {editable && expanded === step.id && info && (
                  <div
                    className="mt-3 grid gap-4 border-t border-line pt-3"
                    data-testid={`tree-params-${step.id}`}
                  >
                    {info.params.map((f) => (
                      <ParamField
                        key={f.name}
                        field={f}
                        step={step}
                        plugins={enabledStepPlugins}
                        testId={`tree-param-${step.id}-${f.name}`}
                        onChange={(name, value) =>
                          updateParam(step.id, name, value)
                        }
                      />
                    ))}
                  </div>
                )}
              </li>
            )
          })}
        </ol>

        {editable && (
          <div
            className="flex flex-wrap items-center gap-2"
            data-testid="tree-add"
          >
            <select
              value={addType}
              onChange={(e) => setAddType(e.target.value)}
              className={`${select} max-w-xs`}
              aria-label="Step type to add"
              data-testid="tree-add-type"
            >
              <option value="">Choose a step…</option>
              {paletteItems.map((p) => (
                <option key={p.type} value={p.type}>
                  {p.label}
                </option>
              ))}
            </select>
            <button
              type="button"
              disabled={!addType}
              onClick={() => {
                add(addType)
                setAddType('')
              }}
              className={btn}
              data-testid="tree-add-button"
            >
              Add step
            </button>
          </div>
        )}
      </section>

      <p className="text-caption text-fg-muted">
        Need the whole graph on a canvas?{' '}
        <Link
          to={`/libraries/${lib.id}/flow`}
          className={linkTouch}
          data-testid="tree-to-flow"
        >
          Open the visual editor
        </Link>
      </p>

      {confirmRevert && (
        <ConfirmDialog
          title="Revert unsaved changes?"
          confirmLabel="Revert"
          cancelLabel="Keep editing"
          testId="tree-revert-dialog"
          confirmTestId="tree-revert-confirm"
          cancelTestId="tree-revert-cancel"
          onConfirm={revert}
          onClose={() => setConfirmRevert(false)}
        >
          <p>
            The edits you made to the tree are not saved. Reverting brings back
            the saved tree.
          </p>
        </ConfirmDialog>
      )}

      {guard.pending && (
        <ConfirmDialog
          title="Leave with unsaved changes?"
          confirmLabel="Leave without saving"
          cancelLabel="Stay on the tree"
          testId="tree-guard-dialog"
          confirmTestId="tree-guard-leave"
          cancelTestId="tree-guard-stay"
          onConfirm={guard.leave}
          onClose={guard.stay}
        >
          <p>The changes you made to the tree are not saved.</p>
        </ConfirmDialog>
      )}
    </PageContainer>
  )
}
