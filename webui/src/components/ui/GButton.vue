<script setup lang="ts">
defineProps<{
  variant?: 'primary' | 'ghost' | 'danger' | 'outline'
  size?: 'sm' | 'md'
  disabled?: boolean
  loading?: boolean
}>()
</script>

<template>
  <button
    class="gbtn"
    :class="[variant || 'primary', size || 'md']"
    :disabled="disabled || loading"
  >
    <span v-if="loading" class="spinner"></span>
    <slot />
  </button>
</template>

<style scoped>
.gbtn {
  display: inline-flex; align-items: center; gap: 6px;
  border: 1px solid transparent; border-radius: var(--radius-sm);
  font-weight: 560; font-size: 13px; cursor: pointer;
  transition: all var(--duration) ease;
  white-space: nowrap; user-select: none;
}
.gbtn:disabled { opacity: 0.5; cursor: not-allowed; }

.gbtn.md { height: 34px; padding: 0 14px; }
.gbtn.sm { height: 28px; padding: 0 10px; font-size: 12px; }

.gbtn.primary {
  background: var(--gradient); color: var(--on-accent);
  box-shadow: var(--shadow-accent);
}
.gbtn.primary:hover:not(:disabled) {
  box-shadow: var(--shadow-accent-hover);
  transform: translateY(-1px);
}

.gbtn.ghost {
  background: var(--glass); color: var(--text-muted);
  border-color: var(--glass-border);
}
.gbtn.ghost:hover:not(:disabled) {
  background: var(--glass-hover); color: var(--text);
  border-color: var(--glass-border-hover);
}

.gbtn.outline {
  background: transparent; color: var(--accent);
  border-color: rgba(45,212,191,0.3);
}
.gbtn.outline:hover:not(:disabled) {
  background: rgba(45,212,191,0.08);
  border-color: rgba(45,212,191,0.5);
}

.gbtn.danger {
  background: rgba(248,113,113,0.1); color: var(--red);
  border-color: rgba(248,113,113,0.25);
}
.gbtn.danger:hover:not(:disabled) {
  background: rgba(248,113,113,0.18);
}

.gbtn:active:not(:disabled) { transform: scale(0.97); }

.spinner {
  width: 13px; height: 13px; border-radius: 50%;
  border: 2px solid currentColor; border-top-color: transparent;
  animation: spin 0.6s linear infinite;
}
@keyframes spin { to { transform: rotate(360deg); } }
</style>
