// SPDX-License-Identifier: AGPL-3.0-only

/**
 * CopyButton tests (task 39): it writes the text to the clipboard and then
 * reads "Copied". A refused clipboard leaves the label alone.
 */
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import CopyButton from './CopyButton'

describe('CopyButton', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('copies the text and then says Copied', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    render(
      <CopyButton text="docker run x" label="Copy command" testId="copy" />,
    )
    expect(screen.getByTestId('copy')).toHaveTextContent('Copy command')
    fireEvent.click(screen.getByTestId('copy'))
    expect(writeText).toHaveBeenCalledWith('docker run x')
    await waitFor(() =>
      expect(screen.getByTestId('copy')).toHaveTextContent('Copied'),
    )
  })

  it('keeps its label when the clipboard refuses', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'))
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    render(<CopyButton text="x" testId="copy" />)
    fireEvent.click(screen.getByTestId('copy'))
    await waitFor(() => expect(writeText).toHaveBeenCalled())
    expect(screen.getByTestId('copy')).toHaveTextContent('Copy')
    expect(screen.getByTestId('copy')).not.toHaveTextContent('Copied')
  })
})
