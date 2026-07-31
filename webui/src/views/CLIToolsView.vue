<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '@/lib/api'
import { useGatewayStore, type CLITool } from '@/stores/gateway'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GSkeleton from '@/components/ui/GSkeleton.vue'
import { TerminalSquare } from 'lucide-vue-next'

const router = useRouter()
const loading = ref(true)
const tools = ref<CLITool[]>([])
const statuses = ref<Record<string, any>>({})

onMounted(async () => {
  try {
    const [t, s] = await Promise.all([api('/api/cli-tools'), api('/api/cli-tools/all-statuses')])
    tools.value = Array.isArray(t?.tools) ? t.tools : (Array.isArray(t) ? t : [])
    statuses.value = s?.statuses || s || {}
  } catch { tools.value = [] }
  loading.value = false
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">CLI Tools</h1>
      <p class="page-desc">Configure AI coding assistants to route through this gateway</p>
    </header>

    <GSkeleton v-if="loading" height="200px" />
    <div v-else class="grid stagger">
      <GCard v-for="tool in tools" :key="tool.id" class="tool-card" @click="router.push(`/cli-tools/${tool.id}`)">
        <div class="tool-icon"><TerminalSquare :size="16" /></div>
        <div class="tool-info">
          <p class="tool-name">{{ tool.name }}</p>
          <p class="tool-desc">{{ tool.description || tool.id }}</p>
        </div>
        <GBadge :color="statuses[tool.id]?.configured ? 'green' : 'gray'">
          {{ statuses[tool.id]?.configured ? 'configured' : 'not set' }}
        </GBadge>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 10px; }
.tool-card { display: flex; align-items: center; gap: 12px; padding: 14px 16px; cursor: pointer; }
.tool-icon {
  width: 36px; height: 36px; border-radius: 10px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient-soft); border: 1px solid var(--glass-border); color: var(--accent);
}
.tool-info { flex: 1; min-width: 0; }
.tool-name { font-size: 13px; font-weight: 600; }
.tool-desc { font-size: 11px; color: var(--text-faint); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
