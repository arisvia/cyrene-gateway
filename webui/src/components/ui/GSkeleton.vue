<template>
  <div class="skeleton" :style="{ width, height, borderRadius: radius }"></div>
</template>

<script setup lang="ts">
defineProps<{ width?: string; height?: string; radius?: string }>()
</script>

<style scoped>
.skeleton {
  background: linear-gradient(90deg, var(--glass-hover) 25%, var(--glass) 50%, var(--glass-hover) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  border-radius: v-bind(radius || 'var(--radius-sm)');
}
@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
@media (prefers-reduced-motion: reduce) {
  .skeleton { animation: none; }
}
</style>
