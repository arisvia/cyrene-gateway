import { type Component, Show, createSignal, createEffect } from 'solid-js'
import { IconServer } from './icons'
interface ProviderIconProps {
  provider: string
  name?: string
  color?: string
  size?: 'sm' | 'md' | 'lg' | 'xl'
  class?: string
}

const SIZES = {
  sm: 'w-6 h-6 rounded-lg text-[10px]',
  md: 'w-9 h-9 rounded-xl text-xs',
  lg: 'w-11 h-11 rounded-xl text-sm',
  xl: 'w-14 h-14 rounded-2xl text-base',
}

const ICON_SIZES = {
  sm: 14,
  md: 20,
  lg: 24,
  xl: 32,
}

// 统一未知/自定义 Provider 优雅降级徽标（已内嵌全量 39 家官方 SVG，兜底专注未知自定义源）
export const ProviderBrandIcon: Component<{ provider: string; name?: string; size?: number; class?: string }> = props => {
  const label = () => (props.name || props.provider || '?').trim()
  const initials = () => {
    const l = label()
    return l.length <= 2 ? l.toUpperCase() : l.slice(0, 2).toUpperCase()
  }
  const sz = () => props.size ?? 20

  return (
    <Show
      when={label() !== '?'}
      fallback={
        <IconServer size={sz()} class={props.class} />
      }
    >
      <span class={`font-bold font-mono select-none tracking-tighter text-foreground/90 ${props.class ?? ''}`}>
        {initials()}
      </span>
    </Show>
  )
}

// 编译期内嵌所有 Provider SVG 为 Data URL，彻底断绝离线或网关停止时的网络请求与图标变色丢失
const RAW_SVGS = import.meta.glob<string>('../../assets/providers/*.svg', {
  query: '?raw',
  import: 'default',
  eager: true,
})

const PROVIDER_DATA_URLS: Record<string, string> = {}
for (const [path, content] of Object.entries(RAW_SVGS)) {
  const match = path.match(/\/([^/]+)\.svg$/)
  if (match && match[1]) {
    PROVIDER_DATA_URLS[match[1]] = `data:image/svg+xml;utf8,${encodeURIComponent(content)}`
  }
}

// 映射当前活跃 provider 与 cli 工具的静态 SVG 资产
const getProviderAsset = (key: string): string | undefined => {
  return PROVIDER_DATA_URLS[key] || `/providers/${key}.svg`
}

const PROVIDER_IMAGE_MAP: Record<string, string> = {
  'alicode-intl': getProviderAsset('alicode')!,
  alicode: getProviderAsset('alicode')!,
  'codebuddy-intl': getProviderAsset('codebuddy')!,
  'codebuddy-cn': getProviderAsset('codebuddy')!,
  codebuddy: getProviderAsset('codebuddy')!,
  'glm-cn': getProviderAsset('glm')!,
  glm: getProviderAsset('glm')!,
  'minimax-cn': getProviderAsset('minimax')!,
  minimax: getProviderAsset('minimax')!,
  'grok-cli': getProviderAsset('grok-cli')!,
  grok: getProviderAsset('grok-cli')!,
  deepseek: getProviderAsset('deepseek')!,
  dsh: getProviderAsset('dsh')!,
  openai: getProviderAsset('openai')!,
  claude: getProviderAsset('claude')!,
  anthropic: getProviderAsset('anthropic')!,
  gemini: getProviderAsset('gemini')!,
  google: getProviderAsset('gemini')!,
  vertex: getProviderAsset('vertex')!,
  opencode: getProviderAsset('opencode')!,
  copilot: getProviderAsset('copilot')!,
  github: getProviderAsset('github')!,
  openrouter: getProviderAsset('openrouter')!,
  qoder: getProviderAsset('qoder')!,
  groq: getProviderAsset('groq')!,
  kimi: getProviderAsset('kimi')!,
  cerebras: getProviderAsset('cerebras')!,
  nvidia: getProviderAsset('nvidia')!,
  xai: getProviderAsset('xai')!,
  cursor: getProviderAsset('cursor')!,
  cline: getProviderAsset('cline')!,
  roo: getProviderAsset('roo')!,
  continue: getProviderAsset('continue')!,
  antigravity: getProviderAsset('antigravity')!,
  aider: getProviderAsset('aider')!,
  windsurf: getProviderAsset('windsurf')!,
  trae: getProviderAsset('trae')!,
  tencent: getProviderAsset('tencent')!,
  codex: getProviderAsset('codex')!,
  'stability-ai': getProviderAsset('stability')!,
  stability: getProviderAsset('stability')!,
  'brave-search': getProviderAsset('brave')!,
  brave: getProviderAsset('brave')!,
  tavily: getProviderAsset('tavily')!,
  exa: getProviderAsset('exa')!,
  firecrawl: getProviderAsset('firecrawl')!,
  elevenlabs: getProviderAsset('elevenlabs')!,
  deepgram: getProviderAsset('deepgram')!,
}

// 容器化 Provider Avatar 组件（支持 public SVG 图像或品牌色渐变背景 SVG）
export const ProviderAvatar: Component<ProviderIconProps> = props => {
  const sizeClass = () => SIZES[props.size ?? 'md']
  const iconPx = () => ICON_SIZES[props.size ?? 'md']
  const name = () => props.name || props.provider
  const normalized = () => props.provider.toLowerCase().replace(/[-_]/g, '')
  const [imgFailed, setImgFailed] = createSignal(false)

  // 当 provider 属性变化时自动复位失败状态
  createEffect(() => {
    props.provider
    setImgFailed(false)
  })

  const imgSrc = () => {
    if (imgFailed()) return null
    const norm = normalized()
    const key = Object.keys(PROVIDER_IMAGE_MAP).find(k => norm.includes(k.replace(/[-_]/g, '')))
    if (key && PROVIDER_IMAGE_MAP[key]) return PROVIDER_IMAGE_MAP[key]
    const dataKey = Object.keys(PROVIDER_DATA_URLS).find(k => norm.includes(k.replace(/[-_]/g, '')))
    return dataKey ? PROVIDER_DATA_URLS[dataKey] : null
  }
  return (
    <div
      class={`shrink-0 flex items-center justify-center rounded-xl overflow-hidden glass-avatar transition-transform group-hover:scale-105 ${sizeClass()} ${props.class ?? ''}`}
      style={{
        background: imgSrc() ? undefined : (props.color || 'var(--gradient)'),
      }}
      title={name()}
    >
      <Show
        when={imgSrc()}
        fallback={<ProviderBrandIcon provider={props.provider} size={iconPx()} />}
      >
        <img
          src={imgSrc()!}
          alt={name()}
          class="w-full h-full object-contain p-1.5 rounded-xl filter drop-shadow-xs"
          onError={() => setImgFailed(true)}
        />
      </Show>
    </div>
  )
}
