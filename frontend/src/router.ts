import { createRouter, createWebHistory } from 'vue-router'
import Home from './components/Home.vue'
import Add from './components/Add.vue'
import Article from './components/Article.vue'

const routes = [
  { path: '/', component: Home },
  { path: '/add', component: Add },
  { path: '/articles/:id', component: Article, props: true },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
