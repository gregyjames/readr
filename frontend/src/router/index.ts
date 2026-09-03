import { createRouter, createWebHistory } from 'vue-router'
import { authState, checkAuthStatus } from '../store/auth'

const routes = [
  { path: '/', name: 'home', component: () => import('../components/Home.vue') },
  {
    path: '/login',
    name: 'login',
    component: () => import('../views/LoginView.vue'),
  },
  { path: '/articles/:id', name: 'article', component: () => import('../components/Article.vue'), props: true },
  { path: '/graph', name: 'graph', component: () => import('../components/Graph.vue') },
  { path: '/chat', name: 'chat', component: () => import('../views/ChatView.vue') },
  { path: '/chat/:id', name: 'chat-session', component: () => import('../views/ChatView.vue'), props: true },
  { path: '/archive', name: 'archive', component: () => import('../views/ArchiveView.vue') },
  { path: '/settings', name: 'settings', component: () => import('../views/SettingsView.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach(async (to, _from, next) => {
  if (!authState.isLoaded) {
    await checkAuthStatus()
  }

  if (!authState.isAuthenticated) {
    if (to.path !== '/login') {
      return next('/login')
    }
  } else {
    if (to.path === '/login') {
      return next('/')
    }
  }

  next()
})

export default router
