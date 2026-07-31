<script setup lang="ts">
defineProps<{
  modelValue: boolean
  disabled?: boolean
}>()

const emit = defineEmits<{ 'update:modelValue': [v: boolean] }>()
</script>

<template>
  <button
    class="gswitch"
    :class="{ on: modelValue }"
    :disabled="disabled"
    role="switch"
    :aria-checked="modelValue"
    @click="emit('update:modelValue', !modelValue)"
  >
    <span class="knob" />
  </button>
</template>

<style scoped>
.gswitch {
  position: relative; width: 36px; height: 20px;
  border-radius: 99px; border: 1px solid var(--glass-border);
  background: var(--glass-hover); cursor: pointer;
  transition: all 0.2s ease; flex-shrink: 0;
}
.gswitch:disabled { opacity: 0.4; cursor: not-allowed; }
.gswitch.on { background: rgba(45,212,191,0.25); border-color: rgba(45,212,191,0.4); }
.knob {
  position: absolute; top: 2px; left: 2px;
  width: 14px; height: 14px; border-radius: 50%;
  background: var(--text-faint);
  transition: all 0.2s var(--ease-spring);
}
.gswitch.on .knob { left: 18px; background: var(--accent); box-shadow: 0 0 8px rgba(45,212,191,0.4); }
</style>
