import { reactive } from 'vue'
import { settings } from './settings'

export const authState = reactive({
  isLoaded: false,
  isAuthConfigured: false,
  isAuthenticated: false,
})

export async function checkAuthStatus(): Promise<boolean> {
  try {
    const res = await fetch('/api/auth/status')
    if (res.ok) {
      const data = await res.json()
      authState.isAuthConfigured = Boolean(data.auth_configured)
      authState.isAuthenticated = Boolean(data.authenticated)
      if (data.theme) {
        settings.theme = data.theme
        if (typeof document !== 'undefined') {
          document.documentElement.className = data.theme
        }
        try { localStorage.setItem('readr_theme', data.theme) } catch {}
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
    })
    if (res.ok) {
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
    })
    if (res.ok) {
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
    const res = await fetch('/api/auth/change-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
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

export async function logout(): Promise<void> {
  try {
    await fetch('/api/auth/logout', { method: 'POST' })
  } catch (err) {
    console.error('Logout error', err)
  } finally {
    authState.isAuthenticated = false
  }
}
