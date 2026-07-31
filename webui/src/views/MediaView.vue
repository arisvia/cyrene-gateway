<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { api } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GSkeleton from '@/components/ui/GSkeleton.vue'
import GEmpty from '@/components/ui/GEmpty.vue'

const props = defineProps<{ kind: string }>()

const loading = ref(true)
const providers = ref<any[]>([])

const kindLabels: Record<string, string> = {
  embedding: 'Embedding', image: 'Text to Image', tts: 'Text to Speech',
  stt: 'Speech to Text', video: 'Video', 'web-fetch': 'Web Fetch & Search', 'web-search': 'Web Search',
}

const endpointMap: Record<string, string> = {
  embedding: 'POST /v1/embeddings', image: 'POST /v1/images/generations',
  tts: 'POST /v1/audio/speech', stt: 'POST /v1/audio/transcriptions',
  video: 'POST /v1/videos/generations', 'web-fetch': 'POST /v1/web/fetch', 'web-search': 'POST /v1/web/search',
}

async function load() {
  loading.value = true
  try {
    const r = await api(`/api/media-providers?kind=${props.kind}`)
    providers.value = Array.isArray(r?.providers) ? r.providers : []
  } catch { providers.value = [] }
  loading.value = false
}

onMounted(load)
watch(() => props.kind, load)
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">{{ kindLabels[kind] || kind }}</h1>
      <p class="page-desc">Media providers for {{ kindLabels[kind]?.toLowerCase() || kind }} · <code class="ep-code">{{ endpointMap[kind] || '—' }}</code></p>
    </header>

    <GSkeleton v-if="loading" height="200px" />
    <GEmpty v-else-if="!providers.length" title="No providers" desc="No media providers registered for this kind yet." />
    <div v-else class="grid stagger">
      <GCard v-for="p in providers" :key="p.id" class="p-card">
        <div class="p-top">
          <span class="p-name">{{ p.name || p.id }}</span>
          <GBadge v-if="p.authType" color="violet">{{ p.authType }}</GBadge>
        </div>
        <p class="p-id">{{ p.id }}</p>
        <p v-if="p.baseUrl" class="p-url">{{ p.baseUrl }}</p>
        <div v-if="p.models?.length" class="p-models">
          <GBadge v-for="m in p.models.slice(0, 4)" :key="m" color="gray">{{ m }}</GBadge>
          <span v-if="p.models.length > 4" class="more">+{{ p.models.length - 4 }}</span>
        </div>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.ep-code { font-size: 11px; background: var(--code-bg); padding: 2px 7px; border-radius: 4px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 10px; }
.p-card { padding: 14px 16px; display: flex; flex-direction: column; gap: 6px; }
.p-top { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.p-name { font-size: 13px; font-weight: 600; }
.p-id { font-size: 10.5px; color: var(--text-faint); font-family: var(--font-mono); }
.p-url { font-size: 11px; color: var(--text-muted); word-break: break-all; }
.p-models { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 4px; }
.more { font-size: 10.5px; color: var(--text-faint); align-self: center; }
</style>
