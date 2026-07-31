<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api, apiPost, apiDelete } from '@/lib/api'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GSkeleton from '@/components/ui/GSkeleton.vue'
import { ArrowLeft, Trash2, Check } from 'lucide-vue-next'

const props = defineProps<{ id: string }>()
const router = useRouter()
const toast = useToast()

const loading = ref(true)
const tool = ref<any>(null)
const configValue = ref('')
const applying = ref(false)
const resetting = ref(false)

onMounted(async () => {
  try {
    const r = await api(`/api/cli-tools/${props.id}`)
    tool.value = r?.tool || r
    configValue.value = tool.value?.currentValue || ''
  } catch (e: any) {
    toast.error(`Failed to load tool: ${e.message}`)
  }
  loading.value = false
})

async function apply() {
  applying.value = true
  try {
    await apiPost(`/api/cli-tools/${props.id}`, { value: configValue.value })
    toast.success(`${tool.value?.name || props.id} configured`)
  } catch (e: any) { toast.error(e.message) }
  applying.value = false
}

async function reset() {
  resetting.value = true
  try {
    await apiDelete(`/api/cli-tools/${props.id}`)
    toast.success('Configuration reset')
    configValue.value = ''
  } catch (e: any) { toast.error(e.message) }
  resetting.value = false
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <button class="back-btn" @click="router.push('/cli-tools')" aria-label="Back"><ArrowLeft :size="15" /></button>
      <div>
        <h1 class="page-title">{{ tool?.name || id }}</h1>
        <p class="page-desc">{{ tool?.description || '' }}</p>
      </div>
      <GBadge v-if="tool?.configured" color="green">configured</GBadge>
    </header>

    <GSkeleton v-if="loading" height="200px" />
    <template v-else-if="tool">
      <!-- Config form -->
      <GCard v-if="tool.configType !== 'guide'" class="config-card">
        <label class="field-label">{{ tool.configType === 'env' ? 'API Key / Token' : 'Configuration Value' }}</label>
        <input v-model="configValue" class="field" :type="tool.configType === 'env' ? 'password' : 'text'" placeholder="Enter value…">
        <div class="config-actions">
          <GButton size="sm" :loading="applying" @click="apply"><Check :size="13" /> Apply</GButton>
          <GButton variant="danger" size="sm" :loading="resetting" @click="reset"><Trash2 :size="13" /> Reset</GButton>
        </div>
      </GCard>

      <!-- Guide steps -->
      <GCard v-if="tool.guide?.length" class="guide-card">
        <p class="guide-title">Setup Guide</p>
        <ol class="guide-steps">
          <li v-for="(step, i) in tool.guide" :key="i" class="guide-step">
            <span class="step-num">{{ i + 1 }}</span>
            <span class="step-text" v-html="step" />
          </li>
        </ol>
      </GCard>
    </template>
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: center; gap: 12px; margin-bottom: 22px; }
.back-btn {
  width: 32px; height: 32px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  background: var(--glass); border: 1px solid var(--glass-border);
  color: var(--text-muted); cursor: pointer; transition: all 0.15s ease;
}
.back-btn:hover { color: var(--text); border-color: var(--glass-border-hover); }
.page-title { font-size: 18px; font-weight: 700; letter-spacing: -0.02em; }
.page-desc { font-size: 12px; color: var(--text-faint); margin-top: 2px; }
.config-card { max-width: 480px; margin-bottom: 16px; }
.field-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.field {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; outline: none; transition: all 0.15s ease;
}
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.config-actions { display: flex; gap: 8px; margin-top: 14px; }
.guide-card { max-width: 560px; }
.guide-title { font-size: 13px; font-weight: 650; margin-bottom: 12px; }
.guide-steps { list-style: none; display: flex; flex-direction: column; gap: 10px; }
.guide-step { display: flex; gap: 10px; align-items: flex-start; }
.step-num {
  width: 22px; height: 22px; border-radius: 50%; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient-soft); border: 1px solid rgba(45,212,191,0.2);
  font-size: 10.5px; font-weight: 700; color: var(--accent);
}
.step-text { font-size: 12.5px; color: var(--text-muted); line-height: 1.6; }
.step-text :deep(code) {
  background: var(--code-bg); padding: 1px 6px; border-radius: 4px;
  font-size: 11px; color: var(--text);
}
</style>
