<script setup lang="ts">
defineProps<{ pad?: boolean; interactive?: boolean }>()
</script>

<template>
  <div :class="['glass-card', pad && 'card-pad', interactive && 'interactive']"><slot /></div>
</template>

<style scoped>
.glass-card {
  position: relative;
  background: var(--glass);
  border: 1px solid var(--glass-border);
  border-radius: var(--radius);
  backdrop-filter: blur(var(--glass-blur)); -webkit-backdrop-filter: blur(var(--glass-blur));
  box-shadow: var(--glass-depth);
  transition: border-color 0.25s ease, box-shadow 0.3s var(--ease-out-expo), transform 0.3s var(--ease-out-expo), background 0.25s ease;
  overflow: hidden;
}
/* Top-edge light refraction line */
.glass-card::before {
  content: '';
  position: absolute; inset: 0 0 auto 0; height: 1px;
  background: var(--glass-edge);
  pointer-events: none;
}
/* Diagonal sheen sweep */
.glass-card::after {
  content: '';
  position: absolute; inset: 0;
  background: var(--glass-sheen);
  pointer-events: none;
  opacity: 0.5;
  transition: opacity 0.3s ease;
}
.glass-card:hover { border-color: var(--glass-border-hover); }
.glass-card:hover::after { opacity: 1; }

.interactive:hover {
  transform: translateY(-2px);
  box-shadow: var(--glass-depth-hover);
}
.card-pad { padding: 18px 20px; }
</style>
