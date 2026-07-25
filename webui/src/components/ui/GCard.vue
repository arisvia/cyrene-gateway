<script setup lang="ts">
defineProps<{ pad?: boolean }>()
</script>

<template>
  <div :class="['glass-card', pad && 'card-pad']"><slot /></div>
</template>

<style scoped>
.glass-card {
  background: var(--glass);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius);
  backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px);
  transition: border-color 0.2s ease;
  overflow: hidden;
}
.glass-card:hover { border-color: var(--glass-border-hover); }
.card-pad { padding: 18px 20px; }
</style>
