// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ConfirmDialog (task 13): the one confirmation for every destructive
 * action. Built on ModalDialog (native <dialog>), so Escape closes it and
 * focus returns to the trigger. The confirm button always names the action
 * ("Remove library", "Stop node") — never "OK" — and the body is expected
 * to say exactly what is and is not affected.
 */
import ModalDialog from './dialog'

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
  children: React.ReactNode
}) {
  return (
    <ModalDialog
      onClose={onClose}
      label={title}
      testId={testId}
      dismissOnBackdrop
    >
      <h2 className="text-lg font-semibold">{title}</h2>
      <div className="mt-2 space-y-2 text-sm text-slate-300">{children}</div>
      <div className="mt-4 flex gap-2">
        <button
          type="button"
          onClick={onConfirm}
          disabled={busy}
          className={`rounded-lg px-4 py-2 text-sm font-semibold text-white disabled:opacity-50 ${
            danger
              ? 'bg-red-600 hover:bg-red-500'
              : 'bg-emerald-600 hover:bg-emerald-500'
          }`}
          data-testid={confirmTestId}
        >
          {busy ? `${confirmLabel}…` : confirmLabel}
        </button>
        <button
          type="button"
          onClick={onClose}
          disabled={busy}
          className="rounded-lg px-4 py-2 text-sm text-slate-300 hover:bg-slate-800 disabled:opacity-50"
          data-testid={cancelTestId}
        >
          {cancelLabel}
        </button>
      </div>
    </ModalDialog>
  )
}
