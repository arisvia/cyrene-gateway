<script setup lang="ts">
defineProps<{
  color?: 'green' | 'amber' | 'red' | 'blue' | 'violet' | 'teal' | 'gray'
  variant?: 'solid' | 'soft'
}>()
</script>

<template>
  <span class="gbadge" :class="[color || 'gray', variant || 'soft']">
    <slot />
  </span>
</template>

<style scoped>
.gbadge {
  display: inline-flex; align-items: center; gap: 4px;
  padding: 2px 8px; border-radius: 99px;
  font-size: 11px; font-weight: 600; letter-spacing: 0.02em;
  white-space: nowrap;
}
.gbadge.green { color: var(--green); background: rgba(52,211,153,0.1); border: 1px solid rgba(52,211,153,0.2); }
.gbadge.amber { color: var(--amber); background: rgba(251,191,36,0.1); border: 1px solid rgba(251,191,36,0.2); }
.gbadge.red { color: var(--red); background: rgba(248,113,113,0.1); border: 1px solid rgba(248,113,113,0.2); }
.gbadge.blue { color: var(--blue); background: rgba(96,165,250,0.1); border: 1px solid rgba(96,165,250,0.2); }
.gbadge.violet { color: #a78bfa; background: rgba(139,92,246,0.1); border: 1px solid rgba(139,92,246,0.2); }
.gbadge.teal { color: var(--accent); background: rgba(45,212,191,0.1); border: 1px solid rgba(45,212,191,0.2); }
.gbadge.gray { color: var(--text-muted); background: var(--glass-hover); border: 1px solid var(--glass-border); }
</style>
