// SPDX-License-Identifier: AGPL-3.0-only

/**
 * CodeBlock tests (task 39): the text is shown whole in a pre that wraps,
 * and `copy` adds a copy button that copies the same text.
 */
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import CodeBlock from './CodeBlock'

describe('CodeBlock', () => {
  it('shows the text in a wrapping pre and no copy button by default', () => {
    render(<CodeBlock testId="code">{'docker run \\\n  image'}</CodeBlock>)
    const root = screen.getByTestId('code')
    const pre = root.querySelector('pre')
    expect(pre).toHaveClass('whitespace-pre-wrap', 'break-words', 'font-mono')
    expect(pre?.textContent).toContain('image')
    expect(root.querySelector('button')).toBeNull()
  })

  it('adds a copy button when copy is set', () => {
    render(
      <CodeBlock copy testId="code">
        join-token
      </CodeBlock>,
    )
    expect(screen.getByTestId('code-copy')).toHaveTextContent('Copy')
  })
})
