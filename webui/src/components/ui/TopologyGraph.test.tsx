import { cleanup, render } from '@solidjs/testing-library'
import { createSignal } from 'solid-js'
import { afterEach, describe, expect, it } from 'vitest'
import { TopologyGraph } from './TopologyGraph'

afterEach(cleanup)

const flowDash = '0.12' + ' ' + '0.88'

function firstPath(container: HTMLElement): string {
  return container.querySelector('path')!.getAttribute('d')!
}

function controlOffsets(d: string, x: number, y: number): number[] {
  const nums = d.match(/-?\d+/g)!.map(Number)
  const [c1x, c1y, c2x, c2y] = nums.slice(2, 6)
  const o1x = c1x - x / 3, o1y = c1y - y / 3
  const o2x = c2x - x * 2 / 3, o2y = c2y - y * 2 / 3
  return [o1x * -y + o1y * x, o2x * -y + o2y * x]
}

describe('TopologyGraph', () => {
  it.each([
    { x: 0, y: -140 },
    { x:240, y: 0 },
    { x: -100, y: 150 },
    { x: -240, y: -100 },
  ])('draws an S-curve spoke with opposite-bending halves to ($x, $y)', ({ x, y }) => {
    const nodes = [{ id: 'provider', x, y, isActive: true, isHitting: false }]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={null} reducedMotion={false} />)
    const graph = view.container.querySelector('svg')!
    const d = firstPath(view.container)

    expect(graph.getAttribute('aria-hidden')).toBe('true')
    expect(d.startsWith('M 0 0 C ')).toBe(true)
    expect(d.endsWith(`${x} ${y}`)).toBe(true)
    const [first, second] = controlOffsets(d, x, y)
    expect(first).toBeGreaterThan(0)
    expect(second).toBeLessThan(0)

    const line = graph.querySelector('path')!
    expect(line.getAttribute('stroke')).toBe('var(--accent)')
    expect(line.getAttribute('stroke-width')).toBe('3')
    expect(line.getAttribute('stroke-dasharray')).toBe('none')
    expect(graph.querySelector('circle')).toBeNull()
  })

  it('keeps the alternating S pattern identical across mirrored node positions', () => {
    const nodes = [
      { id: 'r', x: 223, y: -72, isActive: true, isHitting: false },
      { id: 'l', x:-223, y: -72, isActive: true, isHitting: false },
    ]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={null} reducedMotion={false} />)
    const lines = Array.from(view.container.querySelectorAll('path'))
    const ds = lines.map(p => p.getAttribute('d')!)

    for (const [i, node] of nodes.entries()) {
      const [first, second] = controlOffsets(ds[i], node.x, node.y)
      expect(first).toBeGreaterThan(0)
      expect(second).toBeLessThan(0)
    }
  })

  it('highlights inactive connections on hover without adding particles', () => {
    const [hoveredNode, setHoveredNode] = createSignal<string | null>(null)
    const nodes = [{ id: 'provider', x: 0, y: 190, isActive: false, isHitting: false }]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={hoveredNode()} reducedMotion={false} />)
    const line = view.container.querySelector('path')!

    expect(line.getAttribute('stroke')).toBe('var(--control-border)')
    expect(line.getAttribute('stroke-width')).toBe('2.5')
    expect(line.getAttribute('stroke-dasharray')).toBe('6 7')
    expect(view.container.querySelectorAll('path')).toHaveLength(1)
    setHoveredNode('provider')
    expect(view.container.querySelectorAll('path')).toHaveLength(2)
    expect(view.container.querySelector('circle')).toBeNull()
    setHoveredNode(null)
    expect(view.container.querySelectorAll('path')).toHaveLength(1)
  })

  it('shows one normalized-flow particle per hit and keeps static feedback with reduced motion', () => {
    const [reducedMotion, setReducedMotion] = createSignal(false)
    const [nodes, setNodes] = createSignal([{ id: 'provider', x:240, y: 0, isActive: true, isHitting: true }])
    const view = render(() => <TopologyGraph nodes={nodes()} hoveredNode={null} reducedMotion={reducedMotion()} />)
    const motion = view.container.querySelector('animateMotion')!
    const overlay = view.container.querySelectorAll('path')[1]!

    expect(view.container.querySelectorAll('path')).toHaveLength(2)
    expect(view.container.querySelectorAll('circle')).toHaveLength(1)
    expect(overlay.getAttribute('pathLength')).toBe('1')
    expect(overlay.getAttribute('stroke-dasharray')).toBe(flowDash)
    expect(motion.getAttribute('path')).toBe(firstPath(view.container))
    expect(motion.getAttribute('dur')).toBe('1.35s')
    expect(motion.getAttribute('repeatCount')).toBe('indefinite')
    setReducedMotion(true)
    expect(view.container.querySelectorAll('path')).toHaveLength(2)
    expect(view.container.querySelector('circle')).toBeNull()
    setReducedMotion(false)
    expect(view.container.querySelectorAll('circle')).toHaveLength(1)
    setNodes(nodes => nodes.map(node => ({ ...node, isHitting: false })))
    expect(view.container.querySelectorAll('path')).toHaveLength(1)
    expect(view.container.querySelector('circle')).toBeNull()
  })
})
