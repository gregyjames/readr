import { createRouter, createWebHistory } from 'vue-router'
import HelloWorld from './components/HelloWorld.vue'
import Add from './components/Add.vue'
import Article from './components/Article.vue'

const routes = [
  { path: '/', component: HelloWorld },
  { path: '/add', component: Add },
  { path: '/articles/:id', component: Article, props: true },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
