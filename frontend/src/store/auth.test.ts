import { describe, it, expect, beforeEach, afterEach } from 'bun:test'
import {
  authState,
  getStoredToken,
  checkAuthStatus,
  login,
  setupMasterPassword,
  changePassword,
  logout,
} from './auth'

describe('store/auth', () => {
  const originalFetch = globalThis.fetch

  beforeEach(() => {
    localStorage.clear()
    authState.isLoaded = false
    authState.isAuthConfigured = false
    authState.isAuthenticated = false
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
  })

  describe('getStoredToken', () => {
    it('returns null when no token is present', () => {
      expect(getStoredToken()).toBeNull()
    })

    it('returns stored token from localStorage', () => {
      localStorage.setItem('readr_token', 'test-session-token')
      expect(getStoredToken()).toBe('test-session-token')
    })
  })

  describe('checkAuthStatus', () => {
    it('updates state and returns true when authenticated', async () => {
      localStorage.setItem('readr_token', 'active-token')

      let calledUrl = ''
      let capturedHeaders: Record<string, string> = {}
      let capturedCredentials = ''

      globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
        calledUrl = String(input)
        capturedHeaders = (init?.headers || {}) as Record<string, string>
        capturedCredentials = init?.credentials || ''

        return {
          ok: true,
          json: async () => ({
            auth_configured: true,
            authenticated: true,
            theme: 'dark',
          }),
        } as unknown as Response
      }) as typeof fetch

      const result = await checkAuthStatus()

      expect(result).toBe(true)
      expect(calledUrl).toBe('/api/auth/status')
      expect(capturedCredentials).toBe('include')
      expect(capturedHeaders['Authorization']).toBe('Bearer active-token')
      expect(authState.isLoaded).toBe(true)
      expect(authState.isAuthConfigured).toBe(true)
      expect(authState.isAuthenticated).toBe(true)
    })

    it('handles unauthenticated response cleanly', async () => {
      globalThis.fetch = (async () => {
        return {
          ok: true,
          json: async () => ({
            auth_configured: true,
            authenticated: false,
          }),
        } as unknown as Response
      }) as typeof fetch

      const result = await checkAuthStatus()

      expect(result).toBe(false)
      expect(authState.isLoaded).toBe(true)
      expect(authState.isAuthConfigured).toBe(true)
      expect(authState.isAuthenticated).toBe(false)
    })

    it('handles network error by marking loaded and returning false', async () => {
      const errSpy = console.error
      console.error = () => {}
      try {
        globalThis.fetch = (async () => {
          throw new Error('Connection refused')
        }) as typeof fetch

        const result = await checkAuthStatus()

        expect(result).toBe(false)
        expect(authState.isLoaded).toBe(true)
        expect(authState.isAuthenticated).toBe(false)
      } finally {
        console.error = errSpy
      }
    })
  })

  describe('login', () => {
    it('stores token and sets authentication state on success', async () => {
      let bodyText = ''
      globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
        bodyText = String(init?.body || '')
        return {
          ok: true,
          json: async () => ({ token: 'new-jwt-token' }),
        } as unknown as Response
      }) as typeof fetch

      const res = await login('vault-master-pass')

      expect(res.success).toBe(true)
      expect(JSON.parse(bodyText)).toEqual({ password: 'vault-master-pass' })
      expect(localStorage.getItem('readr_token')).toBe('new-jwt-token')
      expect(authState.isAuthenticated).toBe(true)
      expect(authState.isAuthConfigured).toBe(true)
    })

    it('returns error message when password is wrong', async () => {
      globalThis.fetch = (async () => {
        return {
          ok: false,
          json: async () => ({ error: 'Invalid master password' }),
        } as unknown as Response
      }) as typeof fetch

      const res = await login('bad-pass')

      expect(res.success).toBe(false)
      expect(res.error).toBe('Invalid master password')
      expect(localStorage.getItem('readr_token')).toBeNull()
      expect(authState.isAuthenticated).toBe(false)
    })

    it('handles network failure gracefully', async () => {
      globalThis.fetch = (async () => {
        throw new Error('Network offline')
      }) as typeof fetch

      const res = await login('any-pass')

      expect(res.success).toBe(false)
      expect(res.error).toBe('Network offline')
    })
  })

  describe('setupMasterPassword', () => {
    it('sets master password and updates state on success', async () => {
      globalThis.fetch = (async () => {
        return {
          ok: true,
          json: async () => ({ token: 'setup-session-token' }),
        } as unknown as Response
      }) as typeof fetch

      const res = await setupMasterPassword('new-strong-password')

      expect(res.success).toBe(true)
      expect(localStorage.getItem('readr_token')).toBe('setup-session-token')
      expect(authState.isAuthenticated).toBe(true)
      expect(authState.isAuthConfigured).toBe(true)
    })

    it('handles setup failure response', async () => {
      globalThis.fetch = (async () => {
        return {
          ok: false,
          json: async () => ({ error: 'Password too short' }),
        } as unknown as Response
      }) as typeof fetch

      const res = await setupMasterPassword('short')

      expect(res.success).toBe(false)
      expect(res.error).toBe('Password too short')
    })
  })

  describe('changePassword', () => {
    it('sends current and new password with bearer token', async () => {
      localStorage.setItem('readr_token', 'my-auth-token')
      let sentHeaders: Record<string, string> = {}
      let sentBody = ''

      globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
        sentHeaders = (init?.headers || {}) as Record<string, string>
        sentBody = String(init?.body || '')
        return {
          ok: true,
          json: async () => ({}),
        } as unknown as Response
      }) as typeof fetch

      const res = await changePassword('old-secret', 'new-secret')

      expect(res.success).toBe(true)
      expect(sentHeaders['Authorization']).toBe('Bearer my-auth-token')
      expect(JSON.parse(sentBody)).toEqual({
        current_password: 'old-secret',
        new_password: 'new-secret',
      })
    })

    it('returns error on change password failure', async () => {
      globalThis.fetch = (async () => {
        return {
          ok: false,
          json: async () => ({ error: 'Current password incorrect' }),
        } as unknown as Response
      }) as typeof fetch

      const res = await changePassword('wrong', 'new')

      expect(res.success).toBe(false)
      expect(res.error).toBe('Current password incorrect')
    })
  })

  describe('logout', () => {
    it('clears stored token and resets auth state', async () => {
      localStorage.setItem('readr_token', 'token-to-invalidate')
      authState.isAuthenticated = true

      let capturedUrl = ''
      globalThis.fetch = (async (input: RequestInfo | URL) => {
        capturedUrl = String(input)
        return {
          ok: true,
          json: async () => ({ status: 'success' }),
        } as unknown as Response
      }) as typeof fetch

      const res = await logout()

      expect(res).toBe(true)
      expect(capturedUrl).toBe('/api/auth/logout')
      expect(localStorage.getItem('readr_token')).toBeNull()
      expect(authState.isAuthenticated).toBe(false)
    })

    it('clears local token even if logout API returns error or fails', async () => {
      const errSpy = console.error
      console.error = () => {}
      try {
        localStorage.setItem('readr_token', 'token-to-clear')
        authState.isAuthenticated = true

        globalThis.fetch = (async () => {
          throw new Error('Server unreachable')
        }) as typeof fetch

        const res = await logout()

        expect(res).toBe(false)
        expect(localStorage.getItem('readr_token')).toBeNull()
      } finally {
        console.error = errSpy
      }
    })
  })
})
