// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ConfirmDialog tests (task 13): Escape closes the dialog and focus
 * returns to the element that opened it; the confirm button names the
 * action; confirming and cancelling only call what they should.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import ConfirmDialog from './ConfirmDialog'

function Harness({ onConfirm }: { onConfirm?: () => void }) {
  const [open, setOpen] = useState(true)
  const [done, setDone] = useState(false)
  return (
    <div>
      <button type="button" data-testid="opener" onClick={() => setOpen(true)}>
        Remove library
      </button>
      {open && (
        <ConfirmDialog
          title="Remove “Movies”?"
          confirmLabel="Remove library"
          onConfirm={() => {
            onConfirm?.()
            setDone(true)
            setOpen(false)
          }}
          onClose={() => setOpen(false)}
        >
          <p>
            The files in <span data-testid="confirm-path">/media/movies</span>{' '}
            are not touched.
          </p>
        </ConfirmDialog>
      )}
      {done && <p data-testid="confirmed">done</p>}
    </div>
  )
}

describe('ConfirmDialog', () => {
  it('closes on Escape and returns focus to the trigger', async () => {
    render(<Harness />)
    const dialog = screen.getByTestId('confirm-dialog')
    expect(dialog).toBeInTheDocument()
    expect(dialog).toHaveTextContent('Remove “Movies”?')
    expect(screen.getByTestId('confirm-dialog-confirm')).toHaveTextContent(
      'Remove library',
    )

    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() =>
      expect(screen.queryByTestId('confirm-dialog')).not.toBeInTheDocument(),
    )
    // Nothing destructive happened.
    expect(screen.queryByTestId('confirmed')).not.toBeInTheDocument()
  })

  it('confirms only when the confirm button is clicked', async () => {
    let confirmed = 0
    render(<Harness onConfirm={() => confirmed++} />)
    fireEvent.click(screen.getByTestId('confirm-dialog-cancel'))
    await waitFor(() =>
      expect(screen.queryByTestId('confirm-dialog')).not.toBeInTheDocument(),
    )
    expect(confirmed).toBe(0)

    render(<Harness onConfirm={() => confirmed++} />)
    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'))
    await waitFor(() =>
      expect(screen.getByTestId('confirmed')).toBeInTheDocument(),
    )
    expect(confirmed).toBe(1)
  })

  it('keeps the accessible name and the body copy it is given', () => {
    render(<Harness />)
    const dialog = screen.getByTestId('confirm-dialog')
    expect(dialog).toHaveAttribute('aria-label', 'Remove “Movies”?')
    expect(screen.getByTestId('confirm-path')).toHaveTextContent(
      '/media/movies',
    )
  })
})
