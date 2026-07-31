<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import GBadge from '@/components/ui/GBadge.vue'

const store = useGatewayStore()
const toast = useToast()

const enabled = ref(false)
const level = ref('caveman')
const saving = ref(false)

onMounted(async () => {
  await store.loadSettings()
  const ts = store.settings.tokenSaver || {}
  enabled.value = !!ts.enabled
  level.value = ts.level || 'caveman'
})

async function save() {
  saving.value = true
  try {
    await store.saveSettings({ tokenSaver: { enabled: enabled.value, level: level.value } })
  } catch (e: any) { toast.error(e.message) }
  saving.value = false
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">Token Saver</h1>
      <p class="page-desc">RTK prompt compression — reduce token usage on long contexts</p>
    </header>

    <GCard class="ts-card">
      <div class="ts-row">
        <div>
          <p class="ts-label">Enable Token Saver</p>
          <p class="ts-desc">Compresses system prompts and long conversations before sending upstream</p>
        </div>
        <GSwitch v-model="enabled" @update:model-value="save" />
      </div>

      <div class="ts-row" :class="{ disabled: !enabled }">
        <div>
          <p class="ts-label">Compression Level</p>
          <p class="ts-desc">
            <strong>caveman</strong> — aggressive stripping (max savings) ·
            <strong>ponytail</strong> — balanced (keeps structure)
          </p>
        </div>
        <div class="level-btns">
          <button class="level-btn" :class="{ active: level === 'caveman' }" @click="level = 'caveman'; save()">caveman</button>
          <button class="level-btn" :class="{ active: level === 'ponytail' }" @click="level = 'ponytail'; save()">ponytail</button>
        </div>
      </div>

      <div class="ts-row" :class="{ disabled: !enabled }">
        <div>
          <p class="ts-label">Excluded Providers</p>
          <p class="ts-desc">Comma-separated provider IDs to skip compression for</p>
        </div>
      </div>
      <input
        class="field" :disabled="!enabled"
        :value="store.settings.tokenSaver?.exclude?.join(', ') || ''"
        placeholder="e.g. anthropic, google"
        @change="store.saveSettings({ tokenSaver: { ...store.settings.tokenSaver, enabled, level, exclude: ($event.target as HTMLInputElement).value.split(',').map(s => s.trim()).filter(Boolean) } })"
      >
    </GCard>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.ts-card { max-width: 560px; }
.ts-row {
  display: flex; align-items: center; justify-content: space-between; gap: 16px;
  padding: 14px 0; border-bottom: 1px solid var(--glass-border);
}
.ts-row:last-of-type { border-bottom: none; }
.ts-row.disabled { opacity: 0.45; pointer-events: none; }
.ts-label { font-size: 13px; font-weight: 600; }
.ts-desc { font-size: 11.5px; color: var(--text-faint); margin-top: 2px; }
.level-btns { display: flex; gap: 4px; }
.level-btn {
  padding: 5px 12px; border-radius: var(--radius-sm);
  border: 1px solid var(--glass-border); background: transparent;
  color: var(--text-muted); font-size: 11.5px; font-weight: 560;
  font-family: var(--font-mono); cursor: pointer; transition: all 0.15s ease;
}
.level-btn.active { background: var(--gradient-soft); color: var(--accent); border-color: rgba(45,212,191,0.25); }
.field {
  width: 100%; height: 34px; padding: 0 12px; margin-top: 4px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 12.5px; outline: none; transition: all 0.15s ease;
}
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.field:disabled { opacity: 0.45; }
</style>
