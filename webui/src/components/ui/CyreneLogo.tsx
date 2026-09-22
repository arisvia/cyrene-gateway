import { type Component, createUniqueId } from 'solid-js'

export interface CyreneLogoProps {
  class?: string
  size?: number
  pulsing?: boolean
}

export const CyreneLogo: Component<CyreneLogoProps> = props => {
  const uid = createUniqueId()
  const liquidId = `cyrene-liquid-${uid}`
  const specularId = `cyrene-specular-${uid}`
  const coreId = `cyrene-core-${uid}`

  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 32 32"
      fill="none"
      class={props.class}
      width={props.size}
      height={props.size}
      aria-label="Cyrene Gateway"
    >
      <defs>
        <linearGradient id={liquidId} x1="10%" y1="0%" x2="90%" y2="100%">
          <stop offset="0%" stop-color="var(--brand-mint)" />
          <stop offset="35%" stop-color="var(--brand-cyan)" />
          <stop offset="70%" stop-color="var(--brand-indigo)" />
          <stop offset="100%" stop-color="var(--brand-violet)" />
        </linearGradient>

        {/* Specular Bevel Reflection */}
        <linearGradient id={specularId} x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#ffffff" stop-opacity="0.9" />
          <stop offset="40%" stop-color="#ffffff" stop-opacity="0.25" />
          <stop offset="100%" stop-color="#ffffff" stop-opacity="0.0" />
        </linearGradient>

        {/* Router Singularity Core */}
        <linearGradient id={coreId} x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stop-color="#ffffff" />
          <stop offset="60%" stop-color="#38bdf8" />
          <stop offset="100%" stop-color="#0284c7" />
        </linearGradient>
      </defs>

      {/* Monolithic Hexagonal "C" Gateway Portal */}
      <path
        d="M 27.69 9.25 L 16 2.5 L 4.31 9.25 L 4.31 22.75 L 16 29.5 L 27.69 22.75 L 22.5 19.75 L 16 23.5 L 9.5 19.75 L 9.5 12.25 L 16 8.5 L 22.5 12.25 Z"
        fill={`url(#${liquidId})`}
      />

      {/* Top Specular Bevel Chamfer */}
      <path
        d="M 4.31 9.25 L 16 2.5 L 27.69 9.25"
        stroke={`url(#${specularId})`}
        stroke-width="0.8"
        stroke-linecap="round"
        stroke-linejoin="round"
      />

      {/* Central Router Ingress Diamond (60° Isometric Rhombus) */}
      <polygon
        points="16,13.5 18.17,16 16,18.5 13.83,16"
        fill={`url(#${coreId})`}
        class={props.pulsing ? 'animate-pulse' : ''}
      />
    </svg>
  )
}

export default CyreneLogo
