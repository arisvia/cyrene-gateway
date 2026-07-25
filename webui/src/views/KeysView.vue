<script setup lang="ts">
import { ref } from 'vue'
import { useGatewayStore, type ApiKey } from '@/stores/gateway'
import { maskKey, copyText, formatDate } from '@/lib/format'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, KeyRound, Trash2, Copy, X, CheckCircle2 } from 'lucide-vue-next'

const store = useGatewayStore()
const showAdd = ref(false)
const newKeyName = ref('')
const lastCreated = ref<any>(null)

async function createKey() {
  lastCreated.value = await store.createKey(newKeyName.value)
  newKeyName.value = ''
  showAdd.value = false
}

async function removeKey(k: ApiKey) {
  if (!confirm(`Delete key "${k.name || k.id}"?`)) return
  await store.deleteKey(k)
}
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">API Keys</h1>
      <p class="page-desc">Generate and manage Bearer tokens for gateway access.</p>
    </div>

    <div class="flex-between section-gap">
      <GBadge>{{ store.apiKeys.length }} keys</GBadge>
      <GButton size="sm" @click="showAdd = true"><Plus :size="13" />Generate Key</GButton>
    </div>

    <div v-if="lastCreated" class="key-highlight section-gap">
      <div class="flex-gap" style="margin-bottom:10px">
        <CheckCircle2 :size="15" style="color:var(--green)" />
        <span style="font-size:13px;font-weight:550;color:var(--green)">Key generated — copy it now</span>
      </div>
      <div class="flex-gap">
        <code class="key-code">{{ lastCreated.key }}</code>
        <GButton variant="ghost" size="sm" @click="copyText(lastCreated.key)"><Copy :size="12" />Copy</GButton>
        <GButton variant="danger-ghost" size="icon" @click="lastCreated = null"><X :size="13" /></GButton>
      </div>
    </div>

    <GCard>
      <div v-for="k in store.apiKeys" :key="k.id" class="list-row">
        <div class="flex-gap min-w-0">
          <div class="row-icon"><KeyRound :size="15" /></div>
          <div class="min-w-0">
            <div class="flex-gap">
              <span style="font-size:13.5px;font-weight:550">{{ k.name || 'Unnamed' }}</span>
              <GBadge :color="k.isActive ? 'green' : 'red'">{{ k.isActive ? 'active' : 'disabled' }}</GBadge>
            </div>
            <div class="flex-gap mt-sm">
              <code class="text-xs text-faint mono">{{ maskKey(k.key) }}</code>
              <button class="copy-mini" @click="copyText(k.key)" title="Copy full key"><Copy :size="11" /></button>
            </div>
          </div>
        </div>
        <div class="flex-gap shrink-0">
          <span class="text-xs text-faint">{{ formatDate(k.createdAt) }}</span>
          <GButton variant="danger-ghost" size="icon" @click="removeKey(k)"><Trash2 :size="14" /></GButton>
        </div>
      </div>
      <GEmpty v-if="store.apiKeys.length === 0">No API keys. Generate one to authenticate <code class="mono text-xs">Authorization: Bearer &lt;key&gt;</code> requests.</GEmpty>
    </GCard>

    <GModal v-if="showAdd" title="Generate API Key" desc="The key will be shown once after creation." width="380px" @close="showAdd = false">
      <div class="form-group">
        <label class="form-label">Key Name</label>
        <input v-model="newKeyName" class="input" placeholder="e.g. my-laptop, production" @keyup.enter="createKey">
      </div>
      <div class="modal-actions">
        <GButton variant="ghost" @click="showAdd = false">Cancel</GButton>
        <GButton @click="createKey">Generate</GButton>
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
.key-highlight {
  border: 1px solid rgba(52,211,153,0.3); background: rgba(52,211,153,0.04);
  padding: 14px 16px; border-radius: var(--radius);
}
.key-code {
  flex: 1; padding: 8px 12px; border-radius: var(--radius-sm);
  background: var(--code-bg); border: 1px solid var(--glass-border);
  font-family: var(--font-mono); font-size: 11px; color: var(--green); word-break: break-all;
}
.copy-mini {
  background: none; border: none; color: var(--text-faint); cursor: pointer;
  display: flex; padding: 2px;
}
.copy-mini:hover { color: var(--text); }
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.input {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font); outline: none; transition: all 0.15s ease;
}
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
