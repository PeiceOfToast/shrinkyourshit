// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ModalDialog (task 13): the one wrapper for every dialog in the app,
 * built on a native <dialog> so focus trapping and Escape come from the
 * platform (showModal opens it in the top layer and the browser fires the
 * `cancel` event on Escape). The component also listens for a keydown
 * Escape — the path test environments without a dialog implementation can
 * drive — and restores focus to the element that had it before the dialog
 * opened, which covers the focus-return requirement on every platform.
 *
 * The dialog element is mounted only while it should be open (the parent
 * renders it conditionally); the effect opens it on mount and the cleanup
 * closes it and hands focus back. `dismissOnBackdrop` turns a click on the
 * surrounding backdrop into a close — right for confirmations and pickers,
 * wrong for dialogs that hold form state.
 */
import { useEffect, useRef, type MouseEvent, type ReactNode } from 'react'
import Icon from './Icon'
import { btnIcon } from './uiControls'

export type DialogSize = 'sm' | 'md' | 'lg'

/** Width per size (spec 4.1): sm 28rem, md 36rem, lg 48rem. */
const SIZE: Record<DialogSize, string> = {
  sm: 'max-w-md',
  md: 'max-w-xl',
  lg: 'max-w-3xl',
}

/** The default look of every dialog: overlay level (spec 2.6). */
const BASE =
  'm-auto w-[calc(100%-2rem)] rounded-overlay bg-surface p-0 text-fg shadow-overlay ring-1 ring-line-strong/40'

export default function ModalDialog({
  onClose,
  label,
  testId,
  size = 'sm',
  className,
  dismissOnBackdrop = false,
  children,
}: {
  onClose: () => void
  /** Accessible name of the dialog, shown to assistive technology. */
  label: string
  testId?: string
  /** Panel width. Ignored when `className` replaces the default look. */
  size?: DialogSize
  /** Replaces the default panel classes for a dialog with its own layout. */
  className?: string
  dismissOnBackdrop?: boolean
  children: ReactNode
}) {
  const ref = useRef<HTMLDialogElement | null>(null)
  const restoreTo = useRef<HTMLElement | null>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose

  useEffect(() => {
    const el = ref.current
    if (!el) return
    restoreTo.current = document.activeElement as HTMLElement | null
    // The native modal path gives focus trapping and the Escape-cancel
    // behaviour for free. Environments without a dialog implementation
    // (jsdom) get the open attribute set directly so the element is
    // queryable; the keydown listener below keeps Escape working there.
    if (typeof el.showModal === 'function') {
      el.showModal()
    } else {
      el.setAttribute('open', '')
    }
    const onCancel = (event: Event) => {
      // React state owns the open state; the native close is not wanted.
      event.preventDefault()
      onCloseRef.current()
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onCloseRef.current()
      }
    }
    el.addEventListener('cancel', onCancel)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      el.removeEventListener('cancel', onCancel)
      window.removeEventListener('keydown', onKeyDown)
      if (typeof el.close === 'function' && el.open) el.close()
      const back = restoreTo.current
      if (back && back.isConnected) back.focus()
    }
  }, [])

  const onDialogClick = dismissOnBackdrop
    ? (event: MouseEvent<HTMLDialogElement>) => {
        // Clicks on ::backdrop retarget to the dialog element itself, so a
        // click that did not land on the panel is a backdrop dismissal.
        if (event.target === ref.current) onCloseRef.current()
      }
    : undefined

  return (
    <dialog
      ref={ref}
      aria-label={label}
      data-testid={testId}
      className={className ?? `${BASE} ${SIZE[size]}`}
      onClick={onDialogClick}
    >
      {children}
    </dialog>
  )
}

/**
 * The title row of a dialog: an h2 and, when `onClose` is given, a close
 * button. Padding is `px-6 pt-6` so it lines up with DialogBody.
 */
export function DialogHeader({
  title,
  onClose,
  titleTestId,
}: {
  title: ReactNode
  onClose?: () => void
  titleTestId?: string
}) {
  return (
    <div className="flex items-start justify-between gap-4 px-6 pt-6">
      <h2 className="text-heading text-fg" data-testid={titleTestId}>
        {title}
      </h2>
      {onClose && (
        <button
          type="button"
          onClick={onClose}
          aria-label="Close"
          className={`${btnIcon} -mr-2 -mt-2`}
        >
          <Icon name="close" className="h-5 w-5" />
        </button>
      )}
    </div>
  )
}

/** The scrolling middle of a dialog. */
export function DialogBody({ children }: { children: ReactNode }) {
  return (
    <div className="max-h-[70dvh] overflow-y-auto px-6 py-4 text-body text-fg">
      {children}
    </div>
  )
}

/** The action row of a dialog: right-aligned, wraps on a narrow screen. */
export function DialogFooter({ children }: { children: ReactNode }) {
  return (
    <div className="flex flex-wrap justify-end gap-2 px-6 pb-6">{children}</div>
  )
}
