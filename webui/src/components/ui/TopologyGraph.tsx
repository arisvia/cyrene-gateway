import { For, Show, type JSX } from 'solid-js'

interface TopologyGraphProps {
  nodes: { id: string; x: number; y: number; isActive: boolean; isHitting: boolean }[]
  hoveredNode: string | null
  reducedMotion: boolean
}

// S 形摆臂辐条：从核心沿径向起手，前 1/3 沿旋转方向偏移 bend，
// 后 1/3 以 1.3 倍幅度反向回摆——起段含蓄、摆尾开阔，五条线整体仍严格旋转对称。
// 线身藏在核心卡片与节点卡片之下，可见段恰好是两个卡片的边缘间隙。
export function TopologyGraph(props: TopologyGraphProps): JSX.Element {
  return (
    <svg
      aria-hidden="true"
      class="absolute top-0 left-0 -translate-x-1/2 -translate-y-1/2 w-[1200px] h-[1200px] pointer-events-none overflow-visible -z-10"
      viewBox="-600,-600,1200,1200"
    >
      <For each={props.nodes}>
        {node => {
          const len = Math.hypot(node.x, node.y) || 1
          const bend = Math.max(20, Math.round(len * 0.11))
          const nx = node.y / len
          const ny = -node.x / len
          const c1x = Math.round(node.x / 3 - nx * bend)
          const c1y = Math.round(node.y / 3 - ny * bend)
          const c2x = Math.round(node.x * 2 / 3 + nx * bend * 1.3)
          const c2y = Math.round(node.y * 2 / 3 + ny * bend * 1.3)
          const path = `M 0 0 C ${c1x} ${c1y}, ${c2x} ${c2y}, ${node.x} ${node.y}`
          return (
            <g class="transition-opacity duration-200">
              <path
                d={path}
                fill="none"
                stroke={node.isActive ? 'var(--accent)' : 'var(--control-border)'}
                opacity={node.isActive ? 0.45 : 1}
                stroke-width={node.isActive ? 3 : 2.5}
                stroke-linecap="round"
                stroke-dasharray={node.isActive ? 'none' : '6 7'}
              />
              <Show when={node.isHitting || props.hoveredNode === node.id}>
                <path
                  d={path}
                  fill="none"
                  stroke="var(--accent)"
                  pathLength={1}
                  stroke-width={node.isHitting ? 4 : 3}
                  stroke-linecap="round"
                  stroke-dasharray={node.isHitting && !props.reducedMotion ? '0.12 0.88' : 'none'}
                >
                  <Show when={node.isHitting && !props.reducedMotion}>
                    <animate attributeName="stroke-dashoffset" from="1" to="0" dur="1.35s" repeatCount="indefinite" />
                  </Show>
                </path>
                <Show when={node.isHitting && !props.reducedMotion}>
                  <circle r="4.5" fill="var(--accent)" opacity="0.95">
                    <animate attributeName="r" values="3.5;5.5;3.5" dur="1.35s" repeatCount="indefinite" />
                    <animateMotion path={path} dur="1.35s" repeatCount="indefinite" />
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
