import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', name: 'endpoints', component: () => import('@/views/EndpointsView.vue') },
    { path: '/providers', name: 'providers', component: () => import('@/views/ProvidersView.vue') },
    { path: '/providers/:id', name: 'provider-detail', component: () => import('@/views/ProviderDetailView.vue'), props: true },
    { path: '/media', name: 'media', redirect: '/media/embedding' },
    { path: '/media/:kind', name: 'media-kind', component: () => import('@/views/MediaView.vue'), props: true },
    { path: '/combos', name: 'combos', component: () => import('@/views/CombosView.vue') },
    { path: '/usage', name: 'usage', component: () => import('@/views/UsageView.vue') },
    { path: '/quota', name: 'quota', component: () => import('@/views/QuotaView.vue') },
    { path: '/token-saver', name: 'token-saver', component: () => import('@/views/TokenSaverView.vue') },
    { path: '/cli-tools', name: 'cli-tools', component: () => import('@/views/CLIToolsView.vue') },
    { path: '/cli-tools/:id', name: 'cli-tool-detail', component: () => import('@/views/CLIToolDetailView.vue'), props: true },
    { path: '/console', name: 'console', component: () => import('@/views/ConsoleView.vue') },
    { path: '/proxy-pools', name: 'proxy-pools', component: () => import('@/views/ProxyPoolsView.vue') },
    { path: '/tunnel', name: 'tunnel', component: () => import('@/views/TunnelView.vue') },
    { path: '/mitm', name: 'mitm', component: () => import('@/views/MitmView.vue') },
    { path: '/skills', name: 'skills', component: () => import('@/views/SkillsView.vue') },
    { path: '/settings', name: 'settings', component: () => import('@/views/SettingsView.vue') },
    // Legacy redirects
    { path: '/proxies', redirect: '/proxy-pools' },
    { path: '/tokensaver', redirect: '/token-saver' },
    { path: '/endpoints', redirect: '/' },
    { path: '/keys', redirect: '/' },
    { path: '/chat', redirect: '/' },
    { path: '/aliases', redirect: '/providers' },
    { path: '/status', redirect: '/settings' },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

export default router
