<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { ChevronRight, Terminal, ShieldAlert } from 'lucide-vue-next'

interface ToolDef {
  id: string
  name: string
  icon: string
  color: string
  description: string
  configType: string
}

interface ToolStatus {
  installed: boolean
  has9Router: boolean
  message?: string
}

const router = useRouter()
const tools = ref<ToolDef[]>([])
const statuses = ref<Record<string, ToolStatus>>({})
const loading = ref(true)

function statusBadge(s: ToolStatus | undefined) {
  if (!s) return { label: 'Unknown', color: 'glass' as const }
  if (!s.installed) return { label: 'Not installed', color: 'glass' as const }
  if (s.has9Router) return { label: 'Connected', color: 'green' as const }
  return { label: 'Not configured', color: 'amber' as const }
}

const configurable = ref<ToolDef[]>([])
const manualTools = ref<ToolDef[]>([])

onMounted(async () => {
  try {
    const [listRes, statusRes] = await Promise.all([
      api('/api/cli-tools'),
      api('/api/cli-tools/all-statuses'),
    ])
    tools.value = listRes.tools || []
    statuses.value = statusRes || {}
    configurable.value = tools.value.filter(t => t.configType !== 'guide' && t.configType !== 'mitm')
    manualTools.value = tools.value.filter(t => t.configType === 'guide' || t.configType === 'mitm')
  } catch { /* ignore */ }
  loading.value = false
})
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">CLI Tools</h1>
      <p class="page-desc">Connect AI coding CLIs to route through the gateway.</p>
    </div>

    <div v-if="loading" class="grid-cards">
      <GCard v-for="i in 6" :key="i" pad><div class="skeleton-line"></div></GCard>
    </div>

    <template v-else>
      <div class="grid-cards">
        <GCard
          v-for="tool in configurable" :key="tool.id" pad interactive
          class="tool-card" @click="router.push(`/cli-tools/${tool.id}`)"
        >
          <div class="tool-card-inner">
            <div class="tool-icon" :style="{ background: tool.color + '1a' }">
              <img :src="tool.icon" :alt="tool.name" @error="($event.target as HTMLImageElement).style.display='none'">
            </div>
            <div class="tool-info">
              <span class="tool-name">{{ tool.name }}</span>
              <GBadge :color="statusBadge(statuses[tool.id]).color">{{ statusBadge(statuses[tool.id]).label }}</GBadge>
            </div>
            <ChevronRight :size="15" class="tool-chevron" />
          </div>
        </GCard>
      </div>

      <template v-if="manualTools.length">
        <div class="section-divider">
          <ShieldAlert :size="14" />
          <span>Manual Setup / MITM</span>
        </div>
        <div class="grid-cards">
          <GCard
            v-for="tool in manualTools" :key="tool.id" pad interactive
            class="tool-card" @click="router.push(`/cli-tools/${tool.id}`)"
          >
            <div class="tool-card-inner">
              <div class="tool-icon" :style="{ background: tool.color + '1a' }">
                <img :src="tool.icon" :alt="tool.name" @error="($event.target as HTMLImageElement).style.display='none'">
              </div>
              <div class="tool-info">
                <span class="tool-name">{{ tool.name }}</span>
                <GBadge color="glass">{{ tool.configType === 'mitm' ? 'MITM' : 'Manual' }}</GBadge>
              </div>
              <ChevronRight :size="15" class="tool-chevron" />
            </div>
          </GCard>
        </div>
      </template>

      <GEmpty v-if="tools.length === 0">
        <Terminal :size="24" style="margin-bottom:8px;opacity:0.4" />
        <p>No CLI tools available.</p>
      </GEmpty>
    </template>
  </div>
</template>

<style scoped>
.grid-cards {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 12px;
}
.tool-card { cursor: pointer; }
.tool-card-inner {
  display: flex; align-items: center; gap: 12px;
}
.tool-icon {
  width: 38px; height: 38px; border-radius: 10px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
}
.tool-icon img { width: 22px; height: 22px; object-fit: contain; }
.tool-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.tool-name { font-size: 13.5px; font-weight: 550; }
.tool-chevron { color: var(--text-faint); flex-shrink: 0; }
.section-divider {
  display: flex; align-items: center; gap: 6px;
  margin: 24px 0 12px; font-size: 11px; font-weight: 600;
  text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint);
}
.skeleton-line {
  height: 38px; border-radius: 8px;
  background: var(--glass-hover);
  animation: pulse 1.5s ease infinite;
}
@keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.5; } }
</style>
