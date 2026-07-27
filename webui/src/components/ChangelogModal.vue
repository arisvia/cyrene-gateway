<script setup lang="ts">
import { ref, onMounted } from 'vue'
import GModal from '@/components/ui/GModal.vue'
import GButton from '@/components/ui/GButton.vue'

const props = defineProps<{ version: string }>()

const CHANGELOG_KEY = 'cyrene-changelog-seen'

const show = ref(false)

const notes: { version: string; items: string[] }[] = [
  {
    version: '0.21.0',
    items: [
      'Internationalization: 10 languages (EN, ZH, JA, KO, RU, ES, FR, DE, PT-BR, TR)',
      'Language switcher in sidebar footer',
      'Mobile responsive layout with collapsible sidebar',
      'Keyboard shortcuts: Cmd/Ctrl+K to search, Esc to close modals',
      'Accessibility: focus-visible outlines, aria labels',
      'Loading skeletons for async content',
      'Changelog modal on version update',
    ],
  },
]

onMounted(() => {
  const seen = localStorage.getItem(CHANGELOG_KEY)
  if (seen !== props.version) {
    show.value = true
  }
})

function dismiss() {
  show.value = false
  localStorage.setItem(CHANGELOG_KEY, props.version)
}
</script>

<template>
  <GModal v-if="show" title="What's New" :desc="`Cyrene Gateway v${version}`" width="440px" @close="dismiss">
    <div class="changelog">
      <div v-for="entry in notes" :key="entry.version" class="changelog-entry">
        <p class="changelog-ver">v{{ entry.version }}</p>
        <ul class="changelog-list">
          <li v-for="(item, i) in entry.items" :key="i">{{ item }}</li>
        </ul>
      </div>
    </div>
    <div class="modal-actions">
      <GButton @click="dismiss">Got it</GButton>
    </div>
  </GModal>
</template>

<style scoped>
.changelog { max-height: 300px; overflow-y: auto; }
.changelog-entry { margin-bottom: 16px; }
.changelog-ver { font-size: 13px; font-weight: 600; color: var(--accent); margin-bottom: 8px; }
.changelog-list { padding-left: 18px; font-size: 12.5px; color: var(--text-muted); line-height: 1.8; }
.changelog-list li::marker { color: var(--text-faint); }
.modal-actions { display: flex; justify-content: flex-end; margin-top: 12px; }
</style>
