<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute } from 'vue-router'
import { useGatewayStore } from '@/stores/gateway'
import {
  Link2, Layers, Clapperboard, Combine, BarChart3, RefreshCw, Zap,
  TerminalSquare, ScrollText, Globe, Cable, ShieldHalf, Sparkles,
  Settings, Sun, Moon, Languages, LogOut, ChevronDown,
  Braces, ImageIcon, AudioLines, Mic, Video, SearchCode,
} from 'lucide-vue-next'

defineEmits<{ logout: [] }>()

const route = useRoute()
const store = useGatewayStore()

const mediaExpanded = ref(route.path.startsWith('/media'))
const mobileOpen = ref(false)

const theme = ref<'dark' | 'light'>(document.documentElement.classList.contains('light') ? 'light' : 'dark')
function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.classList.toggle('light', theme.value === 'light')
  localStorage.setItem('cyrene-theme', theme.value)
}

const lang = ref(localStorage.getItem('cyrene-lang') || 'en')
const langs = ['en', 'zh', 'ja', 'ko', 'ru', 'es', 'fr', 'de', 'pt-BR', 'tr']
function cycleLang() {
  const idx = langs.indexOf(lang.value)
  lang.value = langs[(idx + 1) % langs.length]
  localStorage.setItem('cyrene-lang', lang.value)
  document.documentElement.setAttribute('lang', lang.value)
}

const mediaKinds = [
  { kind: 'embedding', label: 'Embedding', icon: Braces },
  { kind: 'image', label: 'Text to Image', icon: ImageIcon },
  { kind: 'tts', label: 'TTS', icon: AudioLines },
  { kind: 'stt', label: 'STT', icon: Mic },
  { kind: 'video', label: 'Video', icon: Video },
  { kind: 'web-fetch', label: 'Web Fetch & Search', icon: SearchCode },
]

const navGateway = [
  { to: '/', label: 'Endpoint & Key', icon: Link2, exact: true },
  { to: '/providers', label: 'Providers', icon: Layers },
]

const navGatewayAfter = [
  { to: '/combos', label: 'Combos', icon: Combine },
  { to: '/usage', label: 'Usage', icon: BarChart3 },
  { to: '/quota', label: 'Quota Tracker', icon: RefreshCw },
  { to: '/token-saver', label: 'Token Saver', icon: Zap },
  { to: '/cli-tools', label: 'CLI Tools', icon: TerminalSquare },
  { to: '/console', label: 'Console Log', icon: ScrollText },
]

const navSystem = [
  { to: '/proxy-pools', label: 'Proxy Pools', icon: Globe },
  { to: '/tunnel', label: 'Tunnel', icon: Cable },
  { to: '/mitm', label: 'MITM Proxy', icon: ShieldHalf },
  { to: '/skills', label: 'Skills', icon: Sparkles },
  { to: '/settings', label: 'Settings', icon: Settings },
]

function isActive(item: { to: string; exact?: boolean }) {
  if (item.exact) return route.path === item.to
  return route.path === item.to || route.path.startsWith(item.to + '/')
}

const isMediaActive = computed(() => route.path.startsWith('/media'))

function closeMobile() { mobileOpen.value = false }
</script>

<template>
  <!-- Mobile hamburger -->
  <button class="hamburger" :class="{ open: mobileOpen }" @click="mobileOpen = !mobileOpen" aria-label="Toggle menu">
    <span /><span /><span />
  </button>
  <div v-if="mobileOpen" class="mobile-backdrop" @click="closeMobile" />

  <aside class="sidebar" :class="{ open: mobileOpen }">
    <!-- Brand -->
    <div class="brand">
      <img src="/icon.png" alt="Cyrene" class="brand-icon">
      <span class="brand-name">Cyrene</span>
      <span class="brand-ver">v{{ store.version }}</span>
    </div>

    <nav class="nav" aria-label="Main navigation">
      <p class="nav-group">Gateway</p>

      <template v-for="item in navGateway" :key="item.to">
        <router-link :to="item.to" class="nav-item" :class="{ active: isActive(item) }" @click="closeMobile">
          <component :is="item.icon" :size="15" />
          <span>{{ item.label }}</span>
        </router-link>
      </template>

      <!-- Media expandable group -->
      <button class="nav-item media-toggle" :class="{ active: isMediaActive }" @click="mediaExpanded = !mediaExpanded" :aria-expanded="mediaExpanded">
        <Clapperboard :size="15" />
        <span>Media Providers</span>
        <ChevronDown :size="13" class="chevron" :class="{ rotated: mediaExpanded }" />
      </button>
      <div class="subnav" :class="{ expanded: mediaExpanded }">
        <router-link
          v-for="m in mediaKinds" :key="m.kind"
          :to="`/media/${m.kind}`"
          class="nav-item sub"
          :class="{ active: route.path === `/media/${m.kind}` }"
          @click="closeMobile"
        >
          <component :is="m.icon" :size="13" />
          <span>{{ m.label }}</span>
        </router-link>
      </div>

      <template v-for="item in navGatewayAfter" :key="item.to">
        <router-link :to="item.to" class="nav-item" :class="{ active: isActive(item) }" @click="closeMobile">
          <component :is="item.icon" :size="15" />
          <span>{{ item.label }}</span>
        </router-link>
      </template>

      <div class="nav-divider" />
      <p class="nav-group">System</p>

      <template v-for="item in navSystem" :key="item.to">
        <router-link :to="item.to" class="nav-item" :class="{ active: isActive(item) }" @click="closeMobile">
          <component :is="item.icon" :size="15" />
          <span>{{ item.label }}</span>
        </router-link>
      </template>
    </nav>

    <!-- Footer -->
    <div class="sidebar-foot">
      <span class="status-dot" :class="{ ok: store.health.status === 'ok' }" />
      <span class="status-text">{{ store.health.status || '…' }}</span>
      <div class="foot-actions">
        <button class="foot-btn" @click="cycleLang" :title="`Language: ${lang}`" aria-label="Switch language">
          <Languages :size="14" />
        </button>
        <button class="foot-btn" @click="toggleTheme" :aria-label="theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'">
          <Sun v-if="theme === 'dark'" :size="14" />
          <Moon v-else :size="14" />
        </button>
        <button class="foot-btn" @click="$emit('logout')" aria-label="Sign out">
          <LogOut :size="14" />
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  position: fixed; top: 0; left: 0; bottom: 0;
  width: var(--sidebar-w); z-index: 50;
  display: flex; flex-direction: column;
  background: var(--glass);
  backdrop-filter: blur(var(--glass-blur));
  -webkit-backdrop-filter: blur(var(--glass-blur));
  border-right: 1px solid var(--glass-border);
  transition: transform 0.25s var(--ease-out-expo);
}

