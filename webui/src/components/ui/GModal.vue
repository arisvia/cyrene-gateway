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
  backdrop-filter: blur(6px); -webkit-backdrop-filter: blur(6px);
  display: flex; align-items: center; justify-content: center;
  animation: overlayIn 0.2s ease;
}
.dialog {
  position: relative;
  width: 100%; max-width: 480px; margin: 16px;
  background: var(--dialog-bg);
  backdrop-filter: blur(var(--glass-blur-heavy)); -webkit-backdrop-filter: blur(var(--glass-blur-heavy));
  border: 1px solid var(--glass-border-hover);
  border-radius: 16px; padding: 24px;
  box-shadow: var(--glass-depth-hover), var(--shadow-glow);
  animation: dialogIn 0.3s var(--ease-spring);
  max-height: 88vh; overflow-y: auto;
}
/* Top-edge refraction */
.dialog::before {
  content: '';
  position: absolute; inset: 0 0 auto 0; height: 1px;
  background: var(--glass-edge);
  border-radius: 16px 16px 0 0;
  pointer-events: none;
}
.dialog-title { font-size: 15px; font-weight: 650; margin-bottom: 4px; }
.dialog-desc { font-size: 12px; color: var(--text-muted); margin-bottom: 18px; }

@keyframes overlayIn { from { opacity: 0; } }
@keyframes dialogIn {
  from { opacity: 0; transform: translateY(16px) scale(0.95); }
  to { opacity: 1; transform: translateY(0) scale(1); }
}
</style>
