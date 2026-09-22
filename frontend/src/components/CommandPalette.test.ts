import { describe, it, expect, beforeEach, vi } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import { nextTick } from 'vue'
import CommandPalette from './CommandPalette.vue'
import { authState } from '../store/auth'
import emitter from '../event-bus'
import axios from 'axios'

describe('CommandPalette.vue', () => {
  const setupTest = async (initialPath = '/', isAuthenticated = true) => {
    authState.isAuthenticated = isAuthenticated
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div>Home</div>' } },
        { path: '/graph', component: { template: '<div>Graph</div>' } },
        { path: '/chat', component: { template: '<div>Chat</div>' } },
        { path: '/archive', component: { template: '<div>Archive</div>' } },
        { path: '/settings', component: { template: '<div>Settings</div>' } },
        { path: '/articles/:id', component: { template: '<div>Article</div>' } },
        { path: '/login', component: { template: '<div>Login</div>' } },
      ],
    })

    await router.push(initialPath)
    await router.isReady()

    const wrapper = mount(CommandPalette, {
      global: {
        plugins: [router],
      },
    })

    return { wrapper, router }
  }

  beforeEach(() => {
    emitter.all.clear()
  })

  it('is closed by default and does not render search modal', async () => {
    const { wrapper } = await setupTest()
    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('does not open when user is unauthenticated', async () => {
    const { wrapper } = await setupTest('/', false)
    emitter.emit('open-search')
    await nextTick()

    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('does not open when current route is /login', async () => {
    const { wrapper } = await setupTest('/login', true)
    emitter.emit('open-search')
    await nextTick()

    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('opens on open-search event when authenticated', async () => {
    const { wrapper } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    expect(wrapper.find('input').exists()).toBe(true)
    expect(wrapper.text()).toContain('Navigation')
    expect(wrapper.text()).toContain('Go to Library')
    expect(wrapper.text()).toContain('Go to Knowledge Graph')
    expect(wrapper.text()).toContain('Go to AI Chat')
  })

  it('closes when escape key is pressed', async () => {
    const { wrapper } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    const input = wrapper.find('input')
    expect(input.exists()).toBe(true)

    await input.trigger('keydown.esc')
    await flushPromises()
    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('closes when clicking the outer backdrop', async () => {
    const { wrapper } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    const backdrop = wrapper.find('.fixed.inset-0')
    expect(backdrop.exists()).toBe(true)

    await backdrop.trigger('click')
    await nextTick()

    expect(wrapper.find('input').exists()).toBe(false)
  })

  it('filters navigation commands based on query', async () => {
    const { wrapper } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    const input = wrapper.find('input')
    await input.setValue('graph')
    await nextTick()

    expect(wrapper.text()).toContain('Go to Knowledge Graph')
    expect(wrapper.text()).not.toContain('Go to Library')
    expect(wrapper.text()).not.toContain('Go to AI Chat')
  })

  it('displays empty state when query matches nothing', async () => {
    const { wrapper } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    const input = wrapper.find('input')
    await input.setValue('xyz123nonexistent')
    await nextTick()

    expect(wrapper.text()).toContain('No matching articles or commands found.')
  })

  it('executes command on click and closes the palette', async () => {
    const { wrapper, router } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    const buttons = wrapper.findAll('button')
    // Command 1 is "Go to Knowledge Graph" (/graph)
    await buttons[1].trigger('click')
    await flushPromises()

    expect(wrapper.find('input').exists()).toBe(false)
    expect(router.currentRoute.value.path).toBe('/graph')
  })

  it('navigates through commands with arrow keys and executes with Enter', async () => {
    const { wrapper, router } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    const input = wrapper.find('input')

    // Press Down arrow to select the second command (Go to Knowledge Graph)
    await input.trigger('keydown.down')
    await nextTick()

    // Press Enter to execute the selected command
    await input.trigger('keydown.enter')
    await flushPromises()

    expect(wrapper.find('input').exists()).toBe(false)
    expect(router.currentRoute.value.path).toBe('/graph')
  })

  it('renders search results from API and sanitizes excerpt HTML', async () => {
    const { wrapper } = await setupTest('/', true)
    emitter.emit('open-search')
    await nextTick()

    const axiosGet = axios.get
    axios.get = (async () => {
      return {
        data: [
          {
            id: 42,
            title: 'Kubernetes Cluster Architecture',
            excerpt: 'Overview of <mark>Kubernetes</mark> <script>alert(1)</script> components.',
          },
        ],
      }
    }) as unknown as typeof axios.get

    vi.useFakeTimers()
    try {
      const input = wrapper.find('input')
      await input.setValue('kube')
      vi.advanceTimersByTime(350)
      await flushPromises()

      expect(wrapper.text()).toContain('Kubernetes Cluster Architecture')
      expect(wrapper.text()).toContain('Kubernetes')
      expect(wrapper.html()).not.toContain('<script>')
    } finally {
      vi.useRealTimers()
      axios.get = axiosGet
    }
  })
})
