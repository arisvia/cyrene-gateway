<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GSwitch from '@/components/ui/GSwitch.vue'

const store = useGatewayStore()
const toast = useToast()

const loaded = ref(false)
const newPass = ref('')
const confirmPass = ref('')
const savingPass = ref(false)
const loopGuard = ref(true)
const maxRetries = ref('3')

onMounted(async () => {
  await store.loadSettings()
  loaded.value = true
  loopGuard.value = store.settings.loopGuard?.enabled !== false
  maxRetries.value = String(store.settings.maxRetries || 3)
})

async function saveGeneral() {
  await store.saveSettings({
    loopGuard: { enabled: loopGuard.value },
    maxRetries: parseInt(maxRetries.value) || 3,
  })
}

async function changePassword() {
  if (!newPass.value) { toast.error('Password cannot be empty'); return }
  if (newPass.value !== confirmPass.value) { toast.error('Passwords do not match'); return }
  savingPass.value = true
  try {
    await store.setPassword(newPass.value)
    toast.success('Dashboard password updated')
    newPass.value = ''; confirmPass.value = ''
  } catch (e: any) { toast.error(e.message) }
  savingPass.value = false
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">Settings</h1>
      <p class="page-desc">Gateway behavior, security, and system information</p>
    </header>

    <!-- General -->
    <GCard class="settings-card">
      <p class="card-title">Routing Behavior</p>
      <div class="s-row">
        <div>
          <p class="row-label">Loop Guard</p>
          <p class="row-desc">Detect and break infinite retry loops between providers</p>
        </div>
        <GSwitch v-model="loopGuard" @update:model-value="saveGeneral" />
      </div>
      <div class="s-row">
        <div>
          <p class="row-label">Max Fallback Retries</p>
          <p class="row-desc">Maximum upstream attempts per request across all providers</p>
        </div>
        <input v-model="maxRetries" class="field small" type="number" min="1" max="10" @change="saveGeneral">
      </div>
    </GCard>

    <!-- Security -->
    <GCard class="settings-card">
      <p class="card-title">Security</p>
      <label class="field-label">New Dashboard Password</label>
      <input v-model="newPass" class="field" type="password" placeholder="Leave empty to keep current">
      <label class="field-label">Confirm Password</label>
      <input v-model="confirmPass" class="field" type="password" placeholder="Repeat password" @keyup.enter="changePassword">
      <div class="s-actions">
        <GButton size="sm" :loading="savingPass" @click="changePassword">Update Password</GButton>
      </div>
    </GCard>

    <!-- About -->
    <GCard class="settings-card">
      <p class="card-title">About</p>
      <div class="about-grid">
        <div class="about-item"><span class="about-label">Version</span><span class="about-val">v{{ store.version }}</span></div>
        <div class="about-item"><span class="about-label">Status</span><span class="about-val">{{ store.health.status || '—' }}</span></div>
        <div class="about-item"><span class="about-label">Connections</span><span class="about-val">{{ store.providers.length }}</span></div>
        <div class="about-item"><span class="about-label">Uptime</span><span class="about-val">{{ store.health.uptime || '—' }}</span></div>
      </div>
    </GCard>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.settings-card { max-width: 520px; margin-bottom: 14px; }
.card-title { font-size: 13px; font-weight: 650; margin-bottom: 12px; }
.s-row {
  display: flex; align-items: center; justify-content: space-between; gap: 16px;
  padding: 12px 0; border-bottom: 1px solid var(--glass-border);
}
.s-row:last-child { border-bottom: none; }
.row-label { font-size: 12.5px; font-weight: 600; }
.row-desc { font-size: 11px; color: var(--text-faint); margin-top: 2px; }
.field-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin: 12px 0 5px; }
.field {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; outline: none; transition: all 0.15s ease;
}
.field.small { width: 70px; text-align: center; }
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.s-actions { margin-top: 14px; }
.about-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.about-item { display: flex; flex-direction: column; gap: 2px; }
.about-label { font-size: 10.5px; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); font-weight: 600; }
.about-val { font-size: 12.5px; font-family: var(--font-mono); }
</style>
