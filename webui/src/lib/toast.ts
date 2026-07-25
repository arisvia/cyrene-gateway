import { ref } from 'vue'

export type ToastType = 'success' | 'error' | 'info'

export interface Toast {
  id: number
  type: ToastType
  message: string
}

const toasts = ref<Toast[]>([])
let nextId = 0
const MAX_TOASTS = 4

function push(type: ToastType, message: string, duration = 3000) {
  const id = nextId++
  toasts.value.push({ id, type, message })
  if (toasts.value.length > MAX_TOASTS) {
    toasts.value.shift()
  }
  setTimeout(() => dismiss(id), type === 'error' ? 5000 : duration)
}

function dismiss(id: number) {
  toasts.value = toasts.value.filter(t => t.id !== id)
}

export function useToast() {
  return {
    toasts,
    dismiss,
    success: (msg: string) => push('success', msg),
    error: (msg: string) => push('error', msg, 5000),
    info: (msg: string) => push('info', msg),
  }
}
