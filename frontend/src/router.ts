import { createRouter, createWebHistory } from 'vue-router'
import HelloWorld from './components/HelloWorld.vue'
import Article from './components/Article.vue'

const routes = [
  { path: '/', component: HelloWorld },
  { path: '/add', component: Article }
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
