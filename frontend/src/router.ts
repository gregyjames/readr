import { createRouter, createWebHistory } from 'vue-router'
import Home from './components/Home.vue'
import Article from './components/Article.vue'

const routes = [
  { path: '/', name: 'home', component: Home },
  { path: '/articles/:id', component: Article, props: true },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
