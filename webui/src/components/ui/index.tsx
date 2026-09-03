import { type Component, type JSX, For, Show, createSignal, onMount, onCleanup } from 'solid-js'
import { useToast } from '@/lib/toast'
export { ProviderAvatar, ProviderBrandIcon } from './ProviderIcon'

export const Card: Component<{
  class?: string
  hover?: boolean
  onClick?: () => void
  children?: JSX.Element
  ref?: HTMLDivElement | ((el: HTMLDivElement) => void)
}> = props => (
  <div
    ref={props.ref}
    class={`rounded-card glass-card ${props.hover ? 'hover:bg-hover hover:border-accent/40 hover:-translate-y-0.5 hover:shadow-glass-hover' : ''} transition-all duration-200 ${props.class ?? ''}`}
    onClick={props.onClick}
  >
    {props.children}
  </div>
)

export const Badge: Component<{ tone?: 'green' | 'amber' | 'red' | 'gray' | 'blue'; class?: string; children?: JSX.Element }> = props => {
  const tones: Record<string, string> = {
    green: 'text-success bg-success/10',
    amber: 'text-warning bg-warning/10',
    red: 'text-danger bg-danger/10',
    blue: 'text-info bg-info/10',
    gray: 'text-faint bg-hover',
  }
  return (
    <span class={`inline-flex items-center px-2 py-0.5 rounded-full text-[11px] font-medium ${tones[props.tone ?? 'gray']} ${props.class ?? ''}`}>
      {props.children}
    </span>
  )
}

export const Empty: Component<{ message: string; children?: JSX.Element }> = props => (
  <div class="py-16 text-center text-sm text-faint">{props.message}</div>
)

export const Spinner: Component = () => (
  <div class="h-4 w-4 shrink-0 rounded-full border-2 border-subtle border-t-accent animate-spin" aria-hidden="true" />
)

export function ToastHost() {
  const { toasts } = useToast()
  return (
    <div class="fixed top-4 right-4 z-[80] space-y-2" role="status" aria-live="polite">
      <For each={toasts()}>
        {t => (
          <div
            class={`max-w-sm px-4 py-2.5 rounded-xl text-sm shadow-lg border backdrop-blur-xl animate-slide-up ${t.kind === 'success'
              ? 'border-success/30 bg-success/10 text-success'
              : t.kind === 'error'
                ? 'border-danger/30 bg-danger/10 text-danger'
                : 'border-subtle bg-bg-elevated text-text'}`}
          >
            {t.message}
          </div>
        )}
      </For>
    </div>
  )
}

export const Button: Component<{
  variant?: 'primary' | 'ghost' | 'danger' | 'secondary'
  size?: 'sm' | 'md'
  disabled?: boolean
  loading?: boolean
  onClick?: (e: MouseEvent) => void
  type?: 'button' | 'submit'
  title?: string
  children?: JSX.Element
}> = props => {
  const base =
    'inline-flex items-center justify-center gap-1.5 rounded-control font-medium transition-colors select-none whitespace-nowrap shrink-0 active:scale-[0.98] disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-2 focus-visible:outline-ring'
  const sizes = { sm: 'h-7 px-3 text-xs min-w-fit', md: 'h-9 px-4 text-sm min-w-fit' }
  const variants = {
    primary: 'bg-accent text-on-accent hover:brightness-110 shadow-accent',
    secondary: 'border border-subtle text-muted hover:text-text hover:border-accent',
    ghost: 'text-muted hover:text-text hover:bg-hover',
    danger: 'border border-danger/30 text-danger hover:bg-danger/10',
  }
  return (
    <button
      type={props.type ?? 'button'}
      title={props.title}
      class={`${base} ${sizes[props.size ?? 'md']} ${variants[props.variant ?? 'secondary']}`}
      disabled={props.disabled || props.loading}
      aria-busy={props.loading || undefined}
      onClick={props.onClick}
    >
      <Show when={props.loading}>
        <Spinner />
      </Show>
      {props.children}
    </button>
  )
}

