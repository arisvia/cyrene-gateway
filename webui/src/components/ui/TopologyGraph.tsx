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
          <feGaussianBlur in="SourceGraphic" stdDeviation="9" result="blur2" />
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
        {/* 为每根辐条生成独立的沿线多色色谱渐变：青蓝 -> 天蓝 -> 霓虹紫 -> 洋红 -> 琥珀金 */}
        <For each={props.nodes}>
          {node => (
            <linearGradient id={`chroma-grad-${node.id}`} x1="0" y1="0" x2={node.x} y2={node.y} gradientUnits="userSpaceOnUse">
              <stop offset="0%" stop-color="#00f2fe" />
              <stop offset="30%" stop-color="#4facfe" />
              <stop offset="60%" stop-color="#a855f7" />
              <stop offset="85%" stop-color="#f43f5e" />
              <stop offset="100%" stop-color="#fbbf24" />
            </linearGradient>
          )}
        </For>
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
                  {/* 1. 全段多色等离子外晕 (Multi-Color Chroma Glow Aura) */}
                  <path
                    d={path}
                    fill="none"
                    stroke={`url(#chroma-grad-${node.id})`}
                    stroke-width={10}
                    stroke-linecap="round"
                    opacity={0.38}
                    filter="url(#topology-laser-glow)"
                  >
                    <Show when={!props.reducedMotion}>
                      <animate attributeName="opacity" values="0.28;0.58;0.28" dur="1.6s" repeatCount="indefinite" />
                    </Show>
                  </path>

                  {/* 2. 全段多色激光主导管 (Chroma Laser Conduit) */}
                  <path
                    d={path}
                    fill="none"
                    stroke={`url(#chroma-grad-${node.id})`}
                    stroke-width={4.5}
                    stroke-linecap="round"
                    opacity={0.9}
                  >
                    <Show when={!props.reducedMotion}>
                      <animate attributeName="stroke-width" values="4;5.2;4" dur="1.6s" repeatCount="indefinite" />
                    </Show>
                  </path>

                  {/* 3. 全段白热微光核心丝 (Core Hot Filament) */}
                  <path
                    d={path}
                    fill="none"
                    stroke="#ffffff"
                    stroke-width={1.4}
                    stroke-linecap="round"
                    opacity={0.8}
                  >
                    <Show when={!props.reducedMotion}>
                      <animate attributeName="opacity" values="0.6;0.95;0.6" dur="0.8s" repeatCount="indefinite" />
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
                      stroke-width={3.5}
                      stroke-linecap="round"
                      stroke-dasharray="0.22 0.78"
                      opacity={0.95}
                    >
                      <animate attributeName="stroke-dashoffset" from="1" to="0" dur="0.95s" repeatCount="indefinite" />
                    </path>

                    {/* 激光脉冲 2: 紧随其后的多色等离子波束 */}
                    <path
                      d={path}
                      fill="none"
                      stroke={`url(#chroma-grad-${node.id})`}
                      pathLength={1}
                      stroke-width={6}
                      stroke-linecap="round"
                      stroke-dasharray="0.36 0.64"
                      opacity={0.8}
                    >
                      <animate attributeName="stroke-dashoffset" from="1.3" to="0.3" dur="0.95s" repeatCount="indefinite" />
                    </path>

                    {/* 5. 领航光子核 1: 霓虹青白粒子 */}
                    <g>
                      <animateMotion path={path} dur="0.95s" repeatCount="indefinite" />
                      <circle r={9} fill="#00f2fe" opacity="0.45" filter="url(#topology-conduit-glow)">
                        <animate attributeName="r" values="7;12;7" dur="0.5s" repeatCount="indefinite" />
                      </circle>
                      <circle r={4.5} fill="#00f2fe" opacity="0.95" />
                      <circle r={2.2} fill="#ffffff" opacity="1" />
                    </g>

                    {/* 6. 次级光子核 2: 洋红紫光粒子 */}
                    <g>
                      <animateMotion path={path} dur="0.95s" begin="0.32s" repeatCount="indefinite" />
                      <circle r={7} fill="#a855f7" opacity="0.4" filter="url(#topology-conduit-glow)">
                        <animate attributeName="r" values="5;9;5" dur="0.5s" repeatCount="indefinite" />
                      </circle>
                      <circle r={3.8} fill="#f43f5e" opacity="0.95" />
                      <circle r={1.8} fill="#ffffff" opacity="1" />
                    </g>

                    {/* 7. 尾随光子核 3: 金黄光子 */}
                    <g>
                      <animateMotion path={path} dur="0.95s" begin="0.64s" repeatCount="indefinite" />
                      <circle r={6} fill="#fbbf24" opacity="0.35" />
                      <circle r={3} fill="#f59e0b" opacity="0.9" />
                      <circle r={1.5} fill="#ffffff" opacity="1" />
                    </g>

                    {/* 8. 两端能量辐射环: 核心端青色 + 目标端金黄色 */}
                    <circle cx={0} cy={0} r={14} fill="none" stroke="#00f2fe" stroke-width={2} opacity={0}>
                      <animate attributeName="r" values="8;24" dur="1.2s" repeatCount="indefinite" />
                      <animate attributeName="opacity" values="0.8;0" dur="1.2s" repeatCount="indefinite" />
                    </circle>
                    <circle cx={node.x} cy={node.y} r={14} fill="none" stroke="#fbbf24" stroke-width={2} opacity={0}>
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
