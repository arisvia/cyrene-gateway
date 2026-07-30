import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', component: () => import('@/views/EndpointsView.vue') },
    { path: '/providers', component: () => import('@/views/ProvidersView.vue') },
    { path: '/providers/:id', component: () => import('@/views/ProviderDetailView.vue') },
    { path: '/media', component: () => import('@/views/MediaView.vue') },
    { path: '/combos', component: () => import('@/views/CombosView.vue') },
    { path: '/usage', component: () => import('@/views/UsageView.vue') },
    { path: '/quota', component: () => import('@/views/QuotaView.vue') },
    { path: '/tokensaver', component: () => import('@/views/TokenSaverView.vue') },
    { path: '/cli-tools', component: () => import('@/views/CLIToolsView.vue') },
    { path: '/cli-tools/:id', component: () => import('@/views/CLIToolDetailView.vue') },
    { path: '/console', component: () => import('@/views/ConsoleView.vue') },
    { path: '/proxies', component: () => import('@/views/ProxiesView.vue') },
    { path: '/tunnel', component: () => import('@/views/TunnelView.vue') },
    { path: '/mitm', component: () => import('@/views/MitmView.vue') },
    { path: '/skills', component: () => import('@/views/SkillsView.vue') },
    { path: '/settings', component: () => import('@/views/SettingsView.vue') },
    // Legacy redirects
    { path: '/endpoints', redirect: '/' },
    { path: '/keys', redirect: '/' },
    { path: '/aliases', redirect: '/providers' },
    { path: '/chat', redirect: '/' },
    { path: '/status', redirect: '/settings' },
  ],
})

export default router
