import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/providers' },
    { path: '/providers', component: () => import('@/views/ProvidersView.vue') },
    { path: '/providers/:id', component: () => import('@/views/ProviderDetailView.vue') },
    { path: '/keys', component: () => import('@/views/KeysView.vue') },
    { path: '/aliases', component: () => import('@/views/AliasesView.vue') },
    { path: '/combos', component: () => import('@/views/CombosView.vue') },
    { path: '/media', component: () => import('@/views/MediaView.vue') },
    { path: '/cli-tools', component: () => import('@/views/CLIToolsView.vue') },
    { path: '/cli-tools/:id', component: () => import('@/views/CLIToolDetailView.vue') },
    { path: '/usage', component: () => import('@/views/UsageView.vue') },
    { path: '/quota', component: () => import('@/views/QuotaView.vue') },
    { path: '/tokensaver', component: () => import('@/views/TokenSaverView.vue') },
    { path: '/proxies', component: () => import('@/views/ProxiesView.vue') },
    { path: '/tunnel', component: () => import('@/views/TunnelView.vue') },
    { path: '/settings', component: () => import('@/views/SettingsView.vue') },
    { path: '/status', component: () => import('@/views/StatusView.vue') },
  ],
})

export default router
