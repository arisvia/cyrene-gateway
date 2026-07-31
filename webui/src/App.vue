<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useGatewayStore } from '@/stores/gateway'
import { api, apiPost } from '@/lib/api'
import AppSidebar from '@/components/layout/AppSidebar.vue'
import GToastHost from '@/components/ui/GToastHost.vue'
import GButton from '@/components/ui/GButton.vue'

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
      store.loadCore()
    } else {
      authState.value = 'login'
    }
  } catch {
    authState.value = 'ready'
    store.loadCore()
  }
}

async function doLogin() {
  if (!loginPassword.value || loginLoading.value) return
  loginLoading.value = true
  loginError.value = ''
  try {
    const res = await apiPost('/api/auth/login', { password: loginPassword.value })
    if (res?.error) {
      loginError.value = res.error === 'too many failed attempts' ? 'Too many attempts — try again later' : 'Invalid password'
    } else {
      loginPassword.value = ''
      authState.value = 'ready'
      store.loadCore()
    }
  } catch {
    loginError.value = 'Connection failed'
  }
  loginLoading.value = false
}

async function doLogout() {
  await apiPost('/api/auth/logout')
  authState.value = 'login'
}

onMounted(checkAuth)
</script>

<template>
  <!-- Ambient gradient orbs -->
  <div class="ambient" aria-hidden="true">
    <div class="orb orb-1" />
    <div class="orb orb-2" />
  </div>

  <GToastHost />

  <!-- Login -->
  <div v-if="authState === 'login'" class="login-wrap">
    <div class="login-card">
      <div class="login-brand">
        <img src="/icon.png" alt="Cyrene" class="login-icon">
        <div>
          <p class="login-title">Cyrene Gateway</p>
          <p class="login-sub">Sign in to access the dashboard</p>
        </div>
      </div>
      <p v-if="loginError" class="login-error">{{ loginError }}</p>
      <label class="field-label" for="pw">Password</label>
      <input
        id="pw" v-model="loginPassword" type="password" class="field"
        placeholder="Enter dashboard password" @keyup.enter="doLogin" autofocus
      >
      <GButton class="login-btn" :loading="loginLoading" @click="doLogin">Sign In</GButton>
    </div>
  </div>

  <!-- Dashboard shell -->
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
/* Ambient background */
.ambient { position: fixed; inset: 0; z-index: -1; overflow: hidden; pointer-events: none; }
.orb {
  position: absolute; border-radius: 50%; filter: blur(80px); opacity: 0.35;
  animation: orbFloat 20s ease-in-out infinite;
}
.orb-1 {
  width: 500px; height: 500px; top: -15%; right: -10%;
  background: radial-gradient(circle, rgba(45,212,191,0.2), transparent 70%);
}
.orb-2 {
  width: 450px; height: 450px; bottom: -10%; left: -5%;
  background: radial-gradient(circle, rgba(139,92,246,0.18), transparent 70%);
  animation-delay: -10s;
}

/* Page transition */
.page-enter-active { animation: slideUp 0.25s var(--ease-out-expo); }
.page-leave-active { animation: fadeIn 0.12s ease reverse; }

/* Layout */
.app { min-height: 100vh; }
.main { margin-left: var(--sidebar-w); min-height: 100vh; }
.content { max-width: 1080px; margin: 0 auto; padding: 32px 36px 80px; }

@media (max-width: 768px) {
  .main { margin-left: 0; }
  .content { padding: 64px 16px 80px; }
}

/* Login */
.login-wrap {
  min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 20px;
}
.login-card {
  width: 100%; max-width: 360px;
  background: var(--glass);
  backdrop-filter: blur(var(--glass-blur));
  border: 1px solid var(--glass-border-hover);
  border-radius: 16px; padding: 34px 30px;
  box-shadow: var(--glass-depth), var(--shadow-glow);
  animation: slideUp 0.3s var(--ease-out-expo);
}
.login-brand { display: flex; align-items: center; gap: 12px; margin-bottom: 26px; }
.login-icon { width: 38px; height: 38px; border-radius: 10px; box-shadow: var(--shadow-accent); }
.login-title { font-size: 16.5px; font-weight: 700; letter-spacing: -0.02em; }
.login-sub { font-size: 12px; color: var(--text-faint); margin-top: 2px; }
.login-error {
  background: rgba(248,113,113,0.08); border: 1px solid rgba(248,113,113,0.25);
  border-radius: var(--radius-sm); padding: 8px 12px;
  font-size: 12px; color: var(--red); margin-bottom: 14px;
}
.field-label { display: block; font-size: 11.5px; font-weight: 550; color: var(--text-muted); margin-bottom: 6px; }
.field {
  width: 100%; height: 36px; padding: 0 12px; margin-bottom: 16px;
  background: var(--code-bg); color: var(--text);
  border: 1px solid var(--glass-border); border-radius: var(--radius-sm);
  font-size: 13px; font-family: var(--font); outline: none;
  transition: all 0.15s ease;
}
.field::placeholder { color: var(--text-faint); }
.field:focus { border-color: var(--ring); box-shadow: 0 0 0 3px var(--ring-soft); }
.login-btn { width: 100%; justify-content: center; height: 38px !important; }
</style>
