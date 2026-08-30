import { createRouter, createWebHistory } from 'vue-router'
import Home from './components/Home.vue'
import Article from './components/Article.vue'
import Graph from './components/Graph.vue'
import ChatView from './views/ChatView.vue'
import SettingsView from './views/SettingsView.vue'

const routes = [
  { path: '/', name: 'home', component: Home },
  { path: '/articles/:id', component: Article, props: true },
  { path: '/graph', name: 'graph', component: Graph },
  { path: '/chat', name: 'chat', component: ChatView },
  { path: '/chat/:id', name: 'chat-session', component: ChatView, props: true },
  { path: '/settings', name: 'settings', component: SettingsView },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
