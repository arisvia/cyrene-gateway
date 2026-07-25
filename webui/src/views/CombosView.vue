<script setup lang="ts">
import { ref } from 'vue'
import { useGatewayStore, type Combo } from '@/stores/gateway'
import GButton from '@/components/ui/GButton.vue'
import GBadge from '@/components/ui/GBadge.vue'
import GCard from '@/components/ui/GCard.vue'
import GModal from '@/components/ui/GModal.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Plus, Trash2 } from 'lucide-vue-next'

const store = useGatewayStore()
const showAdd = ref(false)
const newCombo = ref({ name: '', kind: 'fallback', modelsStr: '' })

async function add() {
  const models = newCombo.value.modelsStr.split(',').map(s => s.trim()).filter(Boolean)
  if (!newCombo.value.name || models.length === 0) return
  await store.addCombo(newCombo.value.name, newCombo.value.kind, models)
  newCombo.value = { name: '', kind: 'fallback', modelsStr: '' }
  showAdd.value = false
}

async function remove(c: Combo) {
  if (!confirm(`Delete combo "${c.name}"?`)) return
  await store.deleteCombo(c)
}
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Combos</h1>
      <p class="page-desc">Fallback and round-robin model groups for resilient routing.</p>
    </div>

    <div class="flex-between section-gap">
      <GBadge>{{ store.combos.length }} combos</GBadge>
      <GButton size="sm" @click="showAdd = true"><Plus :size="13" />Add Combo</GButton>
    </div>

    <GCard>
      <div v-for="c in store.combos" :key="c.id" class="combo-row">
        <div class="flex-between">
          <div class="flex-gap">
            <span style="font-size:13.5px;font-weight:550">{{ c.name }}</span>
            <GBadge :color="c.kind === 'round-robin' ? 'blue' : 'glass'">{{ c.kind }}</GBadge>
          </div>
          <GButton variant="danger-ghost" size="icon" @click="remove(c)"><Trash2 :size="14" /></GButton>
        </div>
        <div class="model-chips">
          <span v-for="m in c.models" :key="m" class="model-chip">{{ m }}</span>
        </div>
      </div>
      <GEmpty v-if="store.combos.length === 0">No combos. Create fallback groups for resilient model routing.</GEmpty>
    </GCard>

    <GModal v-if="showAdd" title="Create Combo" desc="Route a virtual model name across multiple upstreams." width="420px" @close="showAdd = false">
      <div class="form-group">
        <label class="form-label">Combo Name</label>
        <input v-model="newCombo.name" class="input" placeholder="e.g. reliable-chat">
      </div>
      <div class="form-group">
        <label class="form-label">Strategy</label>
        <select v-model="newCombo.kind" class="input">
          <option value="fallback">Fallback — try in order until success</option>
          <option value="round-robin">Round Robin — distribute evenly</option>
        </select>
      </div>
      <div class="form-group">
        <label class="form-label">Models (comma-separated, priority order)</label>
        <input v-model="newCombo.modelsStr" class="input mono" placeholder="openai/gpt-4o, anthropic/claude-sonnet-4-20250514">
      </div>
      <div class="modal-actions">
        <GButton variant="ghost" @click="showAdd = false">Cancel</GButton>
        <GButton @click="add">Create</GButton>
      </div>
    </GModal>
  </div>
</template>

<style scoped>
.combo-row { padding: 14px 16px; border-bottom: 1px solid var(--row-divider); }
.combo-row:last-child { border-bottom: none; }
.model-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
.model-chip {
  font-family: var(--font-mono); font-size: 10px; padding: 3px 8px;
  border-radius: 4px; background: var(--glass-hover);
  border: 1px solid var(--glass-border); color: var(--text-muted);
}
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
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
