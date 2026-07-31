<script setup lang="ts">
import { useToast } from '@/lib/toast'
const { toasts } = useToast()
</script>

<template>
  <Teleport to="body">
    <div class="toast-host" aria-live="polite">
      <TransitionGroup name="toast">
        <div v-for="t in toasts" :key="t.id" class="toast" :class="t.kind">
          <span class="dot" />
          {{ t.message }}
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-host {
  position: fixed; bottom: 20px; right: 20px; z-index: 200;
  display: flex; flex-direction: column; gap: 8px; align-items: flex-end;
}
.toast {
  display: flex; align-items: center; gap: 8px;
  padding: 10px 16px; border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  border: 1px solid var(--glass-border-hover);
  box-shadow: var(--glass-depth);
  font-size: 12.5px; font-weight: 500;
  max-width: 340px;
}
.dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
.toast.success .dot { background: var(--green); box-shadow: 0 0 6px var(--green); }
.toast.error .dot { background: var(--red); box-shadow: 0 0 6px var(--red); }
.toast.info .dot { background: var(--blue); box-shadow: 0 0 6px var(--blue); }

.toast-enter-active { animation: slideUp 0.25s var(--ease-spring); }
.toast-leave-active { animation: fadeIn 0.15s ease reverse; }
</style>
