import { createRouter, createWebHistory } from 'vue-router'
import HelloWorld from './components/HelloWorld.vue'
import Add from './components/Add.vue'

const routes = [
  { path: '/', component: HelloWorld },
  { path: '/add', component: Add }
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
