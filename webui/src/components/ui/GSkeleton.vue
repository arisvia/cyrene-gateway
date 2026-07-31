<template>
  <div class="gskeleton" :style="{ width, height }" />
</template>

<script setup lang="ts">
defineProps<{ width?: string; height?: string }>()
</script>

<style scoped>
.gskeleton {
  border-radius: var(--radius-sm);
  background: linear-gradient(90deg, var(--glass-hover) 25%, var(--glass) 50%, var(--glass-hover) 75%);
  background-size: 200% 100%;
  animation: shimmer 1.5s infinite;
  min-height: 16px;
}
</style>
