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
    ref={el => {
      if (typeof props.ref === 'function') props.ref(el)
      else if (props.ref) (props as unknown as { ref: HTMLDivElement }).ref = el
    }}
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

export const Spinner: Component<{ size?: 'sm' | 'md' | 'lg' }> = props => {
  const sizes = { sm: 'h-3.5 w-3.5', md: 'h-4 w-4', lg: 'h-5 w-5' }
  return (
    <div
      class={`${sizes[props.size ?? 'md']} shrink-0 rounded-full border-2 border-subtle border-t-accent animate-spin`}
      aria-hidden="true"
    />
  )
}

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
  size?: 'sm' | 'md' | 'lg'
  disabled?: boolean
  loading?: boolean
  onClick?: (e: MouseEvent) => void
  type?: 'button' | 'submit'
  title?: string
  children?: JSX.Element
  class?: string
}> = props => {
  const base =
    'inline-flex items-center justify-center font-medium transition-all duration-150 select-none whitespace-nowrap shrink-0 active:scale-[0.98] disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-2 focus-visible:outline-ring cursor-pointer rounded-control'
  const sizes = {
    sm: 'h-8 px-3 text-xs min-w-fit gap-1.5',
    md: 'h-9 px-4 text-sm min-w-fit gap-2',
    lg: 'h-11 px-5 text-base min-w-fit gap-2.5',
  }
  const variants = {
    primary: 'bg-accent text-on-accent hover:brightness-110 shadow-accent',
    secondary: 'border border-subtle text-muted hover:text-text hover:border-accent hover:bg-hover/50',
    ghost: 'text-muted hover:text-text hover:bg-hover',
    danger: 'border border-danger/30 text-danger hover:bg-danger/10',
  }
  return (
    <button
      type={props.type ?? 'button'}
      title={props.title}
      class={`${base} ${sizes[props.size ?? 'md']} ${variants[props.variant ?? 'secondary']} ${props.class ?? ''}`}
      disabled={props.disabled || props.loading}
      aria-busy={props.loading || undefined}
      onClick={props.onClick}
    >
      <Show when={props.loading}>
        <Spinner size={props.size} />
      </Show>
      {props.children}
    </button>
  )
}

export const Input: Component<{
  value?: string
  placeholder?: string
  type?: string
  size?: 'sm' | 'md' | 'lg'
  disabled?: boolean
  onInput?: (v: string) => void
  onKeyDown?: (e: KeyboardEvent) => void
  class?: string
  ariaLabel?: string
}> = props => {
  const sizes = {
    sm: 'h-8 px-2.5 text-xs',
    md: 'h-9 px-3 text-sm',
    lg: 'h-11 px-4 text-base',
  }
  return (
    <input
      type={props.type ?? 'text'}
      value={props.value ?? ''}
      placeholder={props.placeholder}
      disabled={props.disabled}
      aria-label={props.ariaLabel}
      onInput={e => props.onInput?.(e.currentTarget.value)}
      onKeyDown={props.onKeyDown}
      class={`w-full ${sizes[props.size ?? 'md']} rounded-control bg-bg-elevated border border-subtle text-text placeholder:text-faint focus:outline-none focus:border-accent focus:ring-2 focus:ring-ring-soft transition-all duration-150 disabled:opacity-50 disabled:cursor-not-allowed ${props.class ?? ''}`}
    />
  )
}

