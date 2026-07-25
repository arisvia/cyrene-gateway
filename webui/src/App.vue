<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import GToastHost from '@/components/ui/GToastHost.vue'

const store = useGatewayStore()

const authState = ref<'checking' | 'login' | 'ready'>('checking')
const loginPassword = ref('')
const loginError = ref('')
const loginLoading = ref(false)

async function checkAuth() {
  try {
    const st = await api('/api/auth/status')
    if (st.authenticated) {
      authState.value = 'ready'
      store.loadAll()
    } else {
      authState.value = 'login'
    }
  } catch {
    authState.value = 'ready'
    store.loadAll()
  }
}

async function doLogin() {
  if (!loginPassword.value || loginLoading.value) return
  loginLoading.value = true
  loginError.value = ''
  try {
    const res = await apiPost('/api/auth/login', { password: loginPassword.value })
    if (res.error) {
      loginError.value = res.error === 'too many failed attempts'
        ? `Too many attempts — retry in ${Math.ceil((res.retryAfter || 30000) / 1000)}s`
        : 'Invalid password'
    } else {
      loginPassword.value = ''
      authState.value = 'ready'
      store.loadAll()
    }
  } catch {
    loginError.value = 'Connection failed'
  }
  loginLoading.value = false
}

async function doLogout() {
  await apiPost('/api/auth/logout')
  authState.value = 'login'
  loginPassword.value = ''
  loginError.value = ''
}

onMounted(checkAuth)
</script>

<template>
  <div class="ambient"></div>
  <GToastHost />

  <!-- Login Screen -->
  <div v-if="authState === 'login'" class="login-wrap">
    <div class="login-card">
      <div class="login-brand">
        <img src="/icon.png" alt="Cyrene" class="brand-icon-img">
        <div>
          <p class="login-title">Cyrene Gateway</p>
          <p class="login-sub">Sign in to access the dashboard</p>
        </div>
      </div>
      <div v-if="loginError" class="login-error">{{ loginError }}</div>
      <div class="form-group">
        <label class="form-label">Password</label>
        <input v-model="loginPassword" type="password" class="input" placeholder="Enter dashboard password"
          @keyup.enter="doLogin" autofocus>
      </div>
      <button class="btn-login" @click="doLogin" :disabled="loginLoading">
        <span v-if="loginLoading">Signing in…</span><span v-else>Sign In</span>
      </button>
    </div>
  </div>

  <!-- Dashboard -->
  <div v-else-if="authState === 'ready'" class="app">
    <AppSidebar @logout="doLogout" />
    <main class="main">
      <div class="content">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </div>
    </main>
  </div>
</template>

<style scoped>
/* Page transition */
.page-enter-active { animation: pageIn 0.3s var(--ease-out-expo); }
.page-leave-active { animation: pageOut 0.15s ease; }
@keyframes pageIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}
@keyframes pageOut {
  from { opacity: 1; transform: translateY(0); }
  to { opacity: 0; transform: translateY(-6px); }
}

.login-wrap {
  position: relative; z-index: 1; min-height: 100vh;
  display: flex; align-items: center; justify-content: center; padding: 20px;
}
.login-card {
  width: 100%; max-width: 360px;
  background: var(--glass);
  backdrop-filter: blur(20px); -webkit-backdrop-filter: blur(20px);
  border: 1px solid var(--glass-border-hover);
  border-radius: 16px; padding: 36px 32px;
  box-shadow: 0 24px 64px rgba(0,0,0,0.3), var(--shadow-glow);
  animation: slideUp 0.25s ease;
}
.login-brand { display: flex; align-items: center; gap: 12px; margin-bottom: 28px; }
.brand-icon-img {
  width: 38px; height: 38px; border-radius: 10px;
  box-shadow: var(--shadow-accent);
  flex-shrink: 0;
}
.login-title { font-size: 17px; font-weight: 650; letter-spacing: -0.02em; }
.login-sub { font-size: 12px; color: var(--text-faint); margin-top: 2px; }
.login-error {
  background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.25);
  border-radius: var(--radius-sm); padding: 8px 12px;
  font-size: 12px; color: var(--red); margin-bottom: 14px;
  animation: fadeIn 0.15s ease;
}
.form-group { margin-bottom: 14px; }
.form-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.input {
  width: 100%; height: 34px; padding: 0 12px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font);
  transition: all 0.15s ease; outline: none;
}
.input::placeholder { color: var(--text-faint); }
.input:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.btn-login {
  width: 100%; height: 38px; border: none; border-radius: var(--radius-sm);
  background: var(--gradient); color: var(--on-accent);
  font-size: 13px; font-weight: 600; font-family: var(--font);
  cursor: pointer; box-shadow: var(--shadow-accent);
  transition: all 0.15s ease;
}
.btn-login:hover:not(:disabled) { box-shadow: var(--shadow-accent-hover); filter: brightness(1.1); }
.btn-login:disabled { opacity: 0.6; }
</style>