.brand {
  display: flex; align-items: center; gap: 9px;
  padding: 18px 16px 14px;
}
.brand-icon { width: 26px; height: 26px; border-radius: 8px; box-shadow: var(--shadow-accent); }
.brand-name { font-size: 14.5px; font-weight: 700; letter-spacing: -0.02em; }
.brand-ver { font-size: 10px; color: var(--text-faint); font-family: var(--font-mono); margin-left: auto; }

.nav { flex: 1; overflow-y: auto; padding: 0 8px 12px; }

.nav-group {
  font-size: 10px; font-weight: 700; text-transform: uppercase;
  letter-spacing: 0.08em; color: var(--text-faint);
  padding: 14px 10px 6px;
}

.nav-item {
  display: flex; align-items: center; gap: 9px;
  width: 100%; padding: 7px 10px; margin: 1px 0;
  border-radius: var(--radius-sm); border: none;
  background: transparent; color: var(--text-muted);
  font-size: 12.5px; font-weight: 500; text-align: left;
  cursor: pointer; transition: all 0.15s ease;
}
.nav-item:hover { background: var(--glass-hover); color: var(--text); }
.nav-item.active {
  background: var(--gradient-soft);
  color: var(--text);
  border: 1px solid rgba(45,212,191,0.15);
}
.nav-item.active :deep(svg) { color: var(--accent); }

.media-toggle { justify-content: flex-start; }
.chevron { margin-left: auto; transition: transform 0.2s var(--ease-spring); }
.chevron.rotated { transform: rotate(180deg); }

.subnav {
  overflow: hidden; max-height: 0;
  transition: max-height 0.25s var(--ease-out-expo);
  margin-left: 12px; padding-left: 8px;
  border-left: 1px solid var(--glass-border);
}
.subnav.expanded { max-height: 240px; }
.nav-item.sub { font-size: 12px; padding: 5.5px 8px; }

.nav-divider { height: 1px; background: var(--glass-border); margin: 10px 10px 0; }

.sidebar-foot {
  display: flex; align-items: center; gap: 7px;
  padding: 12px 14px; border-top: 1px solid var(--glass-border);
}
.status-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--text-faint); }
.status-dot.ok { background: var(--green); box-shadow: 0 0 6px var(--green); }
.status-text { font-size: 11px; color: var(--text-faint); }
.foot-actions { margin-left: auto; display: flex; gap: 2px; }
.foot-btn {
  width: 27px; height: 27px; border-radius: var(--radius-xs);
  display: flex; align-items: center; justify-content: center;
  background: transparent; border: none; color: var(--text-faint);
  cursor: pointer; transition: all 0.15s ease;
}
.foot-btn:hover { background: var(--glass-hover); color: var(--text); }

/* ─── Mobile ─── */
.hamburger {
  display: none; position: fixed; top: 12px; left: 12px; z-index: 60;
  width: 36px; height: 36px; border-radius: var(--radius-sm);
  background: var(--bg-elevated); border: 1px solid var(--glass-border);
  flex-direction: column; align-items: center; justify-content: center; gap: 4px;
  cursor: pointer;
}
.hamburger span {
  display: block; width: 16px; height: 1.5px;
  background: var(--text-muted); border-radius: 2px;
  transition: all 0.2s ease;
}
.hamburger.open span:nth-child(1) { transform: rotate(45deg) translate(3.5px, 3.5px); }
.hamburger.open span:nth-child(2) { opacity: 0; }
.hamburger.open span:nth-child(3) { transform: rotate(-45deg) translate(4px, -4px); }

.mobile-backdrop {
  display: none; position: fixed; inset: 0; z-index: 45;
  background: rgba(0,0,0,0.4);
}

@media (max-width: 768px) {
  .hamburger { display: flex; }
  .mobile-backdrop { display: block; }
  .sidebar { transform: translateX(-100%); }
  .sidebar.open { transform: translateX(0); box-shadow: var(--glass-depth); }
}
</style>
