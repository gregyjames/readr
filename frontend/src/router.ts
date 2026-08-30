import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'home', component: () => import('./components/Home.vue') },
  { path: '/articles/:id', name: 'article', component: () => import('./components/Article.vue'), props: true },
  { path: '/graph', name: 'graph', component: () => import('./components/Graph.vue') },
  { path: '/chat', name: 'chat', component: () => import('./views/ChatView.vue') },
  { path: '/chat/:id', name: 'chat-session', component: () => import('./views/ChatView.vue'), props: true },
  { path: '/settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
