import { type Component, type JSX, splitProps } from 'solid-js'

export interface IconProps extends JSX.SvgSVGAttributes<SVGSVGElement> {
  size?: number | string
  class?: string
}

const NAMED_SIZES: Record<string, number> = {
  xs: 12,
  sm: 14,
  md: 16,
  lg: 20,
  xl: 24,
}

function baseAttrs(props: IconProps, defaultSize = 16) {
  let sz: number | string = props.size ?? defaultSize
  if (typeof sz === 'string' && sz in NAMED_SIZES) {
    sz = NAMED_SIZES[sz]
  }
  return {
    'width': sz,
    'height': sz,
    'viewBox': '0 0 24 24',
    'fill': 'none',
    'stroke': 'currentColor',
    'stroke-width': '2',
    'stroke-linecap': 'round' as const,
    'stroke-linejoin': 'round' as const,
    'aria-hidden': true as const,
  }
}
// LLM 对话
export const IconChat: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z" />
    </svg>
  )
}
// 图像生成
export const IconPalette: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" />
      <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" />
      <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" />
      <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" />
      <path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c.926 0 1.648-.746 1.648-1.688 0-.437-.18-.835-.437-1.125-.29-.289-.438-.652-.438-1.125a1.64 1.64 0 0 1 1.668-1.668h1.996c3.051 0 5.555-2.503 5.555-5.554C21.965 6.012 17.461 2 12 2z" />
    </svg>
  )
}
// 语音合成 (TTS)
export const IconVolume: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
      <path d="M15.54 8.46a5 5 0 0 1 0 7.07" />
      <path d="M19.07 4.93a10 10 0 0 1 0 14.14" />
    </svg>
  )
}
// 语音识别 (STT)
export const IconMic: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z" />
      <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
      <line x1="12" y1="19" x2="12" y2="22" />
    </svg>
  )
}
// 视频生成
export const IconVideo: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <rect width="20" height="20" x="2" y="2" rx="2.18" ry="2.18" />
      <line x1="7" y1="2" x2="7" y2="22" />
      <line x1="17" y1="2" x2="17" y2="22" />
      <line x1="2" y1="12" x2="22" y2="12" />
      <line x1="2" y1="7" x2="7" y2="7" />
      <line x1="2" y1="17" x2="7" y2="17" />
      <line x1="17" y1="17" x2="22" y2="17" />
      <line x1="17" y1="7" x2="22" y2="7" />
    </svg>
  )
}
// 文本向量 (Embedding)
export const IconVector: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="6" cy="6" r="3" />
      <circle cx="18" cy="18" r="3" />
      <line x1="8.5" y1="8.5" x2="15.5" y2="15.5" />
      <path d="m11 7 4 4" />
      <path d="m13 17 4-4" />
    </svg>
  )
}
// 搜索
export const IconSearch: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  )
}
// 网页/网络
export const IconGlobe: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="12" cy="12" r="10" />
      <line x1="2" y1="12" x2="22" y2="12" />
      <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
    </svg>
  )
}
// 闪电 (通用能力/加速)
export const IconZap: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2" />
    </svg>
  )
}
// 提示/灵感 (Bulb)
export const IconBulb: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5" />
      <path d="M9 18h6" />
      <path d="M10 22h4" />
    </svg>
  )
}
// 插头/连接 (Plug)
export const IconPlug: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M12 22v-5" />
      <path d="M9 8V2" />
      <path d="M15 8V2" />
      <path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z" />
    </svg>
  )
}
// 星星/AI 魔法 (Sparkles)
export const IconSparkles: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="m12 3-1.912 5.813a2 2 0 0 1-1.275 1.275L3 12l5.813 1.912a2 2 0 0 1 1.275 1.275L12 21l1.912-5.813a2 2 0 0 1 1.275-1.275L21 12l-5.813-1.912a2 2 0 0 1-1.275-1.275L12 3Z" />
      <path d="M5 3v4" />
      <path d="M19 17v4" />
      <path d="M3 5h4" />
      <path d="M17 19h4" />
    </svg>
  )
}
// 设置/齿轮 (Gear)
export const IconSettings: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}
// 锁定 (Lock)
export const IconLock: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
      <path d="M7 11V7a5 5 0 0 1 10 0v4" />
    </svg>
  )
}
// 钥匙 (Key / API Key / 凭证)
export const IconKey: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="7.5" cy="15.5" r="5.5" />
      <path d="m21 2-9.6 9.6" />
      <path d="m15.5 7.5 3 3L22 7l-3-3" />
    </svg>
  )
}
// 盾牌/安全防护 (Shield)
export const IconShield: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
    </svg>
  )
}
// 编辑/修改 (Edit)
export const IconEdit: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z" />
    </svg>
  )
}
// 剪贴板/复制 (Clipboard)
export const IconClipboard: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  )
}
// 勾选/成功 (Check)
export const IconCheck: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <polyline points="20 6 9 17 4 12" />
    </svg>
  )
}
// 叉/关闭/移除 (X)
export const IconClose: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <line x1="18" y1="6" x2="6" y2="18" />
      <line x1="6" y1="6" x2="18" y2="18" />
    </svg>
  )
}
// 汉堡菜单 (Menu)
export const IconMenu: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <line x1="4" y1="12" x2="20" y2="12" />
      <line x1="4" y1="6" x2="20" y2="6" />
      <line x1="4" y1="18" x2="20" y2="18" />
    </svg>
  )
}
// 展开/向下箭头 (ChevronDown / ▼)
export const IconChevronDown: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <polyline points="6 9 12 15 18 9" />
    </svg>
  )
}
// 收起/向右箭头 (ChevronRight / ▶)
export const IconChevronRight: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <polyline points="9 18 15 12 9 6" />
    </svg>
  )
}
// 向上箭头 (ChevronUp / ▲)
export const IconChevronUp: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <polyline points="18 15 12 9 6 15" />
    </svg>
  )
}
// 向左箭头 (ChevronLeft / ◀)
export const IconChevronLeft: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <polyline points="15 18 9 12 15 6" />
    </svg>
  )
}
// 右箭头 (ArrowRight / →)
export const IconArrowRight: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <line x1="5" y1="12" x2="19" y2="12" />
      <polyline points="12 5 19 12 12 19" />
    </svg>
  )
}
// 外部链接箭头 (ExternalLink / ↗)
export const IconExternalLink: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
      <polyline points="15 3 21 3 21 9" />
      <line x1="10" y1="14" x2="21" y2="3" />
    </svg>
  )
}
// 信息 (Info)
export const IconInfo: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="12" cy="12" r="10" />
      <line x1="12" y1="16" x2="12" y2="12" />
      <line x1="12" y1="8" x2="12.01" y2="8" />
    </svg>
  )
}
// 警告圆圈/错误 (AlertCircle)
export const IconAlertCircle: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <circle cx="12" cy="12" r="10" />
      <line x1="12" y1="8" x2="12" y2="12" />
      <line x1="12" y1="16" x2="12.01" y2="16" />
    </svg>
  )
}
// 警告三角 (AlertTriangle)
export const IconAlertTriangle: Component<IconProps> = props => {
  const [local, others] = splitProps(props, ['size', 'class'])
  return (
    <svg {...baseAttrs(local)} class={local.class} {...others}>
      <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
      <line x1="12" y1="9" x2="12" y2="13" />
      <line x1="12" y1="17" x2="12.01" y2="17" />
    </svg>
  )
}
