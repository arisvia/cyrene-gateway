<script setup lang="ts">
defineProps<{ modelValue: boolean }>()
defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <button
    :class="['switch', modelValue ? 'switch-on' : 'switch-off']"
    type="button"
    @click="$emit('update:modelValue', !modelValue)"
  >
    <span class="switch-thumb"></span>
  </button>
</template>

<style scoped>
.switch {
  position: relative; width: 36px; height: 20px; border-radius: 20px;
  border: 1px solid transparent; cursor: pointer;
  transition: background 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease;
  flex-shrink: 0;
}
.switch:active .switch-thumb { width: 17px; }
.switch-on { background: var(--accent); box-shadow: 0 0 12px var(--ring-soft); }
.switch-off { background: var(--glass-hover); border-color: var(--glass-border); }
.switch-thumb {
  position: absolute; top: 2px; left: 2px; width: 14px; height: 14px;
  border-radius: 8px; background: white;
  transition: transform 0.25s var(--ease-spring), width 0.15s ease;
  box-shadow: 0 1px 3px rgba(0,0,0,0.4);
}
.switch-on .switch-thumb { transform: translateX(16px); }
.switch-on:active .switch-thumb { transform: translateX(13px); }
</style>
