<script setup lang="ts">
import { ref } from 'vue'
import { useGatewayStore, type ProxyPool } from '@/stores/gateway'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, Globe, Trash2 } from 'lucide-vue-next'

const store = useGatewayStore()
const showAdd = ref(false)
const newProxy = ref({ name: '', proxyUrl: '', noProxy: '', strictProxy: false, type: 'http' })

async function add() {
  await store.addProxy(newProxy.value)
  newProxy.value = { name: '', proxyUrl: '', noProxy: '', strictProxy: false, type: 'http' }
  showAdd.value = false
}

async function remove(pp: ProxyPool) {
  if (!confirm(`Delete proxy "${pp.data.name}"?`)) return
  await store.deleteProxy(pp)
}
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Proxy Pools</h1>
      <p class="page-desc">Outbound proxy rotation for upstream requests.</p>
    </div>

    <div class="flex-between section-gap">
      <GBadge>{{ store.proxyPools.length }} pools</GBadge>
      <GButton size="sm" @click="showAdd = true"><Plus :size="13" />Add Proxy</GButton>
    </div>

    <GCard>
      <div v-for="pp in store.proxyPools" :key="pp.id" class="list-row">
        <div class="flex-gap min-w-0">
          <div class="row-icon"><Globe :size="15" /></div>
          <div class="min-w-0">
            <div class="flex-gap">
              <span style="font-size:13.5px;font-weight:550">{{ pp.data.name }}</span>
              <GBadge>{{ pp.data.type }}</GBadge>
              <GBadge v-if="pp.data.strictProxy" color="amber">strict</GBadge>
            </div>
            <p class="text-xs text-faint mt-sm mono truncate">{{ pp.data.proxyUrl }}</p>
          </div>
        </div>
        <div class="flex-gap shrink-0">
          <GSwitch :model-value="pp.isActive" @update:model-value="store.toggleProxy(pp)" />
          <GButton variant="danger-ghost" size="icon" @click="remove(pp)"><Trash2 :size="14" /></GButton>
        </div>
      </div>
      <GEmpty v-if="store.proxyPools.length === 0">No proxy pools. Outbound requests go direct.</GEmpty>
    </GCard>

    <GModal v-if="showAdd" title="Add Proxy Pool" desc="Outbound proxy rotation for upstream requests." width="420px" @close="showAdd = false">
      <div class="form-grid-2">
        <div class="form-group">
          <label class="form-label">Name</label>
          <input v-model="newProxy.name" class="input" placeholder="e.g. home-proxy">
        </div>
        <div class="form-group">
          <label class="form-label">Type</label>
          <select v-model="newProxy.type" class="input">
            <option value="http">HTTP</option>
            <option value="vercel">Vercel</option>
            <option value="cloudflare">Cloudflare Worker</option>
            <option value="deno">Deno Deploy</option>
          </select>
        </div>
      </div>
      <div class="form-group">
        <label class="form-label">Proxy URL</label>
        <input v-model="newProxy.proxyUrl" class="input mono" placeholder="http://127.0.0.1:7890">
      </div>
      <div class="form-group">
        <label class="form-label">No-Proxy Hosts <span class="text-faint">(optional)</span></label>
        <input v-model="newProxy.noProxy" class="input mono" placeholder="localhost,127.0.0.1">
      </div>
      <div class="form-group" style="display:flex;align-items:center;gap:10px">
        <GSwitch v-model="newProxy.strictProxy" />
        <span style="font-size:13px">Strict mode <span class="text-faint text-xs">— fail if proxy unreachable</span></span>
      </div>
      <div class="modal-actions">
        <GButton variant="ghost" @click="showAdd = false">Cancel</GButton>
        <GButton @click="add">Add Pool</GButton>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.list-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px; gap: 12px; border-bottom: 1px solid var(--row-divider);
  transition: background 0.12s ease;
}
.list-row:last-child { border-bottom: none; }
.list-row:hover { background: var(--glass); }
.row-icon {
  width: 34px; height: 34px; border-radius: 8px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--glass-hover); border: 1px solid var(--glass-border); color: var(--text-muted);
}
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.form-grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.input {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font); outline: none; transition: all 0.15s ease;
}
.input.mono { font-family: var(--font-mono); font-size: 12px; }
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
select.input {
  appearance: none; background-image: var(--select-arrow);
  background-repeat: no-repeat; background-position: right 10px center;
  background-size: 14px; padding-right: 34px; cursor: pointer;
}
select.input option { background-color: var(--bg-elevated); color: var(--text); }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
