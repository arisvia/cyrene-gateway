<script setup lang="ts">
defineProps<{ title: string; desc?: string; width?: string }>()
defineEmits<{ close: [] }>()
</script>

<template>
  <div class="dialog-overlay" @click.self="$emit('close')">
    <div class="dialog" :style="width ? { maxWidth: width } : undefined">
      <h3 class="dialog-title">{{ title }}</h3>
      <p v-if="desc" class="dialog-desc">{{ desc }}</p>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.dialog-overlay {
  position: fixed; inset: 0; z-index: 100;
  background: var(--overlay-bg);
  backdrop-filter: blur(4px); -webkit-backdrop-filter: blur(4px);
  display: flex; align-items: center; justify-content: center;
  animation: fadeIn 0.15s ease;
}
.dialog {
  width: 100%; max-width: 480px; margin: 16px;
  background: var(--dialog-bg);
  backdrop-filter: blur(24px); -webkit-backdrop-filter: blur(24px);
  border: 1px solid var(--glass-border-hover);
  border-radius: 14px; padding: 24px;
  box-shadow: 0 24px 64px rgba(0,0,0,0.35), var(--shadow-glow);
  animation: slideUp 0.2s ease;
  max-height: 88vh; overflow-y: auto;
}
.dialog-title { font-size: 15px; font-weight: 650; margin-bottom: 4px; }
.dialog-desc { font-size: 12px; color: var(--text-muted); margin-bottom: 18px; }
</style>
