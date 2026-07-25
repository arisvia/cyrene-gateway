<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import GButton from '@/components/ui/GButton.vue'
import GCard from '@/components/ui/GCard.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import { Check } from 'lucide-vue-next'

const store = useGatewayStore()
const saved = ref(false)
const newPassword = ref('')

onMounted(() => store.loadSettings())

async function save() {
  await store.saveSettings()
  saved.value = true
  setTimeout(() => { saved.value = false }, 2500)
}

async function setPassword() {
  if (!newPassword.value) return
  const res = await store.setPassword(newPassword.value)
  if (res.error) { alert(res.error); return }
  newPassword.value = ''
  saved.value = true
  setTimeout(() => { saved.value = false }, 2500)
}
</script>

<template>
  <div style="max-width:600px;margin:0 auto">
    <div class="page-header">
      <h1 class="page-title">Settings</h1>
      <p class="page-desc">Security, authentication, and routing configuration.</p>
    </div>

    <GCard pad class="section-gap">
      <p class="card-section-title">Security</p>
      <div class="settings-row">
        <div><p class="settings-title">Require dashboard login</p><p class="settings-desc">Protect management UI with password</p></div>
        <GSwitch v-model="store.settings.requireLogin" />
      </div>
      <div class="settings-row">
        <div><p class="settings-title">Require API key</p><p class="settings-desc">Enforce Bearer token on /v1/* endpoints</p></div>
        <GSwitch v-model="store.settings.requireApiKey" />
      </div>
      <div class="settings-row">
        <div><p class="settings-title">Change password</p><p class="settings-desc">Minimum 6 characters</p></div>
        <div class="flex-gap">
          <input v-model="newPassword" type="password" class="input pw" placeholder="New password">
          <GButton variant="ghost" size="sm" @click="setPassword">Set</GButton>
        </div>
      </div>
    </GCard>

    <GCard pad class="section-gap">
      <p class="card-section-title">Routing</p>
      <div class="settings-row" style="padding-bottom:0">
        <div><p class="settings-title">Default combo strategy</p><p class="settings-desc">Used when combo has no explicit kind</p></div>
        <select v-model="store.settings.comboStrategy" class="input sel">
          <option value="">fallback</option>
          <option value="fallback">fallback</option>
          <option value="round-robin">round-robin</option>
        </select>
      </div>
    </GCard>

    <div class="flex-gap" style="justify-content:center">
      <GButton @click="save">Save Settings</GButton>
      <span v-if="saved" class="saved-indicator"><Check :size="13" />Saved</span>
    </div>
  </div>
</template>

<style scoped>
.card-section-title { font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-faint); margin-bottom: 12px; }
.settings-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 14px 0; }
.settings-row + .settings-row { border-top: 1px solid var(--row-divider); }
.settings-title { font-size: 13px; font-weight: 550; }
.settings-desc { font-size: 11.5px; color: var(--text-faint); margin-top: 2px; }
.saved-indicator { font-size: 12px; color: var(--green); display: inline-flex; align-items: center; gap: 5px; animation: fadeIn 0.2s ease; }
.input.pw {
  width: 140px; height: 29px; font-size: 12px; padding: 0 10px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-family: var(--font); outline: none;
}
.input.pw::placeholder { color: var(--text-faint); }
.input.pw:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.input.sel {
  width: 130px; height: 29px; font-size: 12px; padding: 0 30px 0 10px;
  background-color: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-family: var(--font); outline: none; cursor: pointer;
  appearance: none; background-image: var(--select-arrow);
  background-repeat: no-repeat; background-position: right 10px center; background-size: 14px;
}
.input.sel option { background-color: var(--bg-elevated); color: var(--text); }
</style>
