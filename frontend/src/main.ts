import { createApp } from 'vue'
import axios from 'axios'
import './style.css'
import App from './App.vue'
import router from './router'
import { authState, getStoredToken } from './store/auth'

// Ensure credentials (cookies) are sent with all requests
axios.defaults.withCredentials = true

// Attach stored auth token to every outgoing request if available
axios.interceptors.request.use((config) => {
  const token = getStoredToken()
  if (token && !config.headers.Authorization) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Redirect to login when receiving 401 Unauthorized
axios.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401 && router.currentRoute.value.path !== '/login') {
      authState.isAuthenticated = false
      router.push('/login')
    }
    return Promise.reject(error)
  }
)

createApp(App).use(router).mount('#app')