export const Select: Component<{
  value?: string
  options: { value: string; label: string }[]
  onChange?: (v: string) => void
  size?: 'sm' | 'md' | 'lg'
  class?: string
  ariaLabel?: string
  placeholder?: string
  disabled?: boolean
  align?: 'left' | 'right'
}> = props => {
  const [open, setOpen] = createSignal(false)
  let rootRef: HTMLDivElement | undefined

  onMount(() => {
    const handleOutsideClick = (e: MouseEvent | PointerEvent) => {
      if (rootRef && !rootRef.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!open()) {
        if ((e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter') && rootRef?.contains(document.activeElement)) {
          e.preventDefault()
          setOpen(true)
        }
        return
      }

      if (e.key === 'Escape') {
        e.preventDefault()
        setOpen(false)
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        const idx = props.options.findIndex(o => o.value === (props.value ?? ''))
        const next = idx < props.options.length - 1 ? idx + 1 : 0
        if (props.options[next]) props.onChange?.(props.options[next].value)
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        const idx = props.options.findIndex(o => o.value === (props.value ?? ''))
        const prev = idx > 0 ? idx - 1 : props.options.length - 1
        if (props.options[prev]) props.onChange?.(props.options[prev].value)
      } else if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        setOpen(false)
      }
    }

    window.addEventListener('pointerdown', handleOutsideClick)
    window.addEventListener('keydown', handleKeyDown)
    onCleanup(() => {
      window.removeEventListener('pointerdown', handleOutsideClick)
      window.removeEventListener('keydown', handleKeyDown)
    })
  })

  const selectedOption = () => props.options.find(o => o.value === (props.value ?? ''))
  const isSelected = () => !!selectedOption()
  const displayLabel = () => selectedOption()?.label || props.placeholder || (props.options[0]?.label ?? '')

  const triggerSizes = {
    sm: 'h-8 pl-3 pr-2.5 text-xs gap-2',
    md: 'h-9 pl-3.5 pr-3 text-sm gap-2.5',
    lg: 'h-11 pl-4 pr-3.5 text-base gap-3',
  }

  const chevronSizes = {
    sm: 'w-3 h-3',
    md: 'w-3.5 h-3.5',
    lg: 'w-4 h-4',
  }

  const optionSizes = {
    sm: 'px-2.5 py-1.5 text-xs',
    md: 'px-3 py-2 text-sm',
    lg: 'px-3.5 py-2.5 text-base',
  }

  return (
    <div
      ref={rootRef}
      class={`relative ${props.class?.includes('flex-1') ? 'flex-1' : props.class?.includes('w-full') ? 'w-full' : 'inline-block'} ${props.class ?? ''}`}
    >
      <button
        type="button"
        role="combobox"
        aria-expanded={open()}
        aria-haspopup="listbox"
        aria-label={props.ariaLabel || displayLabel()}
        disabled={props.disabled}
        onClick={() => {
          if (!props.disabled) setOpen(o => !o)
        }}
        class={`w-full flex items-center justify-between ${triggerSizes[props.size ?? 'md']} rounded-control bg-bg-elevated border border-subtle text-text hover:border-accent/40 hover:bg-hover/50 focus:outline-none focus:border-accent focus:ring-2 focus:ring-ring-soft transition-all duration-150 ${
          open() ? 'border-accent ring-2 ring-ring-soft' : ''
        } ${props.disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer'}`}
      >
        <span class={`truncate text-left flex-1 ${isSelected() ? 'text-text' : 'text-faint'}`}>
          {displayLabel()}
        </span>
        {/* 精致内嵌居中的 Chevron 矢量箭头：预留呼吸空间，完全消除原生靠死右侧的视觉不协调感 */}
        <div class="flex items-center justify-center shrink-0 text-faint ml-1">
          <svg
            class={`${chevronSizes[props.size ?? 'md']} transition-transform duration-200 ${
              open() ? 'rotate-180 text-accent' : ''
            }`}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </div>
      </button>

      {/* 现代液态玻璃拟态下拉浮层（彻底淘汰浏览器原生黑灰选项框） */}
      <Show when={open()}>
        <div
          role="listbox"
          class={`absolute ${
            props.align === 'right' ? 'right-0' : 'left-0'
          } top-[calc(100%+6px)] z-50 min-w-full w-max max-w-[min(92vw,400px)] max-h-64 overflow-y-auto rounded-control glass-panel border border-subtle bg-bg-elevated/95 backdrop-blur-xl shadow-glass-hover p-1.5 space-y-0.5 animate-scale-in`}
        >
          <For each={props.options}>
            {o => {
              const active = () => (props.value ?? '') === o.value
              return (
                <button
                  type="button"
                  role="option"
                  aria-selected={active()}
                  class={`w-full flex items-center justify-between gap-3 ${optionSizes[props.size ?? 'md']} rounded-lg text-left transition-all duration-150 cursor-pointer ${
                    active()
                      ? 'bg-accent/15 text-accent font-medium'
                      : 'text-text hover:bg-hover hover:text-text'
                  }`}
                  onClick={() => {
                    props.onChange?.(o.value)
                    setOpen(false)
                  }}
                >
                  <span class="truncate">{o.label}</span>
                  <Show when={active()}>
                    <svg
                      class="w-4 h-4 text-accent shrink-0 animate-scale-in"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      stroke-width="2.5"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      aria-hidden="true"
                    >
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
                  </Show>
                </button>
              )
            }}
          </For>
        </div>
      </Show>
    </div>
  )
}

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
