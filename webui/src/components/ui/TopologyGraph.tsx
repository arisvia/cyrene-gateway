import { For, Show, type JSX } from 'solid-js'

interface TopologyGraphProps {
  nodes: { id: string; x: number; y: number; isActive: boolean; isHitting: boolean }[]
  hoveredNode: string | null
  reducedMotion: boolean
}

export function TopologyGraph(props: TopologyGraphProps): JSX.Element {
  return (
    <svg
      aria-hidden="true"
      class="absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 w-[1200px] h-[1200px] pointer-events-none overflow-visible -z-10"
      viewBox="-600 -600 1200 1200"
    >
      <For each={props.nodes}>
        {node => {
          let path: string
          if (Math.abs(node.y) >= Math.abs(node.x)) {
            // Keep vertical connections curved even when the node is on the axis.
            const arcX = node.x === 0 ? 18 : 0
            const cp1x = Math.round(node.x * 0.25 + arcX)
            const cp1y = Math.round(node.y * 0.5)
            const cp2x = Math.round(node.x * 0.85 + (node.x === 0 ? 8 : 0))
            const cp2y = Math.round(node.y * 0.6)
            path = `M 0 0 C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${node.x} ${node.y}`
          } else {
            // Apply the same axis offset for horizontal connections.
            const arcY = node.y === 0 ? 14 : 0
            const cp1x = Math.round(node.x * 0.5)
            const cp1y = Math.round(node.y * 0.25 + arcY)
            const cp2x = Math.round(node.x * 0.6)
            const cp2y = Math.round(node.y * 0.85 + (node.y === 0 ? 6 : 0))
            path = `M 0 0 C ${cp1x} ${cp1y}, ${cp2x} ${cp2y}, ${node.x} ${node.y}`
          }

          return (
            <g class="transition-opacity duration-200">
              <path
                d={path}
                fill="none"
                stroke={node.isActive ? 'var(--accent)' : 'var(--control-border)'}
                opacity={node.isActive ? 0.3 : 1}
                stroke-width={node.isActive ? 2 : 1.5}
                stroke-dasharray={node.isActive ? 'none' : '4 4'}
              />
              <Show when={node.isHitting || props.hoveredNode === node.id}>
                <path d={path} fill="none" stroke="var(--accent)" stroke-width="2" stroke-linecap="round" />
                <Show when={node.isHitting && !props.reducedMotion}>
                  <circle r="3" fill="var(--accent)">
                    <animateMotion path={path} dur="1.8s" repeatCount="indefinite" />
                  </circle>
                </Show>
              </Show>
            </g>
          )
        }}
      </For>
    </svg>
  )
}
