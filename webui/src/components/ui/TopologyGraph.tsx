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
      <defs>
        {/* 高能激光高斯光晕滤镜 */}
        <filter id="topology-laser-glow" x="-100%" y="-100%" width="300%" height="300%">
          <feGaussianBlur in="SourceGraphic" stdDeviation="5" result="blur1" />
          <feGaussianBlur in="SourceGraphic" stdDeviation="10" result="blur2" />
          <feMerge>
            <feMergeNode in="blur2" />
            <feMergeNode in="blur1" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
        {/* 柔和导管发光滤镜 */}
        <filter id="topology-conduit-glow" x="-50%" y="-50%" width="200%" height="200%">
          <feGaussianBlur in="SourceGraphic" stdDeviation="3" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
      </defs>

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
              {/* 底层基准连线 */}
              <path
                d={path}
                fill="none"
                stroke={node.isActive ? 'var(--accent)' : 'var(--control-border)'}
                opacity={node.isActive ? (node.isHitting ? 0.2 : 0.45) : 1}
                stroke-width={node.isActive ? 3 : 2.5}
                stroke-linecap="round"
                stroke-dasharray={node.isActive ? 'none' : '6 7'}
              />

              {/* 激活/传输脉冲图层 */}
              <Show when={node.isHitting || props.hoveredNode === node.id}>
                <Show
                  when={node.isHitting}
                  fallback={
                    /* 悬停高亮线 */
                    <path
                      d={path}
                      fill="none"
                      stroke="var(--accent)"
                      stroke-width={3}
                      stroke-linecap="round"
                    />
                  }
                >
                  {/* 1. 全段高能等离子激光外晕 (Full-Segment Outer Laser Conduit Aura) */}
                  <path
                    d={path}
                    fill="none"
                    stroke="var(--accent)"
                    stroke-width={8}
                    stroke-linecap="round"
                    opacity={0.32}
                    filter="url(#topology-laser-glow)"
                  >
                    <Show when={!props.reducedMotion}>
                      <animate attributeName="opacity" values="0.25;0.52;0.25" dur="1.6s" repeatCount="indefinite" />
                    </Show>
                  </path>

                  {/* 2. 全段通电激光主导管 (Full-Segment Emissive Conduit) */}
                  <path
                    d={path}
                    fill="none"
                    stroke="var(--accent)"
                    stroke-width={4}
                    stroke-linecap="round"
                    opacity={0.8}
                  >
                    <Show when={!props.reducedMotion}>
                      <animate attributeName="stroke-width" values="3.5;4.8;3.5" dur="1.6s" repeatCount="indefinite" />
                    </Show>
                  </path>

                  {/* 3. 全段白热微光光丝 (White Hot Filament Core) */}
                  <path
                    d={path}
                    fill="none"
                    stroke="#ffffff"
                    stroke-width={1.2}
                    stroke-linecap="round"
                    opacity={0.7}
                  >
                    <Show when={!props.reducedMotion}>
                      <animate attributeName="opacity" values="0.5;0.9;0.5" dur="0.8s" repeatCount="indefinite" />
                    </Show>
                  </path>

                  {/* 4. 连续多级高能激光脉冲波阵 (Multi-Pulse Laser Wave Train) */}
                  <Show when={!props.reducedMotion}>
                    {/* 激光脉冲 1: 高速白光极强波束 */}
                    <path
                      d={path}
                      fill="none"
                      stroke="#ffffff"
                      pathLength={1}
                      stroke-width={3}
                      stroke-linecap="round"
                      stroke-dasharray="0.25 0.75"
                      filter="url(#topology-conduit-glow)"
                      opacity={0.95}
                    >
                      <animate attributeName="stroke-dashoffset" from="1" to="0" dur="0.95s" repeatCount="indefinite" />
                    </path>

                    {/* 激光脉冲 2: 紧随其后的彩色等离子波束 */}
                    <path
                      d={path}
                      fill="none"
                      stroke="var(--accent)"
                      pathLength={1}
                      stroke-width={5}
                      stroke-linecap="round"
                      stroke-dasharray="0.35 0.65"
                      opacity={0.7}
                    >
                      <animate attributeName="stroke-dashoffset" from="1.3" to="0.3" dur="0.95s" repeatCount="indefinite" />
                    </path>

                    {/* 5. 领航光子核 + 次级跟随光子核 (Dual Photon Sparks) */}
                    <g>
                      <animateMotion path={path} dur="0.95s" repeatCount="indefinite" />
                      <circle r={9} fill="var(--accent)" opacity={0.35} filter="url(#topology-conduit-glow)">
                        <animate attributeName="r" values="7;11;7" dur="0.5s" repeatCount="indefinite" />
                      </circle>
                      <circle r={4} fill="var(--accent)" opacity={0.9} />
                      <circle r={2} fill="#ffffff" opacity="1" />
                    </g>

                    <g>
                      <animateMotion path={path} dur="0.95s" begin="0.48s" repeatCount="indefinite" />
                      <circle r={6} fill="var(--accent)" opacity="0.3" />
                      <circle r={2.8} fill="#ffffff" opacity="0.85" />
                    </g>

                    {/* 6. 两端能量辐射波环 (Terminal Pulse Rings) */}
                    <circle cx={0} cy={0} r={14} fill="none" stroke="var(--accent)" stroke-width={2} opacity={0}>
                      <animate attributeName="r" values="8;24" dur="1.2s" repeatCount="indefinite" />
                      <animate attributeName="opacity" values="0.8;0" dur="1.2s" repeatCount="indefinite" />
                    </circle>
                    <circle cx={node.x} cy={node.y} r={14} fill="none" stroke="var(--accent)" stroke-width={2} opacity={0}>
                      <animate attributeName="r" values="8;26" dur="1.2s" repeatCount="indefinite" />
                      <animate attributeName="opacity" values="0.8;0" dur="1.2s" repeatCount="indefinite" />
                    </circle>
                  </Show>
                </Show>
              </Show>
            </g>
          )
        }}
      </For>
    </svg>
  )
}
