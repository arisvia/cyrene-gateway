<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api, apiPost, apiDelete } from '@/lib/api'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import { copyText } from '@/lib/format'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import { ArrowLeft, Copy, Check, Save, RotateCcw, Info, AlertTriangle, ExternalLink } from 'lucide-vue-next'

interface ToolModel { id: string; name: string; alias?: string }
interface ToolNote { type: string; text: string }
interface GuideStep { step: number; title: string; desc?: string; value?: string; type?: string }
interface ToolDef {
  id: string; name: string; icon: string; color: string
  description: string; configType: string; docsUrl?: string
  defaultModels?: ToolModel[]; notes?: ToolNote[]
  guideSteps?: GuideStep[]
}
interface ToolStatus { installed: boolean; has9Router: boolean; configPath?: string; message?: string }

const route = useRoute()
const router = useRouter()
const store = useGatewayStore()
const toast = useToast()

const tool = ref<ToolDef | null>(null)
const status = ref<ToolStatus>({ installed: false, has9Router: false })
const loading = ref(true)

const baseUrl = ref(window.location.origin)
const apiKey = ref('')
const model = ref('')
const busy = ref(false)

const statusBadge = computed(() => {
  const s = status.value
  if (!s.installed) return { label: 'Not installed', color: 'glass' as const }
  if (s.has9Router) return { label: 'Connected', color: 'green' as const }
  return { label: 'Not configured', color: 'amber' as const }
})

const isConfigurable = computed(() =>
  tool.value && tool.value.configType !== 'guide' && tool.value.configType !== 'mitm')

function replaceVars(text: string): string {
  const key = apiKey.value || 'sk_9router'
  const base = baseUrl.value.endsWith('/v1') ? baseUrl.value : baseUrl.value + '/v1'
  return text
    .replace(/\{\{baseUrl\}\}/g, base)
    .replace(/\{\{apiKey\}\}/g, key)
    .replace(/\{\{model\}\}/g, model.value || 'provider/model-id')
}

async function load() {
  loading.value = true
  try {
    const res = await api(`/api/cli-tools/${route.params.id}`)
    tool.value = res.tool
    status.value = res.status || { installed: false, has9Router: false }
    if (tool.value?.defaultModels?.length) model.value = tool.value.defaultModels[0].id
  } catch { tool.value = null }
  loading.value = false
}

async function apply() {
  if (!tool.value) return
  busy.value = true
  try {
    const res = await apiPost(`/api/cli-tools/${tool.value.id}`, {
      baseUrl: baseUrl.value, apiKey: apiKey.value, model: model.value,
    })
    if (res.error) toast.error(res.error)
    else { toast.success(`${tool.value.name} configured`); status.value = res.status }
  } catch (e: any) { toast.error(`Failed: ${e.message}`) }
  busy.value = false
}

async function reset() {
  if (!tool.value) return
  if (!confirm(`Remove gateway configuration from ${tool.value.name}?`)) return
  busy.value = true
  try {
    const res = await apiDelete(`/api/cli-tools/${tool.value.id}`)
    if (res.error) toast.error(res.error)
    else { toast.success(`${tool.value.name} reset`); status.value = res.status }
  } catch (e: any) { toast.error(`Failed: ${e.message}`) }
  busy.value = false
}

const copiedField = ref('')
async function copy(text: string, field: string) {
  await copyText(replaceVars(text))
  copiedField.value = field
  setTimeout(() => { copiedField.value = '' }, 2000)
}

onMounted(async () => {
  if (!store.apiKeys.length) await store.loadKeys()
  if (store.apiKeys.length) apiKey.value = store.apiKeys[0].key
  await load()
})
</script>

