import { describe, it, expect } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import Graph from './Graph.vue'
import LocalGraph from './LocalGraph.vue'

describe('Graph Network Views', () => {
  const setupRouter = async (initialPath = '/') => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div>Home</div>' } },
        { path: '/graph', component: { template: '<div>Graph</div>' } },
        { path: '/articles/:id', component: { template: '<div>Article</div>' } },
      ],
    })
    await router.push(initialPath)
    await router.isReady()
    return router
  }

  describe('Graph.vue', () => {
    const origFetch = globalThis.fetch

    it('shows loading state initially and renders legend and tag toggle after successful fetch', async () => {
      const router = await setupRouter('/graph')
      globalThis.fetch = (async (input: RequestInfo | URL) => {
        if (String(input).includes('/api/graph')) {
          return {
            ok: true,
            json: async () => ({
              nodes: [
                { id: 'article-1', label: 'Raft Paper', group: 'article' },
                { id: 'moc-1', label: 'Distributed Systems', group: 'moc' },
                { id: 'tag-consensus', label: '#consensus', group: 'tag' },
              ],
              edges: [
                { from: 'article-1', to: 'moc-1' },
                { from: 'article-1', to: 'tag-consensus' },
              ],
            }),
          } as unknown as Response
        }
        return { ok: false } as unknown as Response
      }) as typeof fetch

      try {
        const wrapper = mount(Graph, {
          global: {
            plugins: [router],
            stubs: {
              GraphZoomControls: true,
            },
          },
        })

        expect(wrapper.text()).toContain('Simulating neural graph...')

        await flushPromises()

        expect(wrapper.text()).toContain('Article')
        expect(wrapper.text()).toContain('MOC Hub')
        expect(wrapper.text()).toContain('Tag')
        expect(wrapper.text()).toContain('Tags Visible')

        const toggleBtn = wrapper.find('button')
        expect(toggleBtn.exists()).toBe(true)
        await toggleBtn.trigger('click')
        await flushPromises()

        expect(wrapper.text()).toContain('Tags Hidden')
      } finally {
        globalThis.fetch = origFetch
      }
    })

    it('renders error state with retry button when API fails', async () => {
      const router = await setupRouter('/graph')
      const errSpy = console.error
      console.error = () => {}

      globalThis.fetch = (async () => {
        throw new Error('Database locked')
      }) as typeof fetch

      try {
        const wrapper = mount(Graph, {
          global: {
            plugins: [router],
            stubs: {
              GraphZoomControls: true,
            },
          },
        })

        await flushPromises()

        expect(wrapper.text()).toContain('Failed to load graph relations')
        expect(wrapper.find('button').text()).toBe('Retry')
      } finally {
        console.error = errSpy
        globalThis.fetch = origFetch
      }
    })
  })

  describe('LocalGraph.vue', () => {
    it('mounts and renders header and zoom controls container for given articleId', async () => {
      const router = await setupRouter('/articles/10')
      const wrapper = mount(LocalGraph, {
        props: {
          articleId: 10,
        },
        global: {
          plugins: [router],
          stubs: {
            GraphZoomControls: true,
          },
        },
      })

      expect(wrapper.text()).toContain('Local Network')
      expect(wrapper.text()).toContain('1-hop')
      expect(wrapper.find('.w-full.h-\\[360px\\]').exists()).toBe(true)
    })
  })
})
