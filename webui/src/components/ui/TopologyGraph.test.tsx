import { cleanup, render } from '@solidjs/testing-library'
import { createSignal } from 'solid-js'
import { afterEach, describe, expect, it } from 'vitest'
import { TopologyGraph } from './TopologyGraph'

afterEach(cleanup)


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
  // Off-axis spokes keep the S shape (halves bend opposite ways). Vertical
  // spokes are intentionally straight: the lateral offset scales with x, so a
  // spoke with x=0 has no sideways component at all.
  it.each([
    { x:240, y: 0 },
    { x: -100, y: 150 },
    { x: -240, y: -100 },
    { x: 223, y: -72 },
  ])('draws an S-curve spoke with opposite-bending halves to ($x, $y)', ({ x, y }) => {
    const nodes = [{ id: 'provider', x, y, isActive: true, isHitting: false }]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={null} reducedMotion={false} />)
    const graph = view.container.querySelector('svg')!
    const d = firstPath(view.container)

    expect(graph.getAttribute('aria-hidden')).toBe('true')
    expect(d.startsWith('M 0 0 C ')).toBe(true)
    expect(d.endsWith(`${x} ${y}`)).toBe(true)
    const [first, second] = controlOffsets(d, x, y)
    // Bend direction follows which side of the ring the spoke points at, so the
    // figure is mirror-symmetric instead of chiral.
    expect(Math.sign(first)).toBe(Math.sign(x))
    expect(Math.sign(second)).toBe(-Math.sign(x))

    const line = graph.querySelector('path')!
    expect(line.getAttribute('stroke')).toBe('var(--accent)')
    expect(line.getAttribute('stroke-width')).toBe('3')
    expect(line.getAttribute('stroke-dasharray')).toBe('none')
    expect(graph.querySelector('circle')).toBeNull()
  })

  it('mirrors the bend across the vertical axis', () => {
    const nodes = [
      { id: 'r', x: 223, y: -72, isActive: true, isHitting: false },
      { id: 'l', x:-223, y: -72, isActive: true, isHitting: false },
    ]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={null} reducedMotion={false} />)
    const ds = Array.from(view.container.querySelectorAll('path')).map(p => p.getAttribute('d')!)
    const [right] = controlOffsets(ds[0], 223, -72)
    const [left] = controlOffsets(ds[1], -223, -72)
    // Equal magnitude, opposite sign: the pair is a reflection, not a rotation.
    expect(Math.abs(Math.abs(right) - Math.abs(left))).toBeLessThan(1)
    expect(Math.sign(right)).toBe(-Math.sign(left))
  })

  it('keeps a vertical spoke straight', () => {
    const nodes = [{ id: 'top', x: 0, y: -140, isActive: true, isHitting: false }]
    const view = render(() => <TopologyGraph nodes={nodes} hoveredNode={null} reducedMotion={false} />)
    const [first, second] = controlOffsets(firstPath(view.container), 0, -140)
    expect(first).toBe(0)
    expect(second).toBe(0)
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

  it('shows laser pulse beams and leading photon particle on hit, suppressing animation on reduced motion', () => {
    const [reducedMotion, setReducedMotion] = createSignal(false)
    const [nodes, setNodes] = createSignal([{ id: 'provider', x: 240, y: 0, isActive: true, isHitting: true }])
    const view = render(() => <TopologyGraph nodes={nodes()} hoveredNode={null} reducedMotion={reducedMotion()} />)
    const motion = view.container.querySelector('animateMotion')!

    expect(view.container.querySelectorAll('path').length).toBeGreaterThanOrEqual(2)
    expect(view.container.querySelectorAll('circle').length).toBeGreaterThanOrEqual(1)
    expect(motion).toBeTruthy()
    expect(motion.getAttribute('path')).toBe(firstPath(view.container))
    expect(motion.getAttribute('dur')).toBe('0.95s')
    expect(motion.getAttribute('repeatCount')).toBe('indefinite')

    // 开启减弱动效时，抑制动画元素
    setReducedMotion(true)
    expect(view.container.querySelector('animateMotion')).toBeNull()
    expect(view.container.querySelector('circle')).toBeNull()

    // 恢复动效
    setReducedMotion(false)
    expect(view.container.querySelector('animateMotion')).toBeTruthy()
    expect(view.container.querySelectorAll('circle').length).toBeGreaterThanOrEqual(1)

    // 传输结束，恢复基准连线
    setNodes(nodes => nodes.map(node => ({ ...node, isHitting: false })))
    expect(view.container.querySelectorAll('path')).toHaveLength(1)
    expect(view.container.querySelector('circle')).toBeNull()
  })
})

describe('TopologyGraph throughput cadence', () => {
  const renderHitting = (tokensPerSec?: number) => render(() => (
    <TopologyGraph
      nodes={[{ id: 'p', x: 240, y: 0, isActive: true, isHitting: true, tokensPerSec }]}
      hoveredNode={null}
      reducedMotion={false}
    />
  ))

  const motionDurs = (container: HTMLElement) =>
    Array.from(container.querySelectorAll('animateMotion')).map(el => el.getAttribute('dur')!)

  it('speeds the pulse up with throughput and stays inside the readable band', () => {
    const slow = motionDurs(renderHitting(10).container)
    const fast = motionDurs(renderHitting(1000).container)

    expect(slow.length).toBeGreaterThan(0)
    expect(parseFloat(slow[0])).toBeGreaterThan(parseFloat(fast[0]))

    // Clamped so neither extreme strobes nor crawls.
    for (const dur of [...slow, ...fast]) {
      const secs = parseFloat(dur)
      expect(secs).toBeGreaterThanOrEqual(0.35)
      expect(secs).toBeLessThanOrEqual(1.25)
    }
  })

  it('falls back to the default cadence when throughput is unknown', () => {
    expect(motionDurs(renderHitting(undefined).container)[0]).toBe('0.95s')
    expect(motionDurs(renderHitting(0).container)[0]).toBe('0.95s')
  })

  it('stagger the trailing pulses by a share of the live cycle', () => {
    const durs = motionDurs(renderHitting(500).container)
    const begins = Array.from(renderHitting(500).container.querySelectorAll('animateMotion'))
      .map(el => el.getAttribute('begin'))
      .filter((b): b is string => !!b)
    expect(begins.length).toBeGreaterThan(0)
    // Offsets shrink with the cycle instead of staying at the old fixed 0.32s.
    expect(parseFloat(begins[0])).toBeLessThan(parseFloat(durs[0]))
  })
})
