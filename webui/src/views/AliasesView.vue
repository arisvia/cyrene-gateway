<script setup lang="ts">
import { ref } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import GButton from '@/components/ui/GButton.vue'
import GCard from '@/components/ui/GCard.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { ArrowRight, Trash2 } from 'lucide-vue-next'

const store = useGatewayStore()
const newAlias = ref({ alias: '', target: '' })

async function add() {
  if (!newAlias.value.alias || !newAlias.value.target) return
  await store.addAlias(newAlias.value.alias, newAlias.value.target)
  newAlias.value = { alias: '', target: '' }
}
</script>

<template>
  <div>
    <div class="page-header">
      <h1 class="page-title">Model Aliases</h1>
      <p class="page-desc">Map short names to full model identifiers.</p>
    </div>

    <GCard pad class="section-gap">
      <div class="alias-form">
        <input v-model="newAlias.alias" class="input" placeholder="Alias (e.g. fast)" style="flex:1">
        <ArrowRight :size="15" style="color:var(--text-faint);flex-shrink:0" />
        <input v-model="newAlias.target" class="input mono" placeholder="Target (e.g. groq/llama-3.3-70b)" style="flex:1.4">
        <GButton @click="add">Add</GButton>
      </div>
    </GCard>

    <GCard>
      <div v-for="(target, alias) in store.aliases" :key="alias" class="list-row">
        <div class="flex-gap mono" style="font-size:12.5px">
          <span>{{ alias }}</span>
          <ArrowRight :size="13" style="color:var(--text-faint)" />
          <span class="text-muted">{{ target }}</span>
        </div>
        <GButton variant="danger-ghost" size="icon" @click="store.deleteAlias(alias as string)"><Trash2 :size="14" /></GButton>
      </div>
      <GEmpty v-if="Object.keys(store.aliases).length === 0">No aliases configured.</GEmpty>
    </GCard>
  </div>
</template>

<style scoped>
.alias-form { display: flex; gap: 10px; align-items: center; }
.list-row {
  display: flex; align-items: center; justify-content: space-between;
  padding: 14px 16px; gap: 12px; border-bottom: 1px solid var(--row-divider);
  transition: background 0.12s ease;
}
.list-row:last-child { border-bottom: none; }
.list-row:hover { background: var(--glass); }
.input {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font); outline: none; transition: all 0.15s ease;
}
.input.mono { font-family: var(--font-mono); font-size: 12px; }
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
</style>
