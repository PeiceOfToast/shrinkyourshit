// SPDX-License-Identifier: AGPL-3.0-only

/**
 * Toast tests (task 39): the region sits at the bottom on a phone and at
 * the top right from sm up, so it never covers the top bar. Success and
 * failure keep their roles.
 */
import { act, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ToastProvider } from './toast'
import { useToast } from './toastContext'

function Fire() {
  const toast = useToast()
  return (
    <button type="button" onClick={() => toast.success('Library added')}>
      fire
    </button>
  )
}

describe('toast region', () => {
  it('is at the bottom under sm and at the top right from sm up', () => {
    render(
      <ToastProvider>
        <Fire />
      </ToastProvider>,
    )
    const region = screen.getByTestId('toast-region')
    expect(region).toHaveClass(
      'bottom-4',
      'sm:top-4',
      'sm:bottom-auto',
      'z-toast',
    )
    expect(region).toHaveAttribute('aria-live', 'polite')
  })

  it('announces a success as a status', () => {
    render(
      <ToastProvider>
        <Fire />
      </ToastProvider>,
    )
    act(() => {
      screen.getByText('fire').click()
    })
    expect(screen.getByTestId('toast')).toHaveAttribute('role', 'status')
    expect(screen.getByTestId('toast')).toHaveTextContent('Library added')
    expect(
      screen.getByRole('button', { name: 'Dismiss notification' }),
    ).toHaveClass('h-11', 'w-11')
  })
})
