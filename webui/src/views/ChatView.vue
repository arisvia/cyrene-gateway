<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import GButton from '@/components/ui/GButton.vue'
import { Send, Trash2, Loader2 } from 'lucide-vue-next'

const toast = useToast()

interface ChatMsg {
  role: 'user' | 'assistant' | 'system'
  content: string
}

interface ModelMeta {
  id: string
  ownedBy: string
  contextLength?: number
  capabilities?: string[]
}

const models = ref<ModelMeta[]>([])
const selectedModel = ref('')
const messages = ref<ChatMsg[]>([])
const input = ref('')
const streaming = ref(false)
const usage = ref<{ promptTokens: number; completionTokens: number; totalTokens: number } | null>(null)
const streamTokens = ref(0)
const chatRef = ref<HTMLElement>()

// Group models by provider for the selector
const groupedModels = computed(() => {
  const groups: Record<string, ModelMeta[]> = {}
  for (const m of models.value) {
    const provider = m.ownedBy || 'other'
    if (!groups[provider]) groups[provider] = []
    groups[provider].push(m)
  }
  return Object.entries(groups).sort(([a], [b]) => a.localeCompare(b))
})

async function loadModels() {
  try {
    const res = await api('/v1/models')
    models.value = (res.data || []).map((m: any) => ({
      id: m.id,
      ownedBy: m.owned_by || 'unknown',
      contextLength: m.context_length,
      capabilities: m.capabilities,
    })).sort((a: ModelMeta, b: ModelMeta) => a.id.localeCompare(b.id))
    if (models.value.length && !selectedModel.value) {
      selectedModel.value = models.value[0].id
    }
  } catch {
    toast.error('Failed to load models')
  }
}

async function send() {
  const text = input.value.trim()
  if (!text || streaming.value) return
  if (!selectedModel.value) { toast.error('Select a model first'); return }

  messages.value.push({ role: 'user', content: text })
  input.value = ''
  streaming.value = true
  usage.value = null
  streamTokens.value = 0

  // Add placeholder for assistant
  messages.value.push({ role: 'assistant', content: '' })
  const idx = messages.value.length - 1

  await nextTick()
  scrollBottom()

  try {
    const res = await fetch('/v1/chat/completions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        model: selectedModel.value,
        messages: messages.value.slice(0, -1).map(m => ({ role: m.role, content: m.content })),
        stream: true,
      }),
    })

    if (!res.ok) {
      const err = await res.json().catch(() => ({ error: 'Request failed' }))
      const hint = err.error?.hint || err.hint
      messages.value[idx].content = `Error: ${typeof err.error === 'string' ? err.error : err.error?.message || res.statusText}${hint ? '\n\n💡 ' + hint : ''}`
      streaming.value = false
      return
    }

    const reader = res.body?.getReader()
    if (!reader) { streaming.value = false; return }

    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })

      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        const data = line.slice(6).trim()
        if (data === '[DONE]') continue
        try {
          const parsed = JSON.parse(data)
          const delta = parsed.choices?.[0]?.delta?.content
          if (delta) {
            messages.value[idx].content += delta
            // Estimate streaming tokens (~4 chars per token)
            streamTokens.value = Math.ceil(messages.value[idx].content.length / 4)
            scrollBottom()
          }
          if (parsed.usage) {
            usage.value = {
              promptTokens: parsed.usage.prompt_tokens || 0,
              completionTokens: parsed.usage.completion_tokens || 0,
              totalTokens: parsed.usage.total_tokens || 0,
            }
          }
        } catch { /* skip malformed chunks */ }
      }
    }
  } catch (e: any) {
    messages.value[idx].content = `Error: ${e.message || 'Connection failed'}`
  }
  streaming.value = false
}

function reset() {
  messages.value = []
  usage.value = null
  streamTokens.value = 0
}

function scrollBottom() {
  nextTick(() => {
    if (chatRef.value) chatRef.value.scrollTop = chatRef.value.scrollHeight
  })
}

const tokenSummary = computed(() => {
  if (usage.value) {
    return `${usage.value.promptTokens} prompt + ${usage.value.completionTokens} completion = ${usage.value.totalTokens} tokens`
  }
  if (streaming.value && streamTokens.value > 0) {
    return `~${streamTokens.value} tokens (streaming…)`
  }
  return ''
})

onMounted(loadModels)
</script>

