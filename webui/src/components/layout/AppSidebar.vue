<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useGatewayStore } from '@/stores/gateway'
import { formatUptime } from '@/lib/format'
import { LOCALES, locale, setLocale } from '@/i18n'
import { Sun, Moon, LogOut, Server, KeyRound, GitBranch, Layers, BarChart3, Gauge, Zap, Globe, Network, Settings, Activity, Clapperboard, Terminal, ScrollText, Link2, MessageSquare, Sparkles, Languages } from 'lucide-vue-next'

const store = useGatewayStore()
const route = useRoute()
const router = useRouter()

const theme = ref(document.documentElement.classList.contains('light') ? 'light' : 'dark')
function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.classList.toggle('light', theme.value === 'light')
  localStorage.setItem('cyrene-theme', theme.value)
}

const showLangMenu = ref(false)
function selectLang(code: string) {
  setLocale(code)
  showLangMenu.value = false
}

const navMain = [
  { path: '/providers', label: 'Providers', icon: Server },
  { path: '/media', label: 'Media', icon: Clapperboard },
  { path: '/cli-tools', label: 'CLI Tools', icon: Terminal },
  { path: '/endpoints', label: 'Endpoints', icon: Link2 },
  { path: '/chat', label: 'Chat', icon: MessageSquare },
  { path: '/skills', label: 'Skills', icon: Sparkles },
  { path: '/keys', label: 'API Keys', icon: KeyRound },
  { path: '/aliases', label: 'Model Aliases', icon: GitBranch },
  { path: '/combos', label: 'Combos', icon: Layers },
  { path: '/usage', label: 'Usage', icon: BarChart3 },
  { path: '/console', label: 'Console Log', icon: ScrollText },
  { path: '/quota', label: 'Quota', icon: Gauge },
  { path: '/tokensaver', label: 'Token Saver', icon: Zap },
]
const navSystem = [
  { path: '/proxies', label: 'Proxy Pools', icon: Globe },
  { path: '/tunnel', label: 'Tunnel', icon: Network },
  { path: '/settings', label: 'Settings', icon: Settings },
  { path: '/status', label: 'Status', icon: Activity },
]

const emit = defineEmits<{ logout: [] }>()

const mobileOpen = ref(false)
function closeMobile() { mobileOpen.value = false }
</script>

<template>
  <!-- Mobile hamburger -->
  <button class="mobile-toggle" @click="mobileOpen = true" aria-label="Open menu">
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 12h18M3 6h18M3 18h18"/></svg>
  </button>
  <!-- Mobile overlay -->
  <div v-if="mobileOpen" class="mobile-overlay" @click="closeMobile"></div>
  <aside :class="['sidebar', mobileOpen && 'mobile-open']">
    <div class="sidebar-brand">
      <img src="/icon.png" alt="Cyrene" class="brand-icon-img">
      <span class="brand-name">Cyrene</span>
      <span class="brand-ver">v{{ store.version }}</span>
    </div>
    <nav class="sidebar-nav">
      <p class="nav-group-label">Gateway</p>
      <router-link
        v-for="item in navMain" :key="item.path" :to="item.path"
        :class="['nav-item', route.path === item.path && 'active']"
        @click="closeMobile"
      >
        <component :is="item.icon" :size="15" /><span>{{ item.label }}</span>
      </router-link>
      <p class="nav-group-label">System</p>
      <router-link
        v-for="item in navSystem" :key="item.path" :to="item.path"
        :class="['nav-item', route.path === item.path && 'active']"
        @click="closeMobile"
      >
        <component :is="item.icon" :size="15" /><span>{{ item.label }}</span>
      </router-link>
    </nav>
    <div class="sidebar-footer">
      <span class="status-dot"></span>
      <span class="status-text">{{ store.health.status || 'online' }} · {{ formatUptime(store.health.uptimeSeconds) }}</span>
      <div class="lang-wrap">
        <button class="footer-btn" @click="showLangMenu = !showLangMenu" title="Language">
          <Languages :size="14" />
        </button>
        <div v-if="showLangMenu" class="lang-menu">
          <button v-for="l in LOCALES" :key="l.code"
            :class="['lang-item', locale === l.code && 'active']"
            @click="selectLang(l.code)">
            <span>{{ l.flag }}</span><span>{{ l.label }}</span>
          </button>
        </div>
      </div>
      <button class="footer-btn" @click="toggleTheme" :title="theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'">
        <Sun v-if="theme === 'dark'" :size="14" />
        <Moon v-else :size="14" />
      </button>
      <button class="footer-btn" @click="$emit('logout')" title="Sign out">
        <LogOut :size="14" />
      </button>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  position: fixed; inset-block: 0; left: 0; z-index: 40;
  width: var(--sidebar-w); display: flex; flex-direction: column;
  background: var(--sidebar-bg);
  backdrop-filter: blur(20px); -webkit-backdrop-filter: blur(20px);
  border-right: 1px solid var(--glass-border);
  transition: background 0.3s ease;
}
.sidebar-brand {
  display: flex; align-items: center; gap: 10px;
  height: 56px; padding: 0 16px;
  border-bottom: 1px solid var(--glass-border);
}
.brand-icon-img {
  width: 30px; height: 30px; border-radius: 8px;
  box-shadow: var(--shadow-accent);
  flex-shrink: 0;
}
.brand-name { font-size: 13.5px; font-weight: 650; letter-spacing: -0.02em; }
.brand-ver { font-size: 10px; color: var(--text-faint); font-family: var(--font-mono); margin-left: 6px; }

