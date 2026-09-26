import { describe, it, expect, beforeEach, afterEach } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import axios from 'axios'
import LoginView from './LoginView.vue'
import ArchiveView from './ArchiveView.vue'
import { authState } from '../store/auth'

describe('Views: LoginView and ArchiveView', () => {
  const setupRouter = async (initialPath = '/login') => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div>Library Home</div>' } },
        { path: '/login', component: { template: '<div>Login</div>' } },
        { path: '/archive', component: { template: '<div>Archive</div>' } },
        { path: '/articles/:id', component: { template: '<div>Article Detail</div>' } },
      ],
    })
    await router.push(initialPath)
    await router.isReady()
    return router
  }

  describe('LoginView.vue', () => {
    const origFetch = globalThis.fetch

    beforeEach(() => {
      localStorage.clear()
      authState.isLoaded = true
      authState.isAuthConfigured = true
      authState.isAuthenticated = false
    })

    afterEach(() => {
      globalThis.fetch = origFetch
    })

    it('renders login form when auth is configured', async () => {
      const router = await setupRouter('/login')
      const wrapper = mount(LoginView, {
        global: {
          plugins: [router],
        },
      })

      expect(wrapper.text()).toContain('Readr Vault')
      expect(wrapper.text()).toContain('Unlock Vault')
      expect(wrapper.text()).not.toContain('Set Master Password')
      expect(wrapper.find('input[type="password"]').exists()).toBe(true)
    })

    it('renders setup form when master password is not yet configured', async () => {
      authState.isAuthConfigured = false
      const router = await setupRouter('/login')
      const wrapper = mount(LoginView, {
        global: {
          plugins: [router],
        },
      })

      expect(wrapper.text()).toContain('Set Master Password')
      expect(wrapper.text()).toContain('Save & Unlock')
      const inputs = wrapper.findAll('input')
      expect(inputs.length).toBe(2) // password and confirm password
    })

    it('displays error when passwords do not match in setup mode', async () => {
      authState.isAuthConfigured = false
      const router = await setupRouter('/login')
      const wrapper = mount(LoginView, {
        global: {
          plugins: [router],
        },
      })

      const inputs = wrapper.findAll('input')
      await inputs[0].setValue('validpass123')
      await inputs[1].setValue('mismatch123')
      await wrapper.find('form').trigger('submit.prevent')

      expect(wrapper.text()).toContain('Passwords do not match')
    })

    it('displays error when password is too short in setup mode', async () => {
      authState.isAuthConfigured = false
      const router = await setupRouter('/login')
      const wrapper = mount(LoginView, {
        global: {
          plugins: [router],
        },
      })

      const inputs = wrapper.findAll('input')
      await inputs[0].setValue('123')
      await inputs[1].setValue('123')
      await wrapper.find('form').trigger('submit.prevent')

      expect(wrapper.text()).toContain('Password must be at least 6 characters')
    })

    it('logs in successfully and redirects to library', async () => {
      authState.isAuthConfigured = true
      const router = await setupRouter('/login')

      globalThis.fetch = (async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/api/auth/login')) {
          return {
            ok: true,
            json: async () => ({ token: 'mock-valid-token' }),
          } as unknown as Response
        }
        if (url.includes('/api/settings')) {
          return {
            ok: true,
            json: async () => ({ theme: 'dark', view_mode: 'card' }),
          } as unknown as Response
        }
        return { ok: true, json: async () => ({}) } as unknown as Response
      }) as typeof fetch

      const wrapper = mount(LoginView, {
        global: {
          plugins: [router],
        },
      })

      await wrapper.find('input').setValue('secret123')
      await wrapper.find('form').trigger('submit.prevent')
      await flushPromises()

      expect(authState.isAuthenticated).toBe(true)
      expect(router.currentRoute.value.path).toBe('/')
    })

    it('shows error alert on invalid login attempt', async () => {
      authState.isAuthConfigured = true
      const router = await setupRouter('/login')

      globalThis.fetch = (async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/api/auth/login')) {
          return {
            ok: false,
            json: async () => ({ error: 'Invalid master password' }),
          } as unknown as Response
        }
        return { ok: true, json: async () => ({}) } as unknown as Response
      }) as typeof fetch

      const wrapper = mount(LoginView, {
        global: {
          plugins: [router],
        },
      })

      await wrapper.find('input').setValue('wrong-password')
      await wrapper.find('form').trigger('submit.prevent')
      await flushPromises()

      expect(wrapper.text()).toContain('Invalid master password')
      expect(authState.isAuthenticated).toBe(false)
    })
  })

  describe('ArchiveView.vue', () => {
    const origGet = axios.get
    const sampleArchivedArticles = [
      {
        ID: 101,
        title: 'Archived Distributed Consensus Note',
        article: '/articles/archived-note.md',
        tags: 'raft, paxos',
        reading_status: 'finished',
        reading_progress: 100,
        word_count: 500,
      },
    ]

    beforeEach(() => {
      axios.get = (async (url: string) => {
        if (url.includes('/api/getarticles?archived=true')) {
          return { data: sampleArchivedArticles }
        }
        return { data: [] }
      }) as unknown as typeof axios.get
    })

    afterEach(() => {
      axios.get = origGet
    })

    it('renders archived articles list and header count', async () => {
      const router = await setupRouter('/archive')
      const wrapper = mount(ArchiveView, {
        global: {
          plugins: [router],
          stubs: {
            ArticleProgressLabel: true,
            MocProgressLabel: true,
          },
        },
      })

      await flushPromises()

      expect(wrapper.text()).toContain('Archive')
      expect(wrapper.text()).toContain('Archived Distributed Consensus Note')
    })

    it('renders empty archive state when there are no archived articles', async () => {
      axios.get = (async () => ({ data: [] })) as unknown as typeof axios.get

      const router = await setupRouter('/archive')
      const wrapper = mount(ArchiveView, {
        global: {
          plugins: [router],
          stubs: {
            ArticleProgressLabel: true,
            MocProgressLabel: true,
          },
        },
      })

      await flushPromises()

      expect(wrapper.text()).toContain('No archived articles')
    })
  })
})
