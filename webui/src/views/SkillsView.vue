<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import GCard from '@/components/ui/GCard.vue'
import GBadge from '@/components/ui/GBadge.vue'
import { Copy, Check, BookOpen, ChevronDown, ChevronRight } from 'lucide-vue-next'

const toast = useToast()

interface Skill {
  id: string
  name: string
  description: string
  content: string
}

const skills = ref<Skill[]>([])
const loading = ref(true)
const expanded = ref<Record<string, boolean>>({})
const copied = ref('')

async function load() {
  loading.value = true
  try {
    const res = await api('/api/skills')
    skills.value = res.skills || []
  } catch {
    toast.error('Failed to load skills')
  }
  loading.value = false
}

function toggle(id: string) {
  expanded.value[id] = !expanded.value[id]
}

function copyContent(skill: Skill) {
  navigator.clipboard.writeText(skill.content)
  copied.value = skill.id
  toast.success(`Skill "${skill.name}" copied to clipboard`)
  setTimeout(() => { copied.value = '' }, 2000)
}

function categoryLabel(id: string): string {
  const map: Record<string, string> = {
    'cyrene': 'Entry',
    'cyrene-chat': 'Chat',
    'cyrene-image': 'Image',
    'cyrene-tts': 'TTS',
    'cyrene-stt': 'STT',
    'cyrene-embeddings': 'Embeddings',
    'cyrene-web-search': 'Web Search',
    'cyrene-web-fetch': 'Web Fetch',
  }
  return map[id] || id
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-header">
      <h1 class="page-title">Skills</h1>
      <p class="page-sub">Drop-in skill definitions for AI agents. Copy a skill and paste it to your AI to enable Cyrene Gateway capabilities.</p>
    </div>

    <div class="usage-note">
      <BookOpen :size="14" />
      <span>Paste to your AI: <code>"Read this skill and use it: [paste content]"</code> — the agent will configure itself to use your gateway.</span>
    </div>

    <div v-if="loading" class="loading-state">Loading skills…</div>

    <div v-else class="skills-list">
      <GCard v-for="skill in skills" :key="skill.id" class="skill-card">
        <div class="skill-header" @click="toggle(skill.id)">
          <component :is="expanded[skill.id] ? ChevronDown : ChevronRight" :size="14" class="expand-icon" />
          <div class="skill-meta">
            <span class="skill-name">{{ skill.name }}</span>
            <GBadge variant="blue" class="skill-cat">{{ categoryLabel(skill.id) }}</GBadge>
          </div>
          <button class="copy-btn" @click.stop="copyContent(skill)">
            <Check v-if="copied === skill.id" :size="13" />
            <Copy v-else :size="13" />
            <span>{{ copied === skill.id ? 'Copied' : 'Copy' }}</span>
          </button>
        </div>
        <p class="skill-desc">{{ skill.description }}</p>
        <div v-if="expanded[skill.id]" class="skill-content">
          <pre>{{ skill.content }}</pre>
        </div>
      </GCard>
    </div>
  </div>
</template>

<style scoped>
.page { max-width: 800px; }
.page-header { margin-bottom: 20px; }
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-sub { font-size: 13px; color: var(--text-muted); margin-top: 4px; line-height: 1.5; }

.usage-note {
  display: flex; align-items: center; gap: 10px;
  background: rgba(96,165,250,0.06); border: 1px solid rgba(96,165,250,0.2);
  border-radius: var(--radius-sm); padding: 10px 14px;
  font-size: 12.5px; color: var(--text-muted); margin-bottom: 20px;
}
.usage-note code { font-family: var(--font-mono); font-size: 11.5px; color: var(--accent); }

.loading-state { font-size: 13px; color: var(--text-faint); padding: 20px 0; }

.skills-list { display: flex; flex-direction: column; gap: 8px; }
.skill-card { padding: 14px 16px; }
.skill-header {
  display: flex; align-items: center; gap: 10px; cursor: pointer;
}
.expand-icon { color: var(--text-faint); flex-shrink: 0; }
.skill-meta { display: flex; align-items: center; gap: 8px; flex: 1; }
.skill-name { font-size: 13.5px; font-weight: 600; }
.skill-cat { flex-shrink: 0; }
.copy-btn {
  display: flex; align-items: center; gap: 5px;
  padding: 5px 10px; border-radius: var(--radius-sm);
  background: var(--glass-hover); border: 1px solid var(--glass-border);
  color: var(--text-muted); cursor: pointer; font-size: 11.5px;
  transition: all 0.15s ease;
}
.copy-btn:hover { color: var(--text); border-color: var(--glass-border-hover); }

.skill-desc { font-size: 12px; color: var(--text-muted); margin-top: 8px; line-height: 1.5; padding-left: 24px; }
.skill-content {
  margin-top: 12px; padding: 12px 14px; padding-left: 24px;
  background: var(--code-bg); border-radius: var(--radius-sm);
  overflow-x: auto; max-height: 400px; overflow-y: auto;
}
.skill-content pre {
  font-size: 11.5px; font-family: var(--font-mono);
  color: var(--text-muted); white-space: pre-wrap; word-break: break-word;
  margin: 0; line-height: 1.6;
}
</style>
