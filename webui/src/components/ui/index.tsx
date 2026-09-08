import { type Component, type JSX, For, Show, createSignal, createMemo, createEffect, onMount, onCleanup, splitProps } from 'solid-js'
import { Portal } from 'solid-js/web'
import { useToast, dismiss } from '@/lib/toast'
import { IconClose, IconCheck, IconAlertCircle, IconAlertTriangle, IconInfo } from './icons'
export { ProviderAvatar, ProviderBrandIcon } from './ProviderIcon'
export * from './icons'
export { ConfirmDialogHost, confirm, alert } from '@/lib/confirm'
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
export interface PageHeaderProps {
  title: string | JSX.Element
  subtitle?: string | JSX.Element
  badge?: JSX.Element
  actions?: JSX.Element
  children?: JSX.Element
  class?: string
}

export const PageHeader: Component<PageHeaderProps> = props => (
  <header class={`sticky top-[4.75rem] z-20 rounded-2xl glass-card px-5 py-4 shadow-glass transition-all space-y-3 ${props.class ?? ''}`}>
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
      <div class="min-w-0">
        <div class="flex items-center gap-2.5 flex-wrap">
          <h1 class="text-lg sm:text-xl font-semibold text-foreground truncate">{props.title}</h1>
          {props.badge}
        </div>
        <Show when={props.subtitle}>
          <p class="text-xs sm:text-sm text-faint mt-0.5 truncate">{props.subtitle}</p>
        </Show>
      </div>
      <Show when={props.actions}>
        <div class="flex items-center gap-2.5 flex-wrap shrink-0">
          {props.actions}
        </div>
      </Show>
    </div>
    <Show when={props.children}>
      <div class="pt-0.5">
        {props.children}
      </div>
    </Show>
  </header>
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
export const StatusPulse: Component<{
  status?: 'active' | 'paused' | 'error' | 'idle'
  tone?: 'green' | 'amber' | 'red' | 'blue' | 'accent' | 'gray'
  size?: 'xs' | 'sm' | 'md'
  class?: string
}> = props => {
  const isPulsing = () => props.status === 'active' || (!props.status && props.tone !== 'gray')
  const tone = () => {
    if (props.tone) return props.tone
    if (props.status === 'active') return 'accent'
    if (props.status === 'error') return 'red'
    if (props.status === 'paused') return 'gray'
    return 'green'
  }

  const dotColors: Record<string, string> = {
    accent: 'bg-accent',
    green: 'bg-success',
    amber: 'bg-warning',
    red: 'bg-danger',
    blue: 'bg-info',
    gray: 'bg-zinc-500',
  }

  const pingColors: Record<string, string> = {
    accent: 'bg-accent/75',
    green: 'bg-success/75',
    amber: 'bg-warning/75',
    red: 'bg-danger/75',
    blue: 'bg-info/75',
    gray: 'bg-zinc-500/50',
  }

  const sizeClasses: Record<string, string> = {
    xs: 'w-1.5 h-1.5',
    sm: 'w-2 h-2',
    md: 'w-2.5 h-2.5',
  }

  const sz = () => sizeClasses[props.size ?? 'sm']

  return (
    <span class={`relative inline-flex shrink-0 items-center justify-center self-center ${sz()} ${props.class ?? ''}`}>
      <Show when={isPulsing()}>
        <span class={`absolute inline-flex h-full w-full rounded-full animate-ping opacity-75 ${pingColors[tone()]}`} />
      </Show>
      <span class={`relative inline-flex rounded-full ${sz()} ${dotColors[tone()]}`} />
    </span>
  )
}

export function ToastHost() {
  const { toasts } = useToast()

  const kindStyles = {
    success: {
      card: 'border-success/30 bg-bg-elevated/95',
      iconWrap: 'bg-success/15 text-success border-success/30',
    },
    error: {
      card: 'border-danger/30 bg-bg-elevated/95',
      iconWrap: 'bg-danger/15 text-danger border-danger/30',
    },
    warning: {
      card: 'border-warning/30 bg-bg-elevated/95',
      iconWrap: 'bg-warning/15 text-warning border-warning/30',
    },
    info: {
      card: 'border-subtle bg-bg-elevated/95',
      iconWrap: 'bg-accent/15 text-accent border-accent/30',
    },
  }

  return (
    <div class="fixed top-4 right-4 z-[130] w-full max-w-sm pointer-events-none flex flex-col gap-2.5 px-3 sm:px-0" role="status" aria-live="polite">
      <For each={toasts()}>
        {t => {
          const style = () => kindStyles[t.kind] || kindStyles.info
          return (
            <div
              class={`pointer-events-auto relative w-full p-3 rounded-xl border shadow-glass-hover backdrop-blur-2xl transition-all duration-200 animate-slide-up flex items-start gap-3 select-none ${style().card}`}
            >
              <div class={`w-7 h-7 rounded-lg flex items-center justify-center shrink-0 border mt-0.5 ${style().iconWrap}`}>
                <Show when={t.kind === 'success'}>
                  <IconCheck size="sm" class="w-3.5 h-3.5" />
                </Show>
                <Show when={t.kind === 'error'}>
                  <IconAlertCircle size="sm" class="w-3.5 h-3.5" />
                </Show>
                <Show when={t.kind === 'warning'}>
                  <IconAlertTriangle size="sm" class="w-3.5 h-3.5" />
                </Show>
                <Show when={t.kind === 'info'}>
                  <IconInfo size="sm" class="w-3.5 h-3.5" />
                </Show>
              </div>

              <div class="flex-1 min-w-0 pt-0.5">
                <Show when={t.title}>
                  <div class="text-xs font-semibold text-foreground mb-0.5 leading-snug">
                    {t.title}
                  </div>
                </Show>
                <div class="text-xs text-muted leading-relaxed break-words whitespace-pre-wrap">
                  {t.message}
                </div>
              </div>

              <button
                type="button"
                aria-label="关闭提示"
                onClick={() => dismiss(t.id)}
                class="text-faint hover:text-foreground p-1 rounded-md hover:bg-hover transition-colors shrink-0 -mr-1 -mt-1 cursor-pointer"
              >
                <IconClose size="xs" class="w-3.5 h-3.5" />
              </button>
            </div>
          )
        }}
      </For>
    </div>
  )
}

export const Alert: Component<{
  variant?: 'info' | 'success' | 'warning' | 'danger'
  title?: string
  children?: JSX.Element
  closable?: boolean
  onClose?: () => void
  class?: string
}> = props => {
  const [closed, setClosed] = createSignal(false)
  const variant = () => props.variant ?? 'info'

  const styles = {
    info: {
      box: 'border-subtle bg-bg-elevated/80 text-foreground',
      iconWrap: 'text-accent',
    },
    success: {
      box: 'border-success/30 bg-success/10 text-foreground',
      iconWrap: 'text-success',
    },
    warning: {
      box: 'border-warning/30 bg-warning/10 text-foreground',
      iconWrap: 'text-warning',
    },
    danger: {
      box: 'border-danger/30 bg-danger/10 text-foreground',
      iconWrap: 'text-danger',
    },
  }

  const s = () => styles[variant()] || styles.info

  return (
    <Show when={!closed()}>
      <div
        role="alert"
        class={`relative flex items-start gap-3 p-3.5 rounded-xl border backdrop-blur-md text-xs transition-all duration-150 ${s().box} ${props.class ?? ''}`}
      >
        <div class={`shrink-0 mt-0.5 ${s().iconWrap}`}>
          <Show when={variant() === 'success'}>
            <IconCheck size="sm" class="w-4 h-4" />
          </Show>
          <Show when={variant() === 'danger'}>
            <IconAlertCircle size="sm" class="w-4 h-4" />
          </Show>
          <Show when={variant() === 'warning'}>
            <IconAlertTriangle size="sm" class="w-4 h-4" />
          </Show>
          <Show when={variant() === 'info'}>
            <IconInfo size="sm" class="w-4 h-4" />
          </Show>
        </div>

        <div class="flex-1 min-w-0">
          <Show when={props.title}>
            <div class="font-semibold text-foreground mb-0.5 leading-snug">
              {props.title}
            </div>
          </Show>
          <div class="text-muted leading-relaxed break-words">
            {props.children}
          </div>
        </div>

        <Show when={props.closable}>
          <button
            type="button"
            aria-label="关闭提示"
            onClick={() => {
              setClosed(true)
              props.onClose?.()
            }}
            class="text-faint hover:text-foreground p-1 rounded-md hover:bg-hover transition-colors shrink-0 -mr-1 -mt-1 cursor-pointer"
          >
            <IconClose size="xs" class="w-3.5 h-3.5" />
          </button>
        </Show>
      </div>
    </Show>
  )
}

export const controlSizes = {
  sm: 'h-8 text-xs',
  md: 'h-9 text-sm',
  lg: 'h-11 text-base',
} as const

export type ControlSize = keyof typeof controlSizes

export interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'ghost' | 'danger' | 'secondary'
  size?: 'sm' | 'md' | 'lg'
  disabled?: boolean
  loading?: boolean
  onClick?: (e: MouseEvent) => void
  type?: 'button' | 'submit' | 'reset'
  title?: string
  children?: JSX.Element
  class?: string
  ref?: HTMLButtonElement | ((el: HTMLButtonElement) => void)
}

