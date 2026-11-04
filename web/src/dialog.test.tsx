// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ModalDialog tests (task 39): the size prop sets the panel width, the
 * default panel is the overlay surface, and the header, body and footer
 * pieces render in order with the close button wired to onClose.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ModalDialog, { DialogBody, DialogFooter, DialogHeader } from './dialog'

describe('ModalDialog', () => {
  it('sets the width from size and uses the overlay surface by default', () => {
    const { rerender } = render(
      <ModalDialog onClose={() => {}} label="Pick" testId="dlg" size="lg">
        <p>body</p>
      </ModalDialog>,
    )
    const dlg = screen.getByTestId('dlg')
    expect(dlg).toHaveClass(
      'max-w-3xl',
      'bg-surface',
      'shadow-overlay',
      'rounded-overlay',
    )

    rerender(
      <ModalDialog onClose={() => {}} label="Pick" testId="dlg" size="md">
        <p>body</p>
      </ModalDialog>,
    )
    expect(screen.getByTestId('dlg')).toHaveClass('max-w-xl')
  })

  it('renders header with a close button, body and footer in order', () => {
    const onClose = vi.fn()
    render(
      <ModalDialog onClose={onClose} label="Choose a folder" testId="dlg">
        <DialogHeader title="Choose a folder" onClose={onClose} />
        <DialogBody>
          <p>Pick one below.</p>
        </DialogBody>
        <DialogFooter>
          <button type="button">Use this folder</button>
        </DialogFooter>
      </ModalDialog>,
    )
    expect(
      screen.getByRole('heading', { name: 'Choose a folder' }),
    ).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(screen.getByText('Pick one below.').parentElement).toHaveClass(
      'overflow-y-auto',
      'max-h-[70dvh]',
    )
    expect(
      screen.getByRole('button', { name: 'Use this folder' }).parentElement,
    ).toHaveClass('justify-end')
  })
})
