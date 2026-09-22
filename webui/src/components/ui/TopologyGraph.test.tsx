import { cleanup, render } from '@solidjs/testing-library'
import { createSignal } from 'solid-js'
import { afterEach, describe, expect, it } from 'vitest'
import { TopologyGraph } from './TopologyGraph'

afterEach(cleanup)

describe('TopologyGraph', () => {
  it.each([
    { x: 0, y: -140, path: 'M 0 0 C 18 -70, 8 -84, 0 -140' },
    { x: 240, y: 0, path: 'M 0 0 C 120 14, 144 6, 240 0' },
    { x: -100, y: 150, path: 'M 0 0 C -25 75, -85 90, -100 150' },
    { x: -240, y: -100, path: 'M 0 0 C -120 -25, -144 -85, -240 -100' },
  ])('preserves the curve to ($x, $y)', ({ x, y, path }) => {
    const nodes = [{ id: 'provider', x, y, isActive: true, isHitting: false }]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={null} reducedMotion={false} />)
    const graph = view.container.querySelector('svg')!
    const curve = graph.querySelector('path')!

    expect(graph.getAttribute('aria-hidden')).toBe('true')
    expect(graph.getAttribute('viewBox')).toBe('-600 -600 1200 1200')
    expect(curve.getAttribute('d')).toBe(path)
    expect(curve.getAttribute('stroke')).toBe('var(--accent)')
    expect(curve.getAttribute('opacity')).toBe('0.3')
    expect(curve.getAttribute('stroke-width')).toBe('2')
    expect(curve.getAttribute('stroke-dasharray')).toBe('none')
    expect(graph.querySelector('circle')).toBeNull()
  })

  it('highlights inactive connections on hover without adding particles', () => {
    const [hoveredNode, setHoveredNode] = createSignal<string | null>(null)
    const nodes = [{ id: 'provider', x: 0, y: 140, isActive: false, isHitting: false }]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={hoveredNode()} reducedMotion={false} />)
    const curve = view.container.querySelector('path')!

    expect(curve.getAttribute('stroke')).toBe('var(--control-border)')
    expect(curve.getAttribute('opacity')).toBe('1')
    expect(curve.getAttribute('stroke-width')).toBe('1.5')
    expect(curve.getAttribute('stroke-dasharray')).toBe('4 4')
    expect(view.container.querySelectorAll('path')).toHaveLength(1)
    setHoveredNode('provider')
    expect(view.container.querySelectorAll('path')).toHaveLength(2)
    expect(view.container.querySelector('circle')).toBeNull()
    setHoveredNode(null)
    expect(view.container.querySelectorAll('path')).toHaveLength(1)
  })

  it('shows one moving particle per hit and keeps static feedback with reduced motion', () => {
    const [reducedMotion, setReducedMotion] = createSignal(false)
    const [nodes, setNodes] = createSignal([{ id: 'provider', x: 240, y: 0, isActive: true, isHitting: true }])
    const view = render(() => <TopologyGraph nodes={nodes()} hoveredNode={null} reducedMotion={reducedMotion()} />)
    const motion = view.container.querySelector('animateMotion')!

    expect(view.container.querySelectorAll('path')).toHaveLength(2)
    expect(view.container.querySelectorAll('circle')).toHaveLength(1)
    expect(motion.getAttribute('path')).toBe(view.container.querySelector('path')!.getAttribute('d'))
    expect(motion.getAttribute('dur')).toBe('1.8s')
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