export const Button: Component<ButtonProps> = props => {
  const [local, others] = splitProps(props, ['variant', 'size', 'disabled', 'loading', 'children', 'class', 'ref', 'type'])
  const base =
    'inline-flex items-center justify-center font-medium transition-all duration-150 select-none whitespace-nowrap shrink-0 active:scale-[0.98] disabled:opacity-50 disabled:pointer-events-none focus-visible:outline-2 focus-visible:outline-ring cursor-pointer rounded-control'
  const sizes: Record<ControlSize, string> = {
    sm: `${controlSizes.sm} px-3 min-w-fit gap-1.5`,
    md: `${controlSizes.md} px-4 min-w-fit gap-2`,
    lg: `${controlSizes.lg} px-5 min-w-fit gap-2.5`,
  }
  const variants = {
    primary: 'bg-accent text-on-accent hover:brightness-110 shadow-accent',
    secondary: 'border border-subtle text-muted hover:text-text hover:border-accent hover:bg-hover/50',
    ghost: 'text-muted hover:text-text hover:bg-hover',
    danger: 'border border-danger/30 text-danger hover:bg-danger/10',
  }
  return (
    <button
      ref={local.ref}
      type={local.type ?? 'button'}
      class={`${base} ${sizes[local.size ?? 'md']} ${variants[local.variant ?? 'secondary']} ${local.class ?? ''}`}
      disabled={local.disabled || local.loading}
      aria-busy={local.loading || undefined}
      {...others}
    >
      <Show when={local.loading}>
        <Spinner size={local.size} />
      </Show>
      {local.children}
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
  const sizes: Record<ControlSize, string> = {
    sm: `${controlSizes.sm} px-2.5`,
    md: `${controlSizes.md} px-3`,
    lg: `${controlSizes.lg} px-4`,
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
  const [search, setSearch] = createSignal('')
  const [popoverStyle, setPopoverStyle] = createSignal<Record<string, string>>({})
  let rootRef: HTMLDivElement | undefined
  let popoverRef: HTMLDivElement | undefined
  let searchInputRef: HTMLInputElement | undefined

  function updatePosition() {
    if (!rootRef) return
    const rect = rootRef.getBoundingClientRect()
    const spaceBelow = window.innerHeight - rect.bottom
    const placeUp = spaceBelow < 280 && rect.top > spaceBelow

    const style: Record<string, string> = {
      position: 'fixed',
      'z-index': '110',
      'min-width': `${rect.width}px`,
      'max-width': 'min(92vw, 480px)',
    }

    if (props.align === 'right') {
      style.right = `${window.innerWidth - rect.right}px`
    } else {
      style.left = `${Math.max(8, rect.left)}px`
    }

    if (placeUp) {
      style.bottom = `${window.innerHeight - rect.top + 6}px`
    } else {
      style.top = `${rect.bottom + 6}px`
    }

    setPopoverStyle(style)
  }

  function toggleOpen() {
    if (props.disabled) return
    if (!open()) {
      updatePosition()
    }
    setOpen(o => !o)
  }
  onMount(() => {
    const handleOutsideClick = (e: MouseEvent | PointerEvent) => {
      const target = e.target as Node
      if (rootRef?.contains(target) || popoverRef?.contains(target)) {
        return
      }
      setOpen(false)
    }

    const handleScrollOrResize = () => {
      if (open()) {
        updatePosition()
      }
    }

    const handleKeyDown = (e: KeyboardEvent) => {
      if (!open()) {
        if ((e.key === 'ArrowDown' || e.key === 'ArrowUp' || e.key === 'Enter') && rootRef?.contains(document.activeElement)) {
          e.preventDefault()
          toggleOpen()
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
    window.addEventListener('scroll', handleScrollOrResize, true)
    window.addEventListener('resize', handleScrollOrResize)
    onCleanup(() => {
      window.removeEventListener('pointerdown', handleOutsideClick)
      window.removeEventListener('keydown', handleKeyDown)
      window.removeEventListener('scroll', handleScrollOrResize, true)
      window.removeEventListener('resize', handleScrollOrResize)
    })
  })

  const selectedOption = () => props.options.find(o => o.value === (props.value ?? ''))
  const isSelected = () => !!selectedOption()
  const displayLabel = () => selectedOption()?.label || props.placeholder || (props.options[0]?.label ?? '')
  const filteredOptions = createMemo(() => {
    const q = search().trim().toLowerCase()
    if (!q) return props.options
    return props.options.filter(o => o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q))
  })

  const triggerSizes: Record<ControlSize, string> = {
    sm: `${controlSizes.sm} pl-3 pr-2.5 gap-2`,
    md: `${controlSizes.md} pl-3.5 pr-3 gap-2.5`,
    lg: `${controlSizes.lg} pl-4 pr-3.5 gap-3`,
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
        onClick={toggleOpen}
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
      {/* 现代液态玻璃拟态下拉浮层（带搜索过滤与超长自适应） */}
      <Show when={open()}>
        <Portal>
          <div
            ref={popoverRef}
            role="listbox"
            style={popoverStyle()}
            class="rounded-control glass-panel border border-subtle bg-bg-elevated/95 backdrop-blur-xl shadow-glass-hover p-1.5 flex flex-col gap-1 animate-scale-in"
          >
            <Show when={props.options.length > 8}>
              <div class="px-1 pt-0.5 pb-1 border-b border-subtle/60">
                <input
                  ref={searchInputRef}
                  type="text"
                  value={search()}
                  onInput={e => setSearch(e.currentTarget.value)}
                  placeholder="搜索选项…"
                  class="w-full px-2.5 py-1.5 text-xs rounded-md bg-hover/80 text-foreground placeholder:text-faint border border-subtle/60 outline-hidden focus:border-accent"
                  onClick={e => e.stopPropagation()}
                  onKeyDown={e => {
                    if (e.key === 'Escape') {
                      setOpen(false)
                    }
                  }}
                />
              </div>
            </Show>
            <div class="max-h-60 overflow-y-auto space-y-0.5 pr-0.5">
              <For each={filteredOptions()}>
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
                        setSearch('')
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
              <Show when={filteredOptions().length === 0}>
                <div class="px-3 py-3 text-center text-xs text-faint">未找到匹配项</div>
              </Show>
            </div>
          </div>
        </Portal>
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

  // 打开时锁定页面滚动 + Esc 关闭（参考 9router Modal，避免全局未打开时锁死页面滚动）
  createEffect(() => {
    if (props.open) {
      const onKey = (e: KeyboardEvent) => {
        if (e.key === 'Escape') props.onClose()
      }
      document.addEventListener('keydown', onKey)
      document.body.style.overflow = 'hidden'

      queueMicrotask(() => {
        const first = panel()?.querySelector<HTMLElement>('input, select, textarea, button')
        first?.focus()
      })

      onCleanup(() => {
        document.removeEventListener('keydown', onKey)
        document.body.style.overflow = ''
      })
    }
  })

  return (
    <Show when={props.open}>
      <Portal>
        <div class="fixed inset-0 z-[100] flex items-center justify-center p-4">
          <div class="absolute inset-0 bg-black/60 backdrop-blur-md animate-fade-in" onClick={props.onClose} aria-hidden="true" />
          <div
            ref={setPanel}
            role="dialog"
            aria-modal="true"
            aria-label={props.title}
            class="relative w-full max-w-lg rounded-2xl border border-subtle bg-bg-elevated/95 backdrop-blur-2xl shadow-glass-hover animate-scale-in"
          >
            <div class="flex items-center justify-between px-5 py-3.5 border-b border-subtle bg-card/40 rounded-t-2xl">
              <h3 class="text-sm font-semibold">{props.title}</h3>
              <button
                type="button"
                class="flex h-7 w-7 items-center justify-center rounded-control text-faint hover:text-text hover:bg-hover transition-colors"
                onClick={props.onClose}
                aria-label="关闭"
              >
                <IconClose size={14} />
              </button>
            </div>
            <div class="p-5 max-h-[calc(85vh-100px)] overflow-y-auto">{props.children}</div>
          </div>
        </div>
      </Portal>
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
