<script setup lang="ts">
defineProps<{
  variant?: 'primary' | 'ghost' | 'danger-ghost'
  size?: 'sm' | 'icon'
  disabled?: boolean
}>()
</script>

<template>
  <button
    :class="['btn', `btn-${variant || 'primary'}`, size === 'sm' && 'btn-sm', size === 'icon' && 'btn-icon']"
    :disabled="disabled"
  >
    <slot />
  </button>
</template>

<style scoped>
.btn {
  display: inline-flex; align-items: center; justify-content: center; gap: 7px;
  height: 34px; padding: 0 14px; border-radius: var(--radius-sm);
  font-size: 12.5px; font-weight: 550; font-family: var(--font);
  border: 1px solid transparent; cursor: pointer;
  transition: all 0.2s var(--ease-spring); white-space: nowrap;
}
.btn svg { width: 13px; height: 13px; }
.btn:active { transform: scale(0.94); }
.btn:disabled { opacity: 0.5; cursor: not-allowed; }
.btn-primary {
  background: var(--gradient); color: var(--on-accent); border: none;
  box-shadow: var(--shadow-accent);
}
.btn-primary:hover:not(:disabled) { box-shadow: var(--shadow-accent-hover); filter: brightness(1.1); transform: translateY(-1px); }
.btn-primary:active:not(:disabled) { transform: translateY(0) scale(0.94); }
.btn-ghost {
  background: transparent; color: var(--text-muted);
  border-color: var(--glass-border);
}
.btn-ghost:hover:not(:disabled) { color: var(--text); background: var(--glass-hover); border-color: var(--glass-border-hover); }
.btn-danger-ghost { background: transparent; color: var(--text-faint); border-color: transparent; }
.btn-danger-ghost:hover:not(:disabled) { color: var(--red); background: rgba(248,113,113,0.08); }
.btn-sm { height: 29px; padding: 0 11px; font-size: 12px; }
.btn-icon { width: 29px; height: 29px; padding: 0; }
</style>