<template>
  <div class="page chat-page">
    <div class="chat-header">
      <div>
        <h1 class="page-title">Chat Playground</h1>
        <p class="page-sub">Test models through your gateway</p>
      </div>
      <div class="chat-controls">
        <select v-model="selectedModel" class="model-select" aria-label="Select model">
          <optgroup v-for="[provider, providerModels] in groupedModels" :key="provider" :label="provider">
            <option v-for="m in providerModels" :key="m.id" :value="m.id">{{ m.id }}</option>
          </optgroup>
        </select>
        <GButton variant="ghost" size="sm" @click="reset" :disabled="streaming">
          <Trash2 :size="13" /> Clear
        </GButton>
      </div>
    </div>

    <div class="chat-body" ref="chatRef">
      <div v-if="messages.length === 0" class="chat-empty">
        <p>Select a model and start chatting.</p>
        <p class="chat-empty-sub">Messages are sent through <code>/v1/chat/completions</code> with streaming.</p>
      </div>
      <div v-for="(msg, i) in messages" :key="i" :class="['msg', msg.role]">
        <div class="msg-role">{{ msg.role === 'user' ? 'You' : 'Assistant' }}</div>
        <div class="msg-content">{{ msg.content || (streaming && i === messages.length - 1 ? '…' : '') }}</div>
      </div>
    </div>

    <div class="chat-footer">
      <div v-if="tokenSummary" class="token-info">{{ tokenSummary }}</div>
      <div class="input-row">
        <textarea
          v-model="input"
          class="chat-input"
          placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
          rows="1"
          @keydown.enter.exact.prevent="send"
          :disabled="streaming"
        />
        <button class="send-btn" @click="send" :disabled="streaming || !input.trim()">
          <Loader2 v-if="streaming" :size="16" class="spin" />
          <Send v-else :size="16" />
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-page {
  display: flex; flex-direction: column;
  height: calc(100vh - 48px); max-width: 860px;
}
.chat-header {
  display: flex; align-items: flex-start; justify-content: space-between;
  gap: 16px; margin-bottom: 16px; flex-shrink: 0;
}
.page-title { font-size: 20px; font-weight: 700; letter-spacing: -0.03em; }
.page-sub { font-size: 13px; color: var(--text-muted); margin-top: 4px; }
.chat-controls { display: flex; align-items: center; gap: 8px; }
.model-select {
  height: 32px; padding: 0 10px; max-width: 240px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 12px; font-family: var(--font-mono); outline: none;
}
.model-select:focus { border-color: var(--ring); }

.chat-body {
  flex: 1; overflow-y: auto; padding: 16px 0;
  display: flex; flex-direction: column; gap: 12px;
}
.chat-empty { text-align: center; color: var(--text-faint); font-size: 13px; margin: auto; }
.chat-empty-sub { font-size: 11.5px; margin-top: 6px; }
.chat-empty code { font-family: var(--font-mono); font-size: 11px; }

.msg { max-width: 85%; }
.msg.user { align-self: flex-end; }
.msg.assistant { align-self: flex-start; }
.msg-role { font-size: 10.5px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-faint); margin-bottom: 4px; }
.msg.user .msg-role { text-align: right; }
.msg-content {
  padding: 10px 14px; border-radius: 12px;
  font-size: 13.5px; line-height: 1.6; white-space: pre-wrap; word-break: break-word;
}
.msg.user .msg-content {
  background: var(--accent); color: var(--on-accent);
  border-bottom-right-radius: 4px;
}
.msg.assistant .msg-content {
  background: var(--glass); border: 1px solid var(--glass-border);
  border-bottom-left-radius: 4px;
}

.chat-footer { flex-shrink: 0; padding-top: 12px; border-top: 1px solid var(--glass-border); }
.token-info { font-size: 11px; color: var(--text-faint); font-family: var(--font-mono); margin-bottom: 8px; }
.input-row { display: flex; gap: 8px; align-items: flex-end; }
.chat-input {
  flex: 1; min-height: 38px; max-height: 120px; padding: 9px 14px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13.5px; font-family: var(--font); line-height: 1.5;
  resize: none; outline: none; transition: border-color 0.15s ease;
}
.chat-input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.chat-input::placeholder { color: var(--text-faint); }
.send-btn {
  width: 38px; height: 38px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  background: var(--gradient); color: var(--on-accent);
  border: none; cursor: pointer; flex-shrink: 0;
  transition: all 0.15s ease; box-shadow: var(--shadow-accent);
}
.send-btn:hover:not(:disabled) { filter: brightness(1.1); }
.send-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