export const Input: Component<{
  value?: string
  placeholder?: string
  type?: string
  disabled?: boolean
  onInput?: (v: string) => void
  onKeyDown?: (e: KeyboardEvent) => void
  class?: string
  ariaLabel?: string
}> = props => (
  <input
    type={props.type ?? 'text'}
    value={props.value ?? ''}
    placeholder={props.placeholder}
    disabled={props.disabled}
    aria-label={props.ariaLabel}
    onInput={e => props.onInput?.(e.currentTarget.value)}
    onKeyDown={props.onKeyDown}
    class={`w-full px-3 py-1.5 rounded-control bg-bg-elevated border border-subtle text-sm text-text placeholder:text-faint focus:outline-none focus:border-accent focus:ring-2 focus:ring-ring-soft transition-colors disabled:opacity-50 ${props.class ?? ''}`}
  />
)

export const Select: Component<{
  value?: string
  options: { value: string; label: string }[]
  onChange?: (v: string) => void
  class?: string
  ariaLabel?: string
}> = props => (
  <select
    value={props.value ?? ''}
    aria-label={props.ariaLabel}
    onChange={e => props.onChange?.(e.currentTarget.value)}
    class={`px-3 py-1.5 rounded-control bg-bg-elevated border border-subtle text-sm text-text focus:outline-none focus:border-accent focus:ring-2 focus:ring-ring-soft ${props.class ?? ''}`}
  >
    <For each={props.options}>{o => <option value={o.value}>{o.label}</option>}</For>
  </select>
)

export const Toggle: Component<{ checked?: boolean; disabled?: boolean; onChange?: (v: boolean) => void }> = props => (
  <button
    type="button"
    role="switch"
    aria-checked={props.checked ?? false}
    disabled={props.disabled}
    onClick={() => props.onChange?.(!props.checked)}
    class={`relative inline-flex items-center w-9 h-5 shrink-0 p-0.5 rounded-full border transition-colors disabled:opacity-50 disabled:cursor-not-allowed ${
      props.checked
        ? 'bg-accent border-accent'
        : 'bg-hover border-subtle'
    }`}
  >
    <span
      class={`pointer-events-none block w-3.5 h-3.5 rounded-full bg-white shadow-sm transition-transform duration-200 ${
        props.checked ? 'translate-x-[16px]' : 'translate-x-0'
      }`}
    />
  </button>
)

export const Modal: Component<{ open: boolean; title: string; onClose: () => void; children?: JSX.Element }> = props => {
  const [panel, setPanel] = createSignal<HTMLDivElement>()

  // 打开时锁定页面滚动 + Esc 关闭（参考 9router Modal）
  onMount(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') props.onClose()
    }
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    onCleanup(() => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = ''
    })
    // 初始焦点移入弹窗，保证键盘可操作
    queueMicrotask(() => {
      const first = panel()?.querySelector<HTMLElement>('input, select, textarea, button')
      first?.focus()
    })
  })

  return (
    <Show when={props.open}>
      <div class="fixed inset-0 z-[70] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/60 backdrop-blur-sm animate-fade-in" onClick={props.onClose} aria-hidden="true" />
        <div
          ref={setPanel}
          role="dialog"
          aria-modal="true"
          aria-label={props.title}
          class="relative w-full max-w-lg rounded-card border border-subtle bg-bg-elevated shadow-glass animate-scale-in"
        >
          <div class="flex items-center justify-between px-5 py-3.5 border-b border-subtle">
            <h3 class="text-sm font-semibold">{props.title}</h3>
            <button
              type="button"
              class="flex h-7 w-7 items-center justify-center rounded-control text-faint hover:text-text hover:bg-hover transition-colors"
              onClick={props.onClose}
              aria-label="关闭"
            >
              ×
            </button>
          </div>
          <div class="p-5 max-h-[calc(85vh-100px)] overflow-y-auto">{props.children}</div>
        </div>
      </div>
    </Show>
  )
}

export const Skeleton: Component<{ class?: string }> = props => (
  <div class={`animate-pulse rounded-control bg-hover ${props.class ?? 'h-4 w-full'}`} aria-hidden="true" />
)

export const Field: Component<{ label: string; hint?: string; children?: JSX.Element }> = props => (
  <label class="block space-y-2">
    <span class="block text-xs font-medium text-muted/90 select-none">{props.label}</span>
    <div class="mt-1">{props.children}</div>
    <Show when={props.hint}>
      <span class="block text-[11px] text-faint mt-1.5 leading-relaxed">{props.hint}</span>
    </Show>
  </label>
)

export { Show }
