// SPDX-License-Identifier: AGPL-3.0-only

/**
 * ErrorBox tests (task 39): the plain title and reason come first, the
 * technical line carries the HTTP status and code, and onRetry adds a
 * "Try again" button that calls it.
 */
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import ErrorBox from './ErrorBox'

describe('ErrorBox', () => {
  it('shows the technical line as HTTP status and code', () => {
    render(
      <ErrorBox
        title="Could not load the libraries"
        error={new ApiError('Something broke', 500, 'internal', 'raw body')}
      />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not load the libraries',
    )
    expect(screen.getByTestId('error-status')).toHaveTextContent('500')
    expect(screen.getByTestId('error-code')).toHaveTextContent('internal')
    expect(screen.getByTestId('error-raw')).toHaveTextContent('raw body')
    expect(screen.queryByRole('button', { name: 'Try again' })).toBeNull()
  })

  it('calls onRetry from the Try again button', () => {
    const onRetry = vi.fn()
    render(
      <ErrorBox
        title="Could not load"
        error={new Error('network down')}
        onRetry={onRetry}
      />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent('network down')
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })
})
