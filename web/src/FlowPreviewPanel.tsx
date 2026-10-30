// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen's "what this does" panel (task 14): an expandable
 * disclosure that shows the generated steps in plain language, one line
 * per step. The server compiles the easy input and renders the lines, so
 * this panel holds no compiler of its own — it is the teaching surface
 * for the step stack.
 */
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { previewFlow, type EasyInput } from './api'

export default function FlowPreviewPanel({
  easy,
  open: openProp = false,
  onOpenChange,
}: {
  /** The current easy-screen state; the panel compiles it live. */
  easy: EasyInput
  /** Controlled open state (tests); uncontrolled when omitted. */
  open?: boolean
  onOpenChange?: (open: boolean) => void
}) {
  const [openState, setOpenState] = useState(openProp)
  const open = onOpenChange ? openProp : openState
  const setOpen = (v: boolean) =>
    onOpenChange ? onOpenChange(v) : setOpenState(v)

  const preview = useQuery({
    queryKey: ['flow-preview', easy.quality, easy.remove_subtitles, easy.audio],
    queryFn: () => previewFlow({ easy }),
    enabled: open,
  })

  return (
    <section className="rounded-2xl bg-slate-900 ring-1 ring-slate-800">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex w-full items-center justify-between px-6 py-4 text-left"
        data-testid="flow-preview-toggle"
      >
        <span className="font-semibold text-slate-300">What this does</span>
        <span className="text-xs text-slate-500">{open ? 'Hide' : 'Show'}</span>
      </button>
      {open && (
        <div
          className="border-t border-slate-800 px-6 py-4"
          data-testid="flow-preview"
        >
          {preview.isPending && (
            <p
              className="text-sm text-slate-500"
              data-testid="flow-preview-loading"
            >
              Working out the steps…
            </p>
          )}
          {preview.isError && (
            <p
              className="text-sm text-rose-400"
              data-testid="flow-preview-error"
            >
              Could not show the steps right now.
            </p>
          )}
          {preview.data && (
            <ol className="space-y-2" data-testid="flow-preview-lines">
              {preview.data.steps.map((s) => (
                <li
                  key={s.id}
                  className="flex gap-3 text-sm text-slate-300"
                  data-testid="flow-preview-line"
                >
                  <span className="text-slate-500">•</span>
                  <span>{s.plain}</span>
                </li>
              ))}
            </ol>
          )}
        </div>
      )}
    </section>
  )
}
