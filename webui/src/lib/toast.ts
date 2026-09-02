import { createSignal } from 'solid-js'

export interface Toast {
  id: number
  kind: 'success' | 'error' | 'info'
  message: string
}

const [toasts, setToasts] = createSignal<Toast[]>([])
let nextId = 0

function push(kind: Toast['kind'], message: string) {
  const id = nextId++
  setToasts(t => [...t, { id, kind, message }])
  setTimeout(() => {
    setToasts(t => t.filter(x => x.id !== id))
  }, 3500)
}

export function useToast() {
  return {
    toasts,
    success: (m: string) => push('success', m),
    error: (m: string) => push('error', m),
    info: (m: string) => push('info', m),
  }
}