<template>
  <div style="max-width:720px;margin:0 auto">
    <button class="back-link" @click="router.push('/cli-tools')">
      <ArrowLeft :size="14" /> Back to CLI Tools
    </button>

    <div v-if="loading" class="loading-box">Loading…</div>

    <div v-else-if="!tool" class="loading-box">Tool not found.</div>

    <template v-else>
      <div class="tool-header">
        <div class="tool-icon-lg" :style="{ background: tool.color + '1a' }">
          <img :src="tool.icon" :alt="tool.name" @error="($event.target as HTMLImageElement).style.display='none'">
        </div>
        <div class="tool-head-info">
          <h1 class="page-title" style="margin:0">{{ tool.name }}</h1>
          <p class="page-desc" style="margin:2px 0 6px">{{ tool.description }}</p>
          <div class="flex-gap">
            <GBadge :color="statusBadge.color">{{ statusBadge.label }}</GBadge>
            <a v-if="tool.docsUrl" :href="tool.docsUrl" target="_blank" class="docs-link">
              Docs <ExternalLink :size="11" />
            </a>
          </div>
        </div>
      </div>

      <!-- Notes -->
      <div v-if="tool.notes?.length" class="notes">
        <div v-for="(note, i) in tool.notes" :key="i" :class="['note', `note-${note.type}`]">
          <AlertTriangle v-if="note.type === 'warning' || note.type === 'error'" :size="14" />
          <Info v-else :size="14" />
          <span>{{ note.text }}</span>
        </div>
      </div>

      <!-- Configurable tools: apply form -->
      <GCard v-if="isConfigurable" pad class="form-card">
        <p class="card-section-title">Gateway Configuration</p>

        <div class="form-group">
          <label class="form-label">Base URL</label>
          <input v-model="baseUrl" class="input mono" placeholder="http://localhost:20128">
        </div>

        <div class="form-group">
          <label class="form-label">API Key</label>
          <select v-model="apiKey" class="input">
            <option value="">(none / default)</option>
            <option v-for="k in store.apiKeys" :key="k.id" :value="k.key">
              {{ k.name || k.key.slice(0, 12) + '…' }}
            </option>
          </select>
        </div>

        <div class="form-group">
          <label class="form-label">Model</label>
          <input v-model="model" class="input mono" list="model-suggestions" placeholder="provider/model-id">
          <datalist id="model-suggestions">
            <option v-for="m in tool.defaultModels" :key="m.id" :value="m.id">{{ m.name }}</option>
          </datalist>
          <div v-if="tool.defaultModels?.length" class="model-chips">
            <button
              v-for="m in tool.defaultModels" :key="m.id"
              :class="['chip', model === m.id && 'chip-active']"
              @click="model = m.id"
            >{{ m.name }}</button>
          </div>
        </div>

        <div v-if="status.configPath" class="config-path">
          Config: <code class="mono">{{ status.configPath }}</code>
        </div>

        <div class="form-actions">
          <GButton @click="apply" :disabled="busy || !model">
            <Save :size="13" />{{ busy ? 'Working…' : 'Apply' }}
          </GButton>
          <GButton variant="ghost" @click="reset" :disabled="busy">
            <RotateCcw :size="13" />Reset
          </GButton>
        </div>
      </GCard>

      <!-- Guide tools: manual steps -->
      <GCard v-else-if="tool.configType === 'guide'" pad class="form-card">
        <p class="card-section-title">Setup Guide</p>
        <div class="form-group">
          <label class="form-label">API Key</label>
          <select v-model="apiKey" class="input">
            <option value="">(none / default)</option>
            <option v-for="k in store.apiKeys" :key="k.id" :value="k.key">
              {{ k.name || k.key.slice(0, 12) + '…' }}
            </option>
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">Model</label>
          <input v-model="model" class="input mono" placeholder="provider/model-id">
        </div>

        <div v-for="step in tool.guideSteps" :key="step.step" class="guide-step">
          <div class="step-num" :style="{ background: tool.color }">{{ step.step }}</div>
          <div class="step-body">
            <p class="step-title">{{ step.title }}</p>
            <p v-if="step.desc" class="step-desc">{{ step.desc }}</p>
            <div v-if="step.value" class="step-value-row">
              <code class="step-value mono">{{ replaceVars(step.value) }}</code>
              <button class="copy-btn" @click="copy(step.value!, `step-${step.step}`)">
                <Check v-if="copiedField === `step-${step.step}`" :size="12" />
                <Copy v-else :size="12" />
              </button>
            </div>
          </div>
        </div>
      </GCard>

      <!-- MITM tools -->
      <GCard v-else pad class="form-card">
        <p class="card-section-title">MITM Proxy Required</p>
        <p style="font-size:13px;color:var(--text-muted);line-height:1.6">
          This tool routes traffic through the MITM proxy. Enable MITM in local mode
          (Phase 22) to intercept and route its requests through the gateway.
        </p>
      </GCard>
    </template>
  </div>
