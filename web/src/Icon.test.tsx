// SPDX-License-Identifier: AGPL-3.0-only

import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import Icon, { ICONS, type IconName } from './Icon'

describe('Icon', () => {
  it('has every name the spec lists, each with a path', () => {
    const names: IconName[] = [
      'dash',
      'library',
      'nodes',
      'encode',
      'integrations',
      'app',
      'menu',
      'close',
      'collapse',
      'expand',
      'check',
      'alert',
      'info',
      'clock',
      'pause',
      'play',
      'chevron-right',
      'chevron-down',
      'copy',
      'folder',
      'file',
      'external',
      'logout',
      'brand',
    ]
    for (const name of names) {
      expect(ICONS[name], name).toBeTruthy()
    }
    expect(Object.keys(ICONS)).toHaveLength(names.length)
  })

  it('renders a decorative svg with the named path and the default size', () => {
    const { container } = render(<Icon name="library" />)
    const svg = container.querySelector('svg')
    expect(svg).not.toBeNull()
    expect(svg).toHaveAttribute('aria-hidden', 'true')
    expect(svg).toHaveAttribute('data-icon', 'library')
    expect(svg).toHaveClass('h-5', 'w-5', 'shrink-0')
    expect(svg?.querySelector('path')).toHaveAttribute('d', ICONS.library)
  })

  it('takes a custom class', () => {
    const { container } = render(
      <Icon name="alert" className="h-6 w-6 text-danger" />,
    )
    expect(container.querySelector('svg')).toHaveClass(
      'h-6',
      'w-6',
      'text-danger',
    )
  })
})