.sidebar-nav { flex: 1; overflow-y: auto; padding: 12px 10px; }
.nav-group-label {
  padding: 10px 10px 5px; font-size: 10px; font-weight: 600;
  text-transform: uppercase; letter-spacing: 0.08em; color: var(--text-faint);
}
.nav-item {
  display: flex; align-items: center; gap: 10px;
  padding: 8px 10px; border-radius: var(--radius-sm);
  font-size: 13px; font-weight: 480; color: var(--text-muted);
  cursor: pointer; transition: all 0.15s ease; user-select: none;
  border: 1px solid transparent; margin-bottom: 1px;
  text-decoration: none;
}
.nav-item svg { opacity: 0.7; flex-shrink: 0; }
.nav-item:hover { color: var(--text); background: var(--glass-hover); }
.nav-item.active {
  color: var(--text); background: var(--glass-hover);
  border-color: var(--glass-border);
  box-shadow: inset 0 1px 0 rgba(255,255,255,0.04), var(--shadow-glow);
}
.nav-item.active svg { opacity: 1; color: var(--accent); }

.sidebar-footer {
  padding: 12px 14px; border-top: 1px solid var(--glass-border);
  display: flex; align-items: center; gap: 8px;
}
.status-dot { position: relative; width: 7px; height: 7px; border-radius: 50%; background: var(--green); flex-shrink: 0; }
.status-dot::after {
  content: ''; position: absolute; inset: -3px; border-radius: 50%;
  background: var(--green); opacity: 0.4; animation: ping 2s cubic-bezier(0,0,0.2,1) infinite;
}
@keyframes ping { 75%, 100% { transform: scale(2.2); opacity: 0; } }
.status-text { font-size: 11px; color: var(--text-muted); font-family: var(--font-mono); }
.footer-btn {
  width: 27px; height: 27px; border-radius: var(--radius-sm);
  display: flex; align-items: center; justify-content: center;
  background: transparent; border: 1px solid transparent;
  color: var(--text-muted); cursor: pointer; transition: all 0.15s ease;
  margin-left: auto;
}
.footer-btn + .footer-btn { margin-left: 0; }
.footer-btn:hover { color: var(--text); background: var(--glass-hover); border-color: var(--glass-border); }
.lang-wrap { position: relative; margin-left: auto; }
.lang-menu {
  position: absolute; bottom: calc(100% + 8px); right: 0; z-index: 100;
  min-width: 150px; padding: 6px;
  background: var(--dialog-bg); border: 1px solid var(--glass-border-hover);
  border-radius: var(--radius); box-shadow: var(--glass-depth);
  backdrop-filter: blur(20px); -webkit-backdrop-filter: blur(20px);
  animation: langIn 0.15s var(--ease-spring);
}
@keyframes langIn { from { opacity: 0; transform: translateY(4px) scale(0.96); } }
.lang-item {
  display: flex; align-items: center; gap: 8px; width: 100%;
  padding: 7px 10px; border: none; border-radius: var(--radius-sm);
  background: transparent; color: var(--text-muted); font-size: 12px;
  font-family: var(--font); cursor: pointer; transition: all 0.12s ease;
}
.lang-item:hover { background: var(--glass-hover); color: var(--text); }
.lang-item.active { color: var(--accent); font-weight: 550; }

/* Mobile */
.mobile-toggle {
  display: none; position: fixed; top: 14px; left: 14px; z-index: 50;
  width: 36px; height: 36px; border-radius: var(--radius-sm);
  align-items: center; justify-content: center;
  background: var(--glass); border: 1px solid var(--glass-border);
  color: var(--text-muted); cursor: pointer;
  backdrop-filter: blur(12px); -webkit-backdrop-filter: blur(12px);
}
.mobile-overlay {
  display: none; position: fixed; inset: 0; z-index: 39;
  background: var(--overlay-bg);
}
@media (max-width: 768px) {
  .mobile-toggle { display: flex; }
  .mobile-overlay { display: block; }
  .sidebar {
    transform: translateX(-100%);
    transition: transform 0.25s var(--ease-out-expo);
  }
  .sidebar.mobile-open { transform: translateX(0); }
}
</style>
