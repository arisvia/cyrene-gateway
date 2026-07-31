<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/lib/api'
import GCard from '@/components/ui/GCard.vue'
import GSkeleton from '@/components/ui/GSkeleton.vue'
import GEmpty from '@/components/ui/GEmpty.vue'
import { Sparkles, ChevronDown } from 'lucide-vue-next'

const loading = ref(true)
const skills = ref<any[]>([])
const expanded = ref<string | null>(null)

onMounted(async () => {
  try {
    const r = await api('/api/skills')
    skills.value = Array.isArray(r?.skills) ? r.skills : (Array.isArray(r) ? r : [])
  } catch { skills.value = [] }
  loading.value = false
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">Skills</h1>
      <p class="page-desc">Embedded agent skill manifests — copy into your AI tool's skill directory</p>
    </header>

    <GSkeleton v-if="loading" height="200px" />
    <GEmpty v-else-if="!skills.length" title="No skills" desc="No skill manifests registered." />
    <div v-else class="skill-list stagger">
      <GCard v-for="s in skills" :key="s.id" class="skill-card" :class="{ open: expanded === s.id }">
        <button class="skill-head" @click="expanded = expanded === s.id ? null : s.id">
          <div class="skill-icon"><Sparkles :size="14" /></div>
          <div class="skill-info">
            <p class="skill-name">{{ s.name || s.id }}</p>
            <p class="skill-desc">{{ s.description || '' }}</p>
          </div>
          <ChevronDown :size="14" class="skill-chev" :class="{ rotated: expanded === s.id }" />
        </button>
        <pre v-if="expanded === s.id" class="skill-content">{{ s.content || JSON.stringify(s, null, 2) }}</pre>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.page-head { margin-bottom: 22px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-desc { font-size: 12.5px; color: var(--text-faint); margin-top: 3px; }
.skill-list { display: flex; flex-direction: column; gap: 8px; max-width: 700px; }
.skill-card { padding: 0; overflow: hidden; }
.skill-head {
  display: flex; align-items: center; gap: 12px; width: 100%;
  padding: 14px 16px; background: transparent; border: none;
  color: var(--text); cursor: pointer; text-align: left;
}
.skill-icon {
  width: 32px; height: 32px; border-radius: 9px; flex-shrink: 0;
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient-soft); border: 1px solid var(--glass-border); color: var(--accent);
}
.skill-info { flex: 1; min-width: 0; }
.skill-name { font-size: 13px; font-weight: 600; }
.skill-desc { font-size: 11px; color: var(--text-faint); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.skill-chev { color: var(--text-faint); transition: transform 0.2s var(--ease-spring); flex-shrink: 0; }
.skill-chev.rotated { transform: rotate(180deg); }
.skill-content {
  margin: 0; padding: 14px 16px; border-top: 1px solid var(--glass-border);
  background: var(--code-bg); font-size: 11px; line-height: 1.7;
  color: var(--text-muted); white-space: pre-wrap; word-break: break-all;
  max-height: 320px; overflow-y: auto;
  animation: fadeIn 0.15s ease;
}
</style>
