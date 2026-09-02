import { reactive } from 'vue'
import { applyTheme } from './settings'

export const authState = reactive({
  isLoaded: false,
  isAuthConfigured: false,
  isAuthenticated: false,
})

function getStoredToken(): string | null {
  try {
    return localStorage.getItem('readr_token')
  } catch {
    return null
  }
}

function setStoredToken(token: string | null) {
  try {
    if (token) {
      localStorage.setItem('readr_token', token)
    } else {
      localStorage.removeItem('readr_token')
    }
  } catch {}
}

export async function checkAuthStatus(): Promise<boolean> {
  try {
    const headers: Record<string, string> = {}
    const token = getStoredToken()
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }

    const res = await fetch('/api/auth/status', {
      headers,
      credentials: 'include',
    })
    if (res.ok) {
      const data = await res.json()
      authState.isAuthConfigured = Boolean(data.auth_configured)
      authState.isAuthenticated = Boolean(data.authenticated)
      if (data.theme) {
        applyTheme(data.theme)
      }
      return authState.isAuthenticated
    }
  } catch (err) {
    console.error('Failed to check auth status', err)
  } finally {
    authState.isLoaded = true
  }
  return false
}

export async function login(password: string): Promise<{ success: boolean; error?: string }> {
  try {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
      credentials: 'include',
    })
    if (res.ok) {
      const data = await res.json().catch(() => ({}))
      if (data.token) {
        setStoredToken(data.token)
      }
      authState.isAuthenticated = true
      authState.isAuthConfigured = true
      return { success: true }
    }
    const data = await res.json().catch(() => ({}))
    return { success: false, error: data.error || 'Invalid password' }
  } catch (err: any) {
    return { success: false, error: err.message || 'Network error' }
  }
}

export async function setupMasterPassword(password: string): Promise<{ success: boolean; error?: string }> {
  try {
    const res = await fetch('/api/auth/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
      credentials: 'include',
    })
    if (res.ok) {
      const data = await res.json().catch(() => ({}))
      if (data.token) {
        setStoredToken(data.token)
      }
      authState.isAuthenticated = true
      authState.isAuthConfigured = true
      return { success: true }
    }
    const data = await res.json().catch(() => ({}))
    return { success: false, error: data.error || 'Setup failed' }
  } catch (err: any) {
    return { success: false, error: err.message || 'Network error' }
  }
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<{ success: boolean; error?: string }> {
  try {
    const headers: Record<string, string> = { 'Content-Type': 'application/json' }
    const token = getStoredToken()
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }

    const res = await fetch('/api/auth/change-password', {
      method: 'POST',
      headers,
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
      credentials: 'include',
    })
    if (res.ok) {
      return { success: true }
    }
    const data = await res.json().catch(() => ({}))
    return { success: false, error: data.error || 'Password update failed' }
  } catch (err: any) {
    return { success: false, error: err.message || 'Network error' }
  }
}

export async function logout(): Promise<boolean> {
  try {
    const headers: Record<string, string> = {}
    const token = getStoredToken()
    if (token) {
      headers['Authorization'] = `Bearer ${token}`
    }

    const res = await fetch('/api/auth/logout', { 
      method: 'POST',
      headers,
      credentials: 'include',
    })
    setStoredToken(null)
    if (res.ok) {
      authState.isAuthenticated = false
      return true
    }
    return false
  } catch (err) {
    console.error('Logout error', err)
    setStoredToken(null)
    return false
  }
}
