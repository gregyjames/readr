import { describe, it, expect, beforeEach, afterEach } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import axios from 'axios'
import App from './App.vue'
import { authState } from './store/auth'
import emitter from './event-bus'

describe('App.vue Root Component', () => {
  const setupRouter = async (initialPath = '/') => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div>Library Home</div>' } },
        { path: '/graph', component: { template: '<div>Graph</div>' } },
        { path: '/chat', component: { template: '<div>Chat</div>' } },
        { path: '/archive', component: { template: '<div>Archive</div>' } },
        { path: '/settings', component: { template: '<div>Settings</div>' } },
        { path: '/login', component: { template: '<div>Login</div>' } },
      ],
    })
    await router.push(initialPath)
    await router.isReady()
    return router
  }

  const origGet = axios.get
  const origFetch = globalThis.fetch

  beforeEach(() => {
    localStorage.clear()
    authState.isLoaded = true
    authState.isAuthenticated = true
    authState.isAuthConfigured = true

    axios.get = (async (url: string) => {
      if (url.includes('/api/templates')) {
        return { data: [{ name: 'github.com', filename: 'github.jinja' }] }
      }
      return { data: [] }
    }) as unknown as typeof axios.get

    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({}),
    })) as unknown as typeof fetch
  })

  afterEach(() => {
    axios.get = origGet
    globalThis.fetch = origFetch
  })

  it('renders navigation rail when user is authenticated', async () => {
    const router = await setupRouter('/')
    const wrapper = mount(App, {
      global: {
        plugins: [router],
        stubs: {
          CommandPalette: true,
          BookmarkIcon: true,
          HomeIcon: true,
          AddIcon: true,
          GraphIcon: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('aside').exists()).toBe(true)
    expect(wrapper.find('button[title="Ingest Article (⌘N)"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('hides navigation rail when on login route', async () => {
    const router = await setupRouter('/login')
    const wrapper = mount(App, {
      global: {
        plugins: [router],
        stubs: {
          CommandPalette: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('aside').exists()).toBe(false)
    wrapper.unmount()
  })

  it('opens and closes ingest article modal', async () => {
    const router = await setupRouter('/')
    const wrapper = mount(App, {
      global: {
        plugins: [router],
        stubs: {
          CommandPalette: true,
          BookmarkIcon: true,
          HomeIcon: true,
          AddIcon: true,
          GraphIcon: true,
        },
      },
    })

    await flushPromises()

    expect(wrapper.find('form').exists()).toBe(false)

    // Open ingest modal via event
    emitter.emit('open-add-modal')
    await flushPromises()

    expect(wrapper.find('form').exists()).toBe(true)
    expect(wrapper.text()).toContain('Add New Article')

    // Close modal via Cancel button
    const cancelBtn = wrapper.findAll('button').find(b => b.text().includes('Cancel'))
    if (cancelBtn) {
      await cancelBtn.trigger('click')
      await flushPromises()
      expect(wrapper.find('form').exists()).toBe(false)
    }

    wrapper.unmount()
  })
})
