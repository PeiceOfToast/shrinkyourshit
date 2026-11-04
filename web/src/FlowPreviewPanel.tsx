// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The easy screen's "what this does" panel (task 14): an expandable
 * disclosure that shows the generated steps in plain language, one line
 * per step. The server compiles the easy input and renders the lines, so
 * this panel holds no compiler of its own — it is the teaching surface
 * for the step stack.
 *
 * Styling (task 45, spec 4.1 and 4.3): a Card-surface disclosure with a
 * chevron that turns when open, a 44 px summary, and a plain list. It stays
 * controlled (not the native `details` Disclosure) so the steps are fetched
 * only when the panel opens, and the closed panel renders no list at all.
 */
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { previewFlow, type EasyInput } from './api'
import Icon from './Icon'
import { card } from './uiControls'

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
    <section className={`${card} flex flex-col`}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex min-h-11 w-full cursor-pointer items-center justify-between gap-3 rounded-card px-4 text-left text-body font-semibold text-fg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus sm:px-6"
        data-testid="flow-preview-toggle"
      >
        <span>What this does</span>
        <Icon
          name="chevron-right"
          className={`h-5 w-5 shrink-0 text-fg-muted transition-transform duration-base ${
            open ? 'rotate-90' : ''
          }`}
        />
      </button>
      {open && (
        <div className="px-4 pb-4 sm:px-6 sm:pb-6" data-testid="flow-preview">
          {preview.isPending && (
            <p
              className="text-body text-fg-muted"
              data-testid="flow-preview-loading"
            >
              Working out the steps…
            </p>
          )}
          {preview.isError && (
            <p
              className="text-body text-danger"
              data-testid="flow-preview-error"
            >
              Could not show the steps right now.
            </p>
          )}
          {preview.data && (
            <ul
              className="list-disc space-y-1 pl-5 text-body text-fg marker:text-fg-subtle"
              data-testid="flow-preview-lines"
            >
              {preview.data.steps.map((s) => (
                <li key={s.id} data-testid="flow-preview-line">
                  {s.plain}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  )
}
