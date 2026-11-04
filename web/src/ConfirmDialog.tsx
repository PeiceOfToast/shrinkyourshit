// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ConfirmDialog (task 13, restyled in task 39): the one confirmation for
 * every destructive action. Built on ModalDialog (native <dialog>), so Escape
 * closes it and focus returns to the trigger. The confirm button always
 * names the action ("Remove library", "Stop node"), never "OK", and the
 * body is expected to say exactly what is and is not affected.
 *
 * The footer order is Cancel, then confirm (spec 4.1). On a phone both
 * buttons take the full width, and the confirm sits on top, where the
 * thumb lands first.
 */
import type { ReactNode } from 'react'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'
import { btnDanger, btnGhost, btnPrimary } from './uiControls'

export default function ConfirmDialog({
  title,
  busy = false,
  danger = true,
  confirmLabel,
  cancelLabel = 'Cancel',
  onConfirm,
  onClose,
  testId = 'confirm-dialog',
  confirmTestId = 'confirm-dialog-confirm',
  cancelTestId = 'confirm-dialog-cancel',
  children,
}: {
  title: string
  busy?: boolean
  danger?: boolean
  confirmLabel: string
  cancelLabel?: string
  onConfirm: () => void
  onClose: () => void
  testId?: string
  confirmTestId?: string
  cancelTestId?: string
  children: ReactNode
}) {
  return (
    <ModalDialog
      onClose={onClose}
      label={title}
      testId={testId}
      dismissOnBackdrop
    >
      <DialogHeader title={title} />
      <DialogBody>
        <div className="flex flex-col gap-2">{children}</div>
      </DialogBody>
      <DialogFooter>
        <button
          type="button"
          onClick={onClose}
          disabled={busy}
          className={`${btnGhost} w-full sm:w-auto`}
          data-testid={cancelTestId}
        >
          {cancelLabel}
        </button>
        <button
          type="button"
          onClick={onConfirm}
          disabled={busy}
          className={`${danger ? btnDanger : btnPrimary} order-first w-full sm:order-none sm:w-auto`}
          data-testid={confirmTestId}
        >
          {busy ? `${confirmLabel}…` : confirmLabel}
        </button>
      </DialogFooter>
    </ModalDialog>
  )
}
