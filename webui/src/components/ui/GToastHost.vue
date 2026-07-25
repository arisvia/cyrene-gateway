<script setup lang="ts">
import { useToast } from '@/lib/toast'
import { CheckCircle2, XCircle, Info, X } from 'lucide-vue-next'

const { toasts, dismiss } = useToast()

const icons = { success: CheckCircle2, error: XCircle, info: Info }
</script>

<template>
  <div class="toast-host">
    <TransitionGroup name="toast">
      <div v-for="t in toasts" :key="t.id" :class="['toast', `toast-${t.type}`]">
        <component :is="icons[t.type]" :size="15" class="toast-icon" />
        <span class="toast-msg">{{ t.message }}</span>
        <button class="toast-close" @click="dismiss(t.id)"><X :size="12" /></button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.toast-host {
  position: fixed; top: 16px; right: 16px; z-index: 200;
  display: flex; flex-direction: column; gap: 8px;
  max-width: 340px;
}
.toast {
  display: flex; align-items: center; gap: 10px;
  padding: 11px 14px; border-radius: var(--radius);
  background: var(--dialog-bg);
  backdrop-filter: blur(var(--glass-blur-heavy)); -webkit-backdrop-filter: blur(var(--glass-blur-heavy));
  border: 1px solid var(--glass-border-hover);
  box-shadow: var(--glass-depth);
  font-size: 12.5px; font-weight: 500;
  position: relative; overflow: hidden;
}
/* Colored left accent bar */
.toast::before {
  content: ''; position: absolute; left: 0; top: 0; bottom: 0; width: 3px;
}
.toast-success::before { background: var(--green); }
.toast-error::before { background: var(--red); }
.toast-info::before { background: var(--accent-2); }

.toast-success .toast-icon { color: var(--green); flex-shrink: 0; }
.toast-error .toast-icon { color: var(--red); flex-shrink: 0; }
.toast-info .toast-icon { color: var(--accent-2); flex-shrink: 0; }

.toast-msg { flex: 1; min-width: 0; word-break: break-word; }
.toast-close {
  background: none; border: none; color: var(--text-faint);
  cursor: pointer; padding: 2px; flex-shrink: 0;
  display: flex; border-radius: 4px; transition: color 0.15s ease;
}
.toast-close:hover { color: var(--text); }

/* Transitions */
.toast-enter-active { animation: toastIn 0.3s var(--ease-spring); }
.toast-leave-active { animation: toastOut 0.2s ease; }
.toast-move { transition: transform 0.25s var(--ease-out-expo); }
@keyframes toastIn {
  from { opacity: 0; transform: translateX(40px) scale(0.95); }
  to { opacity: 1; transform: translateX(0) scale(1); }
}
@keyframes toastOut {
  from { opacity: 1; transform: translateX(0); }
  to { opacity: 0; transform: translateX(40px); }
}
</style>
