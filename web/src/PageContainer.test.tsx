// SPDX-License-Identifier: AGPL-3.0-only

import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import PageContainer from './PageContainer'

describe('PageContainer', () => {
  it('caps the width at max-w-6xl and stacks children with the default gap', () => {
    render(
      <PageContainer>
        <p>child</p>
      </PageContainer>,
    )
    const root = screen.getByText('child').parentElement!
    expect(root).toHaveClass('max-w-6xl', 'flex', 'flex-col', 'gap-6')
    expect(root).not.toHaveClass('max-w-7xl')
  })

  it('still takes a page className on top of the default', () => {
    render(
      <PageContainer className="pb-8" data-testid="page">
        <p>child</p>
      </PageContainer>,
    )
    expect(screen.getByTestId('page')).toHaveClass('max-w-6xl', 'pb-8')
  })
})
