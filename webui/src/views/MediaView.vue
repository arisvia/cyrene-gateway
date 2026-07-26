<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { api } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'

interface MediaProvider {
  provider: string
  name: string
  kinds: string[]
  models?: { id: string; name: string; kind: string }[]
  configs?: Record<string, { baseUrl: string; format: string; authType: string }>
}

const kinds = [
  { id: 'embedding', label: 'Embedding', desc: 'Text → Vector embeddings' },
  { id: 'image', label: 'Text to Image', desc: 'Prompt → Image generation' },
  { id: 'tts', label: 'Text to Speech', desc: 'Text → Audio synthesis' },
  { id: 'stt', label: 'Speech to Text', desc: 'Audio → Transcript' },
  { id: 'video', label: 'Video', desc: 'Prompt → Video generation' },
  { id: 'web-fetch', label: 'Web Fetch', desc: 'URL → Content extraction' },
  { id: 'web-search', label: 'Web Search', desc: 'Query → Search results' },
]

const activeKind = ref('embedding')
const providers = ref<MediaProvider[]>([])
const loading = ref(false)

const activeKindInfo = computed(() => kinds.find(k => k.id === activeKind.value)!)

async function loadProviders() {
  loading.value = true
  try {
    const res = await api(`/api/media-providers?kind=${activeKind.value}`)
    providers.value = res.providers || []
  } catch {
    providers.value = []
  }
  loading.value = false
}

function switchKind(kind: string) {
  activeKind.value = kind
  loadProviders()
}

type BadgeColor = 'glass' | 'green' | 'red' | 'amber' | 'blue' | 'violet'
function kindBadgeColor(kind: string): BadgeColor {
  const map: Record<string, BadgeColor> = {
    embedding: 'blue', image: 'violet', tts: 'green', stt: 'amber',
    video: 'red', 'web-fetch': 'blue', 'web-search': 'amber',
  }
  return map[kind] || 'glass'
}

onMounted(loadProviders)
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Media Providers</h1>
      <p class="page-desc">Embedding, Image, TTS, STT, Video, Web Fetch & Search — unified API routing.</p>
    </div>

    <!-- Kind tabs -->
    <div class="kind-tabs">
      <button
        v-for="k in kinds" :key="k.id"
        :class="['kind-tab', activeKind === k.id && 'active']"
        @click="switchKind(k.id)"
      >
        {{ k.label }}
      </button>
    </div>

    <p class="kind-desc">{{ activeKindInfo.desc }}</p>

    <!-- Provider cards -->
    <div v-if="loading" class="loading-state">Loading providers…</div>
    <div v-else-if="providers.length === 0" class="empty-state">
      No providers registered for {{ activeKindInfo.label }}.
    </div>
    <div v-else class="provider-grid">
      <GCard v-for="p in providers" :key="p.provider" pad class="provider-card">
        <div class="provider-header">
          <span class="provider-name">{{ p.name }}</span>
          <GBadge :color="kindBadgeColor(activeKind)">{{ activeKindInfo.label }}</GBadge>
        </div>
        <p class="provider-id">{{ p.provider }}</p>
        <div v-if="p.models && p.models.length" class="model-list">
          <span v-for="m in p.models.slice(0, 4)" :key="m.id" class="model-chip">{{ m.name }}</span>
          <span v-if="p.models.length > 4" class="model-chip more">+{{ p.models.length - 4 }}</span>
        </div>
        <div v-if="p.configs && p.configs[activeKind]" class="config-info">
          <span class="config-label">Format:</span> {{ p.configs[activeKind].format }}
          <span class="config-label" style="margin-left:12px">Auth:</span> {{ p.configs[activeKind].authType }}
        </div>
      </GCard>
    </div>

    <!-- API Examples -->
    <GCard pad style="margin-top:20px">
      <p class="card-section-title">API Endpoints</p>
      <div class="endpoint-list">
        <div class="endpoint-row"><code>POST /v1/embeddings</code><span>OpenAI-compatible embedding</span></div>
        <div class="endpoint-row"><code>POST /v1/images/generations</code><span>Image generation</span></div>
        <div class="endpoint-row"><code>POST /v1/audio/speech</code><span>Text-to-Speech</span></div>
        <div class="endpoint-row"><code>POST /v1/audio/transcriptions</code><span>Speech-to-Text</span></div>
        <div class="endpoint-row"><code>POST /v1/videos/generations</code><span>Video generation</span></div>
        <div class="endpoint-row"><code>GET /v1/videos/{id}</code><span>Video job status</span></div>
        <div class="endpoint-row"><code>POST /v1/web/fetch</code><span>Web content extraction</span></div>
        <div class="endpoint-row"><code>POST /v1/search</code><span>Web search</span></div>
      </div>
    </GCard>
  </div>
</template>

<style scoped>
.kind-tabs {
  display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 8px;
}
.kind-tab {
  padding: 6px 14px; border-radius: var(--radius-sm);
  font-size: 12.5px; font-weight: 500; cursor: pointer;
  background: var(--glass-bg); border: 1px solid var(--glass-border);
  color: var(--text-muted); transition: all 0.15s ease;
}
.kind-tab:hover { color: var(--text); background: var(--glass-hover); }
.kind-tab.active {
  color: var(--text); background: var(--glass-hover);
  border-color: var(--accent); box-shadow: var(--shadow-glow);
}
.kind-desc { font-size: 12px; color: var(--text-faint); margin-bottom: 16px; }

.provider-grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px;
}
.provider-card { transition: border-color 0.15s ease; }
.provider-card:hover { border-color: var(--accent); }
.provider-header { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.provider-name { font-size: 14px; font-weight: 600; }
.provider-id { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); margin-top: 2px; }
.model-list { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 10px; }
.model-chip {
  font-size: 11px; padding: 2px 8px; border-radius: 4px;
  background: var(--glass-bg); border: 1px solid var(--glass-border);
  color: var(--text-muted);
}
.model-chip.more { color: var(--text-faint); font-style: italic; }
.config-info { margin-top: 10px; font-size: 11.5px; color: var(--text-muted); }
.config-label { font-weight: 600; color: var(--text-faint); }

.loading-state, .empty-state {
  padding: 32px; text-align: center; color: var(--text-muted); font-size: 13px;
}

.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 12px; }
.endpoint-list { display: flex; flex-direction: column; gap: 6px; }
.endpoint-row {
  display: flex; align-items: center; gap: 12px; font-size: 12.5px;
  padding: 6px 0; border-bottom: 1px solid var(--glass-border);
}
.endpoint-row:last-child { border-bottom: none; }
.endpoint-row code {
  font-family: var(--font-mono); font-size: 11.5px; color: var(--accent);
  background: var(--glass-bg); padding: 2px 6px; border-radius: 4px;
}
.endpoint-row span { color: var(--text-muted); }
</style>
