import { createSignal } from 'solid-js'

export type ToastKind = 'success' | 'error' | 'warning' | 'info'

export interface ToastOptions {
  title?: string
  duration?: number
}

export interface Toast {
  id: number
  kind: ToastKind
  message: string
  title?: string
  duration: number
  createdAt: number
}

const [toasts, setToasts] = createSignal<Toast[]>([])
const timers = new Map<number, ReturnType<typeof setTimeout>>()
let nextId = 0

const DEFAULT_DURATIONS: Record<ToastKind, number> = {
  success: 3500,
  info: 3500,
  warning: 4500,
  error: 5500,
}

export function dismiss(id: number) {
  const timer = timers.get(id)
  if (timer) {
    clearTimeout(timer)
    timers.delete(id)
  }
  setToasts(t => t.filter(x => x.id !== id))
}

export function clearToasts() {
  timers.forEach(clearTimeout)
  timers.clear()
  setToasts([])
}

function push(kind: ToastKind, message: string, opts?: ToastOptions | string): number {
  const current = toasts()
  // 1. Exact duplicate toast already visible: ignore to prevent stacking
  const duplicate = current.find(t => t.kind === kind && t.message === message)
  if (duplicate) {
    return duplicate.id
  }

  // 2. Subsumption: if new toast message embeds an existing toast's message (e.g. "发起授权失败: " + msg),
  // dismiss the older less-specific toast so only the rich message remains.
  const subsumed = current.find(t => t.kind === kind && message.includes(t.message))
  if (subsumed) {
    dismiss(subsumed.id)
  } else {
    // If new toast is less specific and already covered by an existing message, ignore it
    const superseding = current.find(t => t.kind === kind && t.message.includes(message))
    if (superseding) {
      return superseding.id
    }
  }

  const id = ++nextId
  const options: ToastOptions = typeof opts === 'string' ? { title: opts } : opts || {}
  const duration = options.duration !== undefined ? options.duration : DEFAULT_DURATIONS[kind]

  const item: Toast = {
    id,
    kind,
    message,
    title: options.title,
    duration,
    createdAt: Date.now(),
  }

  setToasts(t => [...t, item])
  if (duration > 0) {
    const timer = setTimeout(() => {
      dismiss(id)
    }, duration)
    timers.set(id, timer)
  }

  return id
}

export const toast = {
  toasts,
  success: (m: string, opts?: ToastOptions | string) => push('success', m, opts),
  error: (m: string, opts?: ToastOptions | string) => push('error', m, opts),
  warning: (m: string, opts?: ToastOptions | string) => push('warning', m, opts),
  info: (m: string, opts?: ToastOptions | string) => push('info', m, opts),
  dismiss,
  clear: clearToasts,
}

export function useToast() {
  return toast
}
