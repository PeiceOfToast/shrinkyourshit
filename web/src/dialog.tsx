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

export default function ModalDialog({
  onClose,
  label,
  testId,
  className = 'm-auto w-full max-w-md rounded-2xl bg-slate-900 p-6 text-slate-100 shadow-xl ring-1 ring-slate-800',
  dismissOnBackdrop = false,
  children,
}: {
  onClose: () => void
  /** Accessible name of the dialog, shown to assistive technology. */
  label: string
  testId?: string
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
      className={className}
      onClick={onDialogClick}
    >
      {children}
    </dialog>
  )
}
