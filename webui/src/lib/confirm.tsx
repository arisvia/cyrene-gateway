import { createSignal, Show, type Component, onMount, onCleanup } from 'solid-js'
import { Button } from '@/components/ui'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  variant?: 'danger' | 'primary'
}

interface DialogState extends ConfirmOptions {
  isOpen: boolean
  isAlert?: boolean
  resolve: (value: boolean) => void
}

const [state, setState] = createSignal<DialogState>({
  isOpen: false,
  message: '',
  resolve: () => {},
})

export function confirm(opts: ConfirmOptions | string): Promise<boolean> {
  const options = typeof opts === 'string' ? { message: opts } : opts
  return new Promise<boolean>(resolve => {
    setState({
      ...options,
      isOpen: true,
      isAlert: false,
      resolve,
    })
  })
}

export function alert(message: string, title?: string): Promise<boolean> {
  return new Promise<boolean>(resolve => {
    setState({
      title: title || '提示',
      message,
      confirmText: '好的',
      variant: 'primary',
      isOpen: true,
      isAlert: true,
      resolve,
    })
  })
}

export const ConfirmDialogHost: Component = () => {
  const s = () => state()

  const handleClose = (res: boolean) => {
    const fn = state().resolve
    setState(prev => ({ ...prev, isOpen: false }))
    fn(res)
  }

  onMount(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (!state().isOpen) return
      if (e.key === 'Escape') {
        e.preventDefault()
        handleClose(false)
      } else if (e.key === 'Enter') {
        e.preventDefault()
        handleClose(true)
      }
    }
    window.addEventListener('keydown', handleKey)
    onCleanup(() => window.removeEventListener('keydown', handleKey))
  })

  return (
    <Show when={s().isOpen}>
      <div class="fixed inset-0 z-[100] flex items-center justify-center p-4">
        {/* 背景毛玻璃遮罩 */}
        <div
          class="absolute inset-0 bg-black/60 backdrop-blur-md animate-fade-in"
          onClick={() => handleClose(false)}
          aria-hidden="true"
        />

        {/* 现代 Liquid Glass 对话框卡片 */}
        <div
          role="alertdialog"
          aria-modal="true"
          class="relative w-full max-w-md rounded-2xl border border-subtle bg-bg-elevated/95 backdrop-blur-2xl shadow-glass-hover p-6 animate-scale-in select-none"
        >
          <div class="flex items-start gap-4">
            <div
              class={`w-10 h-10 rounded-xl flex items-center justify-center shrink-0 ${
                s().variant === 'danger'
                  ? 'bg-danger/15 text-danger border border-danger/30'
                  : 'bg-accent/15 text-accent border border-accent/30'
              }`}
            >
              <Show
                when={s().variant === 'danger'}
                fallback={
                  <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="12" cy="12" r="10" />
                    <line x1="12" y1="16" x2="12" y2="12" />
                    <line x1="12" y1="8" x2="12.01" y2="8" />
                  </svg>
                }
              >
                <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                  <line x1="12" y1="9" x2="12" y2="13" />
                  <line x1="12" y1="17" x2="12.01" y2="17" />
                </svg>
              </Show>
            </div>

            <div class="flex-1 min-w-0">
              <h3 class="text-base font-semibold text-foreground leading-snug">
                {s().title || (s().variant === 'danger' ? '确认操作' : '提示')}
              </h3>
              <p class="text-sm text-muted mt-1.5 leading-relaxed whitespace-pre-wrap break-words">
                {s().message}
              </p>
            </div>
          </div>

          <div class="mt-6 flex items-center justify-end gap-2.5">
            <Show when={!s().isAlert}>
              <Button
                variant="secondary"
                size="sm"
                onClick={() => handleClose(false)}
              >
                {s().cancelText || '取消'}
              </Button>
            </Show>
            <Button
              variant={s().variant === 'danger' ? 'danger' : 'primary'}
              size="sm"
              onClick={() => handleClose(true)}
            >
              {s().confirmText || (s().variant === 'danger' ? '确认删除' : '确认')}
            </Button>
          </div>
        </div>
      </div>
    </Show>
  )
}
