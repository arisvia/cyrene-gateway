<script setup lang="ts">
defineProps<{ icon?: string; title?: string; desc?: string }>()
</script>

<template>
  <div class="gempty">
    <div class="icon-wrap"><slot name="icon"><span class="fallback-icon">{{ icon || '◇' }}</span></slot></div>
    <p class="title">{{ title || 'Nothing here yet' }}</p>
    <p class="desc" v-if="desc">{{ desc }}</p>
    <slot />
  </div>
</template>

<style scoped>
.gempty {
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  padding: 48px 24px; text-align: center;
}
.icon-wrap {
  width: 48px; height: 48px; border-radius: 14px;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient-soft); border: 1px solid var(--glass-border);
  margin-bottom: 14px; color: var(--accent);
}
.fallback-icon { font-size: 20px; }
.title { font-size: 14px; font-weight: 600; margin-bottom: 4px; }
.desc { font-size: 12.5px; color: var(--text-faint); max-width: 360px; }
</style>
