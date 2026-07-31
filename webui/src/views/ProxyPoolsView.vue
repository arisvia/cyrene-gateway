<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GModal from '@/components/ui/GModal.vue'
import GSwitch from '@/components/ui/GSwitch.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, Trash2, Globe } from 'lucide-vue-next'

const store = useGatewayStore()
const toast = useToast()

const showForm = ref(false)
const form = ref({ id: '', name: '', type: 'http', proxyUrl: '', noProxy: '' })
const saving = ref(false)

onMounted(() => { if (!store.proxyPools.length) store.loadProxyPools() })

function openCreate() {
  form.value = { id: '', name: '', type: 'http', proxyUrl: '', noProxy: '' }
  showForm.value = true
}

async function save() {
  if (!form.value.name || !form.value.proxyUrl || saving.value) return
  saving.value = true
  try {
    await store.saveProxyPool({
      id: form.value.id || undefined,
      name: form.value.name, type: form.value.type,
      proxyUrl: form.value.proxyUrl, noProxy: form.value.noProxy || undefined,
    })
    showForm.value = false
  } catch (e: any) { toast.error(e.message) }
  saving.value = false
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">Proxy Pools</h1>
        <p class="page-desc">Outbound proxy rotation for provider connections</p>
      </div>
      <GButton size="sm" @click="openCreate"><Plus :size="13" /> Add Pool</GButton>
    </header>

    <GEmpty v-if="!store.proxyPools.length" title="No proxy pools" desc="Add a proxy pool to route provider traffic through rotating proxies." />
    <div v-else class="pool-list stagger">
      <GCard v-for="pp in store.proxyPools" :key="pp.id" class="pool-card">
        <div class="pool-left">
          <Globe :size="15" class="pool-icon" />
          <div>
            <p class="pool-name">{{ pp.data.name }}</p>
            <p class="pool-url">{{ pp.data.type }} · {{ pp.data.proxyUrl }}</p>
          </div>
        </div>
        <div class="pool-right">
          <GSwitch :model-value="pp.isActive" @update:model-value="store.toggleProxyPool(pp)" />
          <button class="icon-btn danger" @click="store.deleteProxyPool(pp.id)" title="Delete pool">
            <Trash2 :size="13" />
          </button>
        </div>
      </GCard>
    </div>

    <GModal v-if="showForm" title="Add Proxy Pool" @close="showForm = false">
      <label class="field-label">Name</label>
      <input v-model="form.name" class="field" placeholder="residential-pool">
      <label class="field-label">Type</label>
      <select v-model="form.type" class="field select">
        <option value="http">http</option>
        <option value="socks5">socks5</option>
      </select>
      <label class="field-label">Proxy URL</label>
      <input v-model="form.proxyUrl" class="field" placeholder="http://user:pass@host:port">
      <label class="field-label">No-Proxy (optional)</label>
      <input v-model="form.noProxy" class="field" placeholder="localhost,127.0.0.1">
      <div class="modal-actions">
        <GButton variant="ghost" @click="showForm = false">Cancel</GButton>
        <GButton :loading="saving" @click="save">Save</GButton>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 22px; flex-wrap: wrap; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.pool-list { display: flex; flex-direction: column; gap: 8px; max-width: 640px; }
.pool-card { display: flex; align-items: center; justify-content: space-between; padding: 13px 16px; }
.pool-left { display: flex; align-items: center; gap: 11px; min-width: 0; }
.pool-icon { color: var(--accent); flex-shrink: 0; }
.pool-name { font-size: 13px; font-weight: 600; }
.pool-url { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 340px; }
.pool-right { display: flex; align-items: center; gap: 10px; }
.icon-btn {
  width: 26px; height: 26px; border-radius: var(--radius-xs);
  display: flex; align-items: center; justify-content: center;
  background: transparent; border: 1px solid transparent;
  color: var(--text-muted); cursor: pointer; transition: all 0.12s ease;
}
.icon-btn.danger:hover { color: var(--red); background: rgba(248,113,113,0.08); }
.field-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin: 12px 0 5px; }
.field {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; outline: none; transition: all 0.15s ease;
}
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.select { appearance: auto; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
</style>
