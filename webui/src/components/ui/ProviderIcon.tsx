import { type Component, Show } from 'solid-js'

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

// 统一品牌官方 SVG 矢量图标库（支持暗黑/明亮自适应）
export const ProviderBrandIcon: Component<{ provider: string; size?: number; class?: string }> = props => {
  const p = () => props.provider.toLowerCase().replace(/[-_]/g, '')
  const sz = () => props.size ?? 20

  return (
    <Show
      when={p()}
      fallback={<span class="font-bold">?</span>}
    >
      {/* OpenAI / ChatGPT */}
      <Show when={p().includes('openai') || p() === 'oa'}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M22.2819 9.8211a5.9847 5.9847 0 0 0-.5157-4.9108 6.0462 6.0462 0 0 0-6.5098-2.9A6.0651 6.0651 0 0 0 4.9807 4.1818a5.9847 5.9847 0 0 0-3.9977 2.9 6.0462 6.0462 0 0 0 .7427 7.0966 5.98 5.98 0 0 0 .511 4.9107 6.051 6.051 0 0 0 6.5146 2.9001A5.9847 5.9847 0 0 0 13.2599 24a6.0557 6.0557 0 0 0 5.7718-4.2058 5.9894 5.9894 0 0 0 3.9977-2.9001 6.0557 6.0557 0 0 0-.7475-7.0729zm-9.022 12.6081a4.4755 4.4755 0 0 1-2.8764-1.0408l.1419-.0804 4.7783-2.7582a.7948.7948 0 0 0 .3927-.6813v-6.7369l2.02 1.1686a.071.071 0 0 1 .038.052v5.5826a4.504 4.504 0 0 1-4.4945 4.4944zm-9.6607-4.1254a4.4708 4.4708 0 0 1-.5346-3.0137l.142.0852 4.783 2.7582a.7712.7712 0 0 0 .7806 0l5.8428-3.3685v2.3324a.0804.0804 0 0 1-.0332.0615L9.74 19.9502a4.4992 4.4992 0 0 1-6.1408-1.6464zM2.3408 7.8956a4.485 4.485 0 0 1 2.3655-1.9728V11.6a.7664.7664 0 0 0 .3879.6765l5.8144 3.3543-2.0201 1.1685a.0757.0757 0 0 1-.071 0l-4.8303-2.7865A4.504 4.504 0 0 1 2.3408 7.8956zm16.0993 3.8558L12.5973 8.3829l2.0201-1.1639a.0804.0804 0 0 1 .071 0l4.8303 2.7913a4.4944 4.4944 0 0 1-.6765 8.1042v-5.6772a.79.79 0 0 0-.4021-.6859zm2.0107-3.0231l-.142-.0852-4.7735-2.7818a.7759.7759 0 0 0-.7854 0L8.9071 9.2297V6.8974a.0662.0662 0 0 1 .0331-.0615l4.8966-2.8291a4.504 4.504 0 0 1 6.6027 4.8492zm-12.6413.7914l2.9248-1.688 2.9248 1.688v3.3732l-2.9248 1.688-2.9248-1.688z" />
        </svg>
      </Show>

      {/* Claude / Anthropic */}
      <Show when={p().includes('claude') || p().includes('anthropic')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M17.3 3H6.7C4.7 3 3 4.7 3 6.7v10.6C3 19.3 4.7 21 6.7 21h10.6c2 0 3.7-1.7 3.7-3.7V6.7C21 4.7 19.3 3 17.3 3zm-2.8 13.8l-1-2.9h-3l-1 2.9H7.8L11 7.2h2l3.2 9.6h-1.7zm-2.5-4.4l-1-2.9-1 2.9h2z" />
        </svg>
      </Show>

      {/* Google / Gemini / Vertex */}
      <Show when={p().includes('gemini') || p().includes('google') || p().includes('vertex')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M12 24C12 17.3726 17.3726 12 24 12C17.3726 12 12 6.62742 12 0C12 6.62742 6.62742 12 0 12C6.62742 12 12 17.3726 12 24Z" />
        </svg>
      </Show>

      {/* OpenCode */}
      <Show when={p().includes('opencode') || p() === 'oc'}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M8.5 4a1.5 1.5 0 0 0-1.06.44l-5 5a1.5 1.5 0 0 0 0 2.12l5 5a1.5 1.5 0 1 0 2.12-2.12L5.62 10.5l3.94-3.94A1.5 1.5 0 0 0 8.5 4zm7 0a1.5 1.5 0 0 0-1.06 2.56l3.94 3.94-3.94 3.94a1.5 1.5 0 1 0 2.12 2.12l5-5a1.5 1.5 0 0 0 0-2.12l-5-5A1.5 1.5 0 0 0 15.5 4zM13.8 5.1a1.5 1.5 0 0 0-1.9 1l-3 10a1.5 1.5 0 1 0 2.86.86l3-10a1.5 1.5 0 0 0-.96-1.86z" />
        </svg>
      </Show>

      {/* DeepSeek */}
      <Show when={p().includes('deepseek')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M12 2a10 10 0 0 0-9.95 9h11.64L9.4 6.71a1 1 0 0 1 1.41-1.42l6 6a1 1 0 0 1 0 1.42l-6 6a1 1 0 0 1-1.41-1.42l4.29-4.29H2.05A10 10 0 1 0 12 2z" />
        </svg>
      </Show>

      {/* GitHub / Copilot */}
      <Show when={p().includes('github') || p().includes('copilot')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path fill-rule="evenodd" clip-rule="evenodd" d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
        </svg>
      </Show>

      {/* OpenRouter */}
      <Show when={p().includes('openrouter')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
        </svg>
      </Show>

      {/* Qoder */}
      <Show when={p().includes('qoder')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm1 14.5h-2v-2h2v2zm0-4h-2V7h2v5.5z" />
        </svg>
      </Show>

      {/* Groq */}
      <Show when={p().includes('groq')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-width="2.5" fill="none" />
          <path d="M12 7v5l3 3" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
        </svg>
      </Show>

      {/* Kimi / Moonshot */}
      <Show when={p().includes('kimi') || p().includes('moonshot')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M12 3a9 9 0 1 0 9 9c0-.46-.04-.92-.1-1.36a5.389 5.389 0 0 1-4.4 2.26 5.403 5.403 0 0 1-3.14-9.8A9 9 0 0 0 12 3z" />
        </svg>
      </Show>

      {/* GLM / Zhipu */}
      <Show when={p().includes('glm') || p().includes('zhipu')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M12 2L2 12h5v8h10v-8h5L12 2z" />
        </svg>
      </Show>

      {/* Minimax */}
      <Show when={p().includes('minimax')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <rect x="3" y="6" width="18" height="12" rx="3" stroke="currentColor" stroke-width="2" fill="none" />
          <circle cx="8" cy="12" r="2" fill="currentColor" />
          <circle cx="16" cy="12" r="2" fill="currentColor" />
        </svg>
      </Show>

      {/* Tencent / Hunyuan */}
      <Show when={p().includes('tencent') || p().includes('hunyuan')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 15h-2v-6h2v6zm4 0h-2V7h2v10z" />
        </svg>
      </Show>

      {/* Mistral */}
      <Show when={p().includes('mistral')}>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="currentColor" class={props.class}>
          <path d="M4 4h4v4H4zm12 0h4v4h-4zm-8 4h8v4H8zm-4 4h4v4H4zm12 0h4v4h-4zm-8 4h8v4H8z" />
        </svg>
      </Show>

      {/* Default fallback icon */}
      <Show when={
        !p().includes('openai') && !p().includes('claude') && !p().includes('anthropic') &&
        !p().includes('gemini') && !p().includes('google') && !p().includes('vertex') &&
        !p().includes('opencode') && !p().includes('deepseek') && !p().includes('github') &&
        !p().includes('copilot') && !p().includes('openrouter') && !p().includes('qoder') &&
        !p().includes('groq') && !p().includes('kimi') && !p().includes('glm') &&
        !p().includes('minimax') && !p().includes('tencent') && !p().includes('mistral')
      }>
        <svg width={sz()} height={sz()} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={props.class}>
          <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
          <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
          <line x1="6" y1="6" x2="6.01" y2="6" />
          <line x1="6" y1="18" x2="6.01" y2="18" />
        </svg>
      </Show>
    </Show>
  )
}

// 映射已有 public/providers/*.png 资源
const PROVIDER_IMAGE_MAP: Record<string, string> = {
  openai: '/providers/openai.png',
  claude: '/providers/claude.png',
  anthropic: '/providers/anthropic.png',
  gemini: '/providers/gemini.png',
  google: '/providers/gemini.png',
  vertex: '/providers/vertex.png',
  opencode: '/providers/opencode.png',
  deepseek: '/providers/deepseek.png',
  copilot: '/providers/copilot.png',
  github: '/providers/github.png',
  openrouter: '/providers/openrouter.png',
  qoder: '/providers/qoder.png',
  groq: '/providers/groq.png',
  kimi: '/providers/kimi.png',
  glm: '/providers/glm.png',
  minimax: '/providers/minimax.png',
  mistral: '/providers/mistral.png',
  ollama: '/providers/ollama.png',
  siliconflow: '/providers/siliconflow.png',
  together: '/providers/together.png',
  cohere: '/providers/cohere.png',
  perplexity: '/providers/perplexity.png',
  huggingface: '/providers/huggingface.png',
  cerebras: '/providers/cerebras.png',
  xai: '/providers/xai.png',
  grok: '/providers/xai.png',
  novita: '/providers/novita.png',
  fireworks: '/providers/fireworks.png',
  elevenlabs: '/providers/elevenlabs.png',
  fal: '/providers/fal-ai.png',
  cursor: '/providers/cursor.png',
  cline: '/providers/cline.png',
  roo: '/providers/roo.png',
  alicode: '/providers/alicode.png',
  kilocode: '/providers/kilocode.png',
}

// 容器化 Provider Avatar 组件（支持 public PNG 图像或品牌色渐变背景 SVG）
export const ProviderAvatar: Component<ProviderIconProps> = props => {
  const sizeClass = () => SIZES[props.size ?? 'md']
  const iconPx = () => ICON_SIZES[props.size ?? 'md']
  const name = () => props.name || props.provider
  const normalized = () => props.provider.toLowerCase().replace(/[-_]/g, '')
  const imgSrc = () => {
    const key = Object.keys(PROVIDER_IMAGE_MAP).find(k => normalized().includes(k))
    return key ? PROVIDER_IMAGE_MAP[key] : null
  }

  return (
    <div
      class={`shrink-0 flex items-center justify-center rounded-xl overflow-hidden shadow-sm transition-transform group-hover:scale-105 bg-card border border-subtle ${sizeClass()} ${props.class ?? ''}`}
      style={{
        background: imgSrc() ? 'transparent' : (props.color || 'var(--gradient)'),
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
          class="w-full h-full object-contain p-1 rounded-xl"
          onError={e => {
            // 图片加载失败时隐藏并显示矢量 fallback
            (e.currentTarget as HTMLElement).style.display = 'none'
          }}
        />
      </Show>
    </div>
  )
}