</template>

<style scoped>
.back-link {
  display: inline-flex; align-items: center; gap: 5px;
  background: none; border: none; cursor: pointer;
  color: var(--text-muted); font-size: 12.5px; font-family: var(--font);
  margin-bottom: 16px; padding: 0;
}
.back-link:hover { color: var(--text); }
.loading-box { padding: 48px; text-align: center; color: var(--text-faint); font-size: 13px; }
.tool-header { display: flex; align-items: flex-start; gap: 14px; margin-bottom: 18px; }
.tool-icon-lg {
  width: 52px; height: 52px; border-radius: 14px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
}
.tool-icon-lg img { width: 30px; height: 30px; object-fit: contain; }
.tool-head-info { min-width: 0; }
.docs-link {
  display: inline-flex; align-items: center; gap: 3px;
  font-size: 11.5px; color: var(--accent); text-decoration: none;
}
.docs-link:hover { text-decoration: underline; }
.notes { display: flex; flex-direction: column; gap: 8px; margin-bottom: 16px; }
.note {
  display: flex; align-items: flex-start; gap: 8px;
  padding: 10px 12px; border-radius: var(--radius-sm);
  font-size: 12.5px; line-height: 1.5; border: 1px solid;
}
.note-info { background: rgba(56,189,248,0.06); border-color: rgba(56,189,248,0.2); color: var(--accent-2); }
.note-warning { background: rgba(251,191,36,0.06); border-color: rgba(251,191,36,0.2); color: var(--amber); }
.note-error { background: rgba(248,113,113,0.06); border-color: rgba(248,113,113,0.2); color: var(--red); }
.note svg { flex-shrink: 0; margin-top: 1px; }
.form-card { margin-bottom: 16px; }
.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 14px; }
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.input {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font); outline: none; transition: all 0.15s ease;
}
.input.mono { font-family: var(--font-mono); font-size: 12px; }
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
select.input { appearance: none; background-image: var(--select-arrow); background-repeat: no-repeat; background-position: right 10px center; padding-right: 30px; }
.model-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 8px; }
.chip {
  padding: 3px 9px; border-radius: 20px; font-size: 11px; cursor: pointer;
  background: var(--glass-hover); border: 1px solid var(--glass-border);
  color: var(--text-muted); font-family: var(--font); transition: all 0.15s ease;
}
.chip:hover { color: var(--text); border-color: var(--glass-border-hover); }
.chip-active { background: var(--accent); color: var(--on-accent); border-color: transparent; }
.config-path { font-size: 11.5px; color: var(--text-faint); margin-bottom: 14px; }
.config-path code { color: var(--text-muted); }
.form-actions { display: flex; gap: 8px; }
.guide-step { display: flex; align-items: flex-start; gap: 12px; margin-bottom: 16px; }
.step-num {
  width: 26px; height: 26px; border-radius: 50%; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  color: white; font-size: 12px; font-weight: 600;
}
.step-body { flex: 1; min-width: 0; }
.step-title { font-size: 13px; font-weight: 550; }
.step-desc { font-size: 12px; color: var(--text-muted); margin-top: 2px; }
.step-value-row { display: flex; align-items: center; gap: 6px; margin-top: 6px; }
.step-value {
  flex: 1; padding: 7px 10px; border-radius: var(--radius-sm);
  background: var(--code-bg); border: 1px solid var(--glass-border);
  font-size: 11.5px; overflow-x: auto; white-space: nowrap;
}
.copy-btn {
  width: 30px; height: 30px; border-radius: var(--radius-sm); flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--glass-hover); border: 1px solid var(--glass-border);
  color: var(--text-muted); cursor: pointer; transition: all 0.15s ease;
}
.copy-btn:hover { color: var(--text); border-color: var(--glass-border-hover); }
</style>
