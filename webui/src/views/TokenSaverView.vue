<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import GButton from '@/components/ui/GButton.vue'
import GCard from '@/components/ui/GCard.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import { Check } from 'lucide-vue-next'

const store = useGatewayStore()
const saved = ref(false)

onMounted(() => store.loadSettings())

async function save() {
  await store.saveSettings()
  saved.value = true
  setTimeout(() => { saved.value = false }, 2500)
}
</script>

<template>
  <div style="max-width:600px;margin:0 auto">
    <div class="page-header">
      <h1 class="page-title">Token Saver</h1>
      <p class="page-desc">RTK compression, caveman, and ponytail token optimization.</p>
    </div>

    <GCard pad class="section-gap">
      <p class="card-section-title">RTK Compression</p>
      <div class="settings-row">
        <div><p class="settings-title">Enable RTK</p><p class="settings-desc">Compress large tool_result outputs (smart truncation)</p></div>
        <GSwitch v-model="store.settings.rtkEnabled" />
      </div>
    </GCard>

    <GCard pad class="section-gap">
      <p class="card-section-title">Caveman (Terse Output)</p>
      <div class="settings-row">
        <div><p class="settings-title">Enable Caveman</p><p class="settings-desc">Inject terse-style system prompt to reduce output tokens</p></div>
        <GSwitch v-model="store.settings.cavemanEnabled" />
      </div>
      <div class="settings-row" v-if="store.settings.cavemanEnabled" style="padding-bottom:0">
        <div><p class="settings-title">Level</p></div>
        <select v-model="store.settings.cavemanLevel" class="input sel">
          <option value="lite">lite</option>
          <option value="full">full</option>
          <option value="ultra">ultra</option>
          <option value="wenyan-lite">wenyan-lite</option>
          <option value="wenyan">wenyan</option>
          <option value="wenyan-ultra">wenyan-ultra</option>
        </select>
      </div>
    </GCard>

    <GCard pad class="section-gap">
      <p class="card-section-title">Ponytail (Minimal Code)</p>
      <div class="settings-row">
        <div><p class="settings-title">Enable Ponytail</p><p class="settings-desc">Inject lazy-senior-dev prompt to bias toward minimal code</p></div>
        <GSwitch v-model="store.settings.ponytailEnabled" />
      </div>
      <div class="settings-row" v-if="store.settings.ponytailEnabled" style="padding-bottom:0">
        <div><p class="settings-title">Level</p></div>
        <select v-model="store.settings.ponytailLevel" class="input sel">
          <option value="lite">lite</option>
          <option value="full">full</option>
          <option value="ultra">ultra</option>
        </select>
      </div>
    </GCard>

    <div class="flex-gap" style="justify-content:center">
      <GButton @click="save">Save Token Saver</GButton>
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
.input.sel {
  width: 140px; height: 29px; font-size: 12px; padding: 0 30px 0 10px;
  background-color: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-family: var(--font); outline: none; cursor: pointer;
  appearance: none; background-image: var(--select-arrow);
  background-repeat: no-repeat; background-position: right 10px center; background-size: 14px;
}
.input.sel option { background-color: var(--bg-elevated); color: var(--text); }
</style>
