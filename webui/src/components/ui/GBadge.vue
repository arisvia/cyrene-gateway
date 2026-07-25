<script setup lang="ts">
defineProps<{
  color?: 'glass' | 'green' | 'red' | 'amber' | 'blue' | 'violet'
}>()
</script>

<template>
  <span :class="['badge', `badge-${color || 'glass'}`]"><slot /></span>
</template>

<style scoped>
.badge {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 2px 8px; border-radius: 20px;
  font-size: 10.5px; font-weight: 600; letter-spacing: 0.01em;
  border: 1px solid transparent;
}
.badge-glass { background: var(--glass-hover); color: var(--text-muted); border-color: var(--glass-border); }
.badge-green { background: rgba(52,211,153,0.1); color: var(--green); border-color: rgba(52,211,153,0.2); }
.badge-red { background: rgba(248,113,113,0.1); color: var(--red); border-color: rgba(248,113,113,0.2); }
.badge-amber { background: rgba(251,191,36,0.1); color: var(--amber); border-color: rgba(251,191,36,0.2); }
.badge-blue { background: rgba(56,189,248,0.1); color: var(--accent-2); border-color: rgba(56,189,248,0.2); }
.badge-violet { background: rgba(167,139,250,0.1); color: var(--accent-3); border-color: rgba(167,139,250,0.2); }
</style>
