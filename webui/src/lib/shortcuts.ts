import { onMounted, onUnmounted } from 'vue'

export function useKeyboardShortcuts(handlers: {
  onSearch?: () => void
  onEscape?: () => void
}) {
  function handleKeydown(e: KeyboardEvent) {
    // Cmd/Ctrl+K → focus search
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault()
      handlers.onSearch?.()
      return
    }
    // Escape → close modals
    if (e.key === 'Escape') {
      handlers.onEscape?.()
    }
  }

  onMounted(() => document.addEventListener('keydown', handleKeydown))
  onUnmounted(() => document.removeEventListener('keydown', handleKeydown))
}
