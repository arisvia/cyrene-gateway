<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, Trash2, Combine } from 'lucide-vue-next'

const store = useGatewayStore()
const toast = useToast()

const showCreate = ref(false)
const name = ref('')
const kind = ref('fallback')
const models = ref('')
const saving = ref(false)

onMounted(() => { if (!store.combos.length && !store.registryCategories.length) store.loadCore() })

async function create() {
  if (!name.value || saving.value) return
  saving.value = true
  try {
    await store.saveCombo({ name: name.value, kind: kind.value, models: models.value.split(',').map(s => s.trim()).filter(Boolean) })
    showCreate.value = false
    name.value = ''; models.value = ''
  } catch (e: any) { toast.error(e.message) }
  saving.value = false
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">Combos</h1>
        <p class="page-desc">Model routing combos — fallback chains, round-robin pools, sticky groups</p>
      </div>
      <GButton size="sm" @click="showCreate = true"><Plus :size="13" /> New Combo</GButton>
    </header>

    <GEmpty v-if="!store.combos.length" title="No combos yet" desc="Combos let you route a single model name across multiple upstreams with fallback or round-robin strategies.">
      <GButton size="sm" style="margin-top:14px" @click="showCreate = true"><Plus :size="13" /> Create your first combo</GButton>
    </GEmpty>

    <div v-else class="combo-list stagger">
      <GCard v-for="c in store.combos" :key="c.id" class="combo-card">
        <div class="combo-left">
          <Combine :size="15" class="combo-icon" />
          <div>
            <p class="combo-name">{{ c.name }}</p>
            <p class="combo-models">{{ c.models.join(' → ') }}</p>
          </div>
        </div>
        <div class="combo-right">
          <GBadge :color="c.kind === 'fallback' ? 'teal' : c.kind === 'round-robin' ? 'violet' : 'blue'">{{ c.kind }}</GBadge>
          <button class="icon-btn danger" @click="store.deleteCombo(c.id)" title="Delete combo">
            <Trash2 :size="13" />
          </button>
        </div>
      </GCard>
    </div>

    <GModal v-if="showCreate" title="Create Combo" @close="showCreate = false">
      <label class="field-label">Name (model alias)</label>
      <input v-model="name" class="field" placeholder="my-smart-route">
      <label class="field-label">Strategy</label>
      <select v-model="kind" class="field select">
        <option value="fallback">fallback</option>
        <option value="round-robin">round-robin</option>
        <option value="sticky">sticky</option>
      </select>
      <label class="field-label">Models (comma-separated, in priority order)</label>
      <input v-model="models" class="field" placeholder="anthropic/claude-sonnet-4, openai/gpt-4o">
      <div class="modal-actions">
        <GButton variant="ghost" @click="showCreate = false">Cancel</GButton>
        <GButton :loading="saving" @click="create">Create</GButton>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 22px; flex-wrap: wrap; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.combo-list { display: flex; flex-direction: column; gap: 8px; max-width: 700px; }
.combo-card { display: flex; align-items: center; justify-content: space-between; padding: 14px 16px; }
.combo-left { display: flex; align-items: center; gap: 12px; min-width: 0; }
.combo-icon { color: var(--accent); flex-shrink: 0; }
.combo-name { font-size: 13px; font-weight: 600; }
.combo-models { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 400px; }
.combo-right { display: flex; align-items: center; gap: 8px; }
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
