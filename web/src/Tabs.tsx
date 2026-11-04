// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Tabs (task 39, spec 4.1): a tablist that switches between views. It is
 * a WAI-ARIA tablist: one tab stop (roving tabindex), Left and Right move
 * between tabs and select them, Home and End jump to the ends. The consumer
 * renders the tab panel; this component only draws the tabs.
 *
 * Selection is neutral (spec 2.1): the selected tab gets a brighter text
 * colour and an accent underline. The fill is not used, so tabs never look
 * like primary buttons.
 */
import { useRef, type KeyboardEvent } from 'react'

export type TabItem = { id: string; label: string; testId?: string }

export default function Tabs({
  tabs,
  value,
  onChange,
  label,
}: {
  tabs: TabItem[]
  value: string
  onChange: (id: string) => void
  /** Accessible name of the tablist. */
  label: string
}) {
  const refs = useRef<Record<string, HTMLButtonElement | null>>({})

  const focusAndSelect = (index: number) => {
    const next = tabs[(index + tabs.length) % tabs.length]
    onChange(next.id)
    refs.current[next.id]?.focus()
  }

  const onKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    switch (event.key) {
      case 'ArrowRight':
        event.preventDefault()
        focusAndSelect(index + 1)
        break
      case 'ArrowLeft':
        event.preventDefault()
        focusAndSelect(index - 1)
        break
      case 'Home':
        event.preventDefault()
        focusAndSelect(0)
        break
      case 'End':
        event.preventDefault()
        focusAndSelect(tabs.length - 1)
        break
      default:
    }
  }

  return (
    <div
      role="tablist"
      aria-label={label}
      className="flex gap-1 overflow-x-auto border-b border-line"
    >
      {tabs.map((tab, index) => {
        const selected = tab.id === value
        return (
          <button
            key={tab.id}
            ref={(el) => {
              refs.current[tab.id] = el
            }}
            type="button"
            role="tab"
            id={`tab-${tab.id}`}
            aria-selected={selected}
            tabIndex={selected ? 0 : -1}
            data-testid={tab.testId}
            onClick={() => onChange(tab.id)}
            onKeyDown={(event) => onKeyDown(event, index)}
            className={`relative -mb-px inline-flex min-h-11 shrink-0 items-center px-3 text-body font-medium transition-colors duration-fast ease-standard focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-focus ${
              selected
                ? 'text-fg after:absolute after:inset-x-0 after:-bottom-px after:h-0.5 after:bg-accent'
                : 'text-fg-muted hover:text-fg'
            }`}
          >
            {tab.label}
          </button>
        )
      })}
    </div>
  )
}
