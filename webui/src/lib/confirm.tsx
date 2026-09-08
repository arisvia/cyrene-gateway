import { createSignal, Show, type Component, createEffect, onMount, onCleanup } from 'solid-js'
import { Portal } from 'solid-js/web'
import { Button } from '@/components/ui'
import { IconAlertCircle, IconAlertTriangle, IconInfo } from '@/components/ui/icons'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  variant?: 'danger' | 'warning' | 'primary'
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
  let prevFocus: HTMLElement | null = null
  let confirmBtnRef: HTMLButtonElement | undefined

  const handleClose = (res: boolean) => {
    const fn = state().resolve
    setState(prev => ({ ...prev, isOpen: false }))
    fn(res)
    if (prevFocus && typeof prevFocus.focus === 'function') {
      prevFocus.focus()
      prevFocus = null
    }
  }

  createEffect(() => {
    if (s().isOpen) {
      prevFocus = document.activeElement as HTMLElement | null
      queueMicrotask(() => {
        confirmBtnRef?.focus()
      })
    }
  })

  onMount(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (!state().isOpen) return
      if (e.key === 'Escape') {
        e.preventDefault()
        handleClose(false)
      }
    }
    window.addEventListener('keydown', handleKey)
    onCleanup(() => window.removeEventListener('keydown', handleKey))
  })

  return (
    <Show when={s().isOpen}>
      <Portal>
        <div class="fixed inset-0 z-120 flex items-center justify-center p-4">
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
              class={`w-10 h-10 rounded-xl flex items-center justify-center shrink-0 border ${
                s().variant === 'danger'
                  ? 'bg-danger/15 text-danger border-danger/30'
                  : s().variant === 'warning'
                    ? 'bg-warning/15 text-warning border-warning/30'
                    : 'bg-accent/15 text-accent border-accent/30'
              }`}
            >
              <Show when={s().variant === 'danger'}>
                <IconAlertCircle size="md" class="w-5 h-5" />
              </Show>
              <Show when={s().variant === 'warning'}>
                <IconAlertTriangle size="md" class="w-5 h-5" />
              </Show>
              <Show when={s().variant !== 'danger' && s().variant !== 'warning'}>
                <IconInfo size="md" class="w-5 h-5" />
              </Show>
            </div>

            <div class="flex-1 min-w-0">
              <h3 class="text-base font-semibold text-foreground leading-snug">
                {s().title || (s().variant === 'danger' ? '确认操作' : s().variant === 'warning' ? '重要提醒' : '提示')}
              </h3>
              <p class="text-sm text-muted mt-1.5 leading-relaxed whitespace-pre-wrap wrap-break-word">
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
              ref={confirmBtnRef}
              variant={s().variant === 'danger' ? 'danger' : 'primary'}
              size="sm"
              onClick={() => handleClose(true)}
            >
              {s().confirmText || (s().variant === 'danger' ? '确认删除' : '确认')}
            </Button>
          </div>
        </div>
      </div>
      </Portal>
    </Show>
  )
}
