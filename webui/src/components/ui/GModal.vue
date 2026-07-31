<script setup lang="ts">
import { onMounted, onBeforeUnmount } from 'vue'

const props = defineProps<{
  title?: string
  width?: string
}>()

const emit = defineEmits<{ close: [] }>()

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => document.addEventListener('keydown', onKey))
onBeforeUnmount(() => document.removeEventListener('keydown', onKey))
</script>

<template>
  <Teleport to="body">
    <div class="overlay" @click.self="emit('close')">
      <div class="modal" :style="{ maxWidth: width || '480px' }" role="dialog" aria-modal="true">
        <div class="modal-head" v-if="title">
          <h3 class="modal-title">{{ title }}</h3>
          <button class="close-btn" @click="emit('close')" aria-label="Close">✕</button>
        </div>
        <div class="modal-body">
          <slot />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.overlay {
  position: fixed; inset: 0; z-index: 100;
  display: flex; align-items: center; justify-content: center;
  background: rgba(0,0,0,0.55);
  backdrop-filter: blur(4px);
  animation: fadeIn 0.15s ease;
  padding: 20px;
}
.modal {
  width: 100%;
  max-height: 85vh; overflow-y: auto;
  background: var(--bg-elevated);
  border: 1px solid var(--glass-border-hover);
  border-radius: 16px;
  box-shadow: var(--glass-depth);
  animation: scaleIn 0.2s var(--ease-spring);
}
.modal-head {
  display: flex; align-items: center; justify-content: space-between;
  padding: 18px 22px 0;
}
.modal-title { font-size: 15px; font-weight: 650; letter-spacing: -0.01em; }
.close-btn {
  width: 28px; height: 28px; border-radius: var(--radius-xs);
  display: flex; align-items: center; justify-content: center;
  background: transparent; border: none; color: var(--text-faint);
  cursor: pointer; font-size: 13px; transition: all 0.15s ease;
}
.close-btn:hover { background: var(--glass-hover); color: var(--text); }
.modal-body { padding: 18px 22px 22px; }
</style>
