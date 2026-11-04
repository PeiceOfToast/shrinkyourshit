// SPDX-License-Identifier: AGPL-3.0-only

/**
 * The canvas settings that spec 4.3 and 5.9 fix (task 46): fitView never
 * zooms below 0.85, and the minimap only appears on a graph of 10 or more
 * steps. The real ReactFlow still renders; the mock only records its props.
 */
import { isValidElement, Children, createElement } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MiniMap } from '@xyflow/react'
import FlowCanvas from './FlowCanvas'
import type { FlowGraph } from './api'

const seen = vi.hoisted(() => ({
  props: null as null | Record<string, unknown>,
}))

vi.mock('@xyflow/react', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@xyflow/react')>()
  return {
    ...actual,
    ReactFlow: (props: Record<string, unknown>) => {
      seen.props = props
      return createElement(actual.ReactFlow, props as never)
    },
  }
})

/** A single chain of `n` probe steps. */
function chain(n: number): FlowGraph {
  return {
    schema: 1,
    nodes: Array.from({ length: n }, (_, i) => ({
      id: `s${i}`,
      type: 'probe',
    })),
    edges: Array.from({ length: n - 1 }, (_, i) => ({
      from: `s${i}`,
      to: `s${i + 1}`,
    })),
  }
}

function renderCanvas(n: number) {
  seen.props = null
  return render(
    <FlowCanvas
      graph={chain(n)}
      palette={[]}
      errors={{}}
      editing={false}
      selection={{ nodeId: null, edge: null }}
      onSelect={() => {}}
      onChange={() => {}}
    />,
  )
}

function hasMiniMap(): boolean {
  const children = Children.toArray(
    seen.props?.children as Parameters<typeof Children.toArray>[0],
  )
  return children.some((c) => isValidElement(c) && c.type === MiniMap)
}

describe('FlowCanvas view settings (task 46, spec 4.3 and 5.9)', () => {
  it('fits the view with a 0.85 minimum zoom so node text stays readable', () => {
    renderCanvas(3)
    expect(seen.props?.fitViewOptions).toEqual({
      padding: 0.2,
      minZoom: 0.85,
      maxZoom: 1,
    })
    expect(screen.getByTestId('flow-canvas')).toBeInTheDocument()
  })

  it('shows the minimap only from 10 steps', () => {
    renderCanvas(9)
    expect(hasMiniMap()).toBe(false)
    renderCanvas(10)
    expect(hasMiniMap()).toBe(true)
  })
})
