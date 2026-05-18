import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'services', component: () => import('../views/DashboardView.vue') },
  { path: '/traces', name: 'traces', component: () => import('../views/TracesView.vue') },
  { path: '/metrics', name: 'metrics', component: () => import('../views/MetricsView.vue') },
  { path: '/resources', name: 'resources', component: () => import('../views/ResourcesView.vue') },
  { path: '/logs', name: 'logs', component: () => import('../views/LogsView.vue') },
  { path: '/mcp-setup', name: 'mcp-setup', component: () => import('../views/McpSetupView.vue') },
]

export default createRouter({
  history: createWebHistory(),
  routes,
})
