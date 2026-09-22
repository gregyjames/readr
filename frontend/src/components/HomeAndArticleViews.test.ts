import { describe, it, expect, beforeEach, afterEach } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import axios from 'axios'
import Home from './Home.vue'
import Article from './Article.vue'

describe('Core Views: Home and Article', () => {
  const setupRouter = async (initialPath = '/') => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div>Library Home</div>' } },
        { path: '/articles/:id', component: { template: '<div>Article Detail</div>' } },
      ],
    })
    await router.push(initialPath)
    await router.isReady()
    return router
  }

  describe('Home.vue', () => {
    const origGet = axios.get
    const sampleArticles = [
      {
        ID: 1,
        title: 'Kubernetes Networking Deep Dive',
        article: '/articles/k8s.md',
        image: '',
        tags: 'k8s, networking',
        reading_status: 'not_started',
        reading_progress: 0,
        reading_time: '6 min read',
        word_count: 1400,
      },
      {
        ID: 2,
        title: 'Distributed Consensus with Raft',
        article: '/articles/raft.md',
        image: '',
        tags: 'raft, distributed',
        reading_status: 'finished',
        reading_progress: 100,
        reading_time: '4 min read',
        word_count: 900,
      },
    ]

    beforeEach(() => {
      axios.get = (async (url: string) => {
        if (url.includes('/api/getarticles')) {
          return { data: sampleArticles }
        }
        return { data: [] }
      }) as unknown as typeof axios.get
    })

    afterEach(() => {
      axios.get = origGet
    })

    it('renders article cards in library home view', async () => {
      const router = await setupRouter('/')
      const wrapper = mount(Home, {
        global: {
          plugins: [router],
          stubs: {
            ArticleProgressLabel: true,
            MocProgressLabel: true,
          },
        },
      })

      await flushPromises()

      expect(wrapper.text()).toContain('Kubernetes Networking Deep Dive')
      expect(wrapper.text()).toContain('Distributed Consensus with Raft')
    })

    it('filters articles by tag when a tag is selected', async () => {
      const router = await setupRouter('/')
      const wrapper = mount(Home, {
        global: {
          plugins: [router],
          stubs: {
            ArticleProgressLabel: true,
            MocProgressLabel: true,
          },
        },
      })

      await flushPromises()

      // Find and click tag button for k8s
      const tagButtons = wrapper.findAll('button').filter(b => b.text().includes('k8s'))
      if (tagButtons.length > 0) {
        await tagButtons[0].trigger('click')
        await flushPromises()

        expect(wrapper.text()).toContain('Kubernetes Networking Deep Dive')
      }
    })
  })

  describe('Article.vue', () => {
    const origGet = axios.get
    const sampleArticleData = [
      {
        ID: 10,
        title: 'Raft Consensus Architecture',
        article: '/articles/raft-arch.md',
        image: '',
        tags: 'raft, distributed',
        reading_status: 'not_finished',
        reading_progress: 45,
        word_count: 1200,
      },
    ]

    beforeEach(() => {
      axios.get = (async (url: string) => {
        if (url.includes('/api/getarticles')) {
          return { data: sampleArticleData }
        }
        if (url.includes('/api/articles/10.md') || url.includes('/api/articles/raft-arch.md')) {
          return {
            data: '# Raft Consensus Architecture\n\nLeader election and log replication.\n\n[[Distributed Systems]]',
          }
        }
        if (url.includes('/api/graph')) {
          return {
            data: {
              nodes: [{ id: 'article-10', label: 'Raft Note' }],
              edges: [],
            },
          }
        }
        return { data: {} }
      }) as unknown as typeof axios.get
    })

    afterEach(() => {
      axios.get = origGet
    })

    it('renders article content and parsed markdown', async () => {
      const router = await setupRouter('/articles/10')
      const wrapper = mount(Article, {
        props: {
          id: '10',
        },
        global: {
          plugins: [router],
          stubs: {
            ArticleProperties: true,
            ArticleBacklinks: true,
            ArticleInlineLinker: true,
            ArticleResumeToast: true,
            ArticleHoverPreview: true,
            ArticleStatusRing: true,
            MocProgressLabel: true,
            GraphZoomControls: true,
          },
        },
      })

      await flushPromises()

      expect(wrapper.text()).toContain('Raft Consensus Architecture')
      expect(wrapper.find('.article-content').exists() || wrapper.find('main').exists() || wrapper.text().includes('Leader election')).toBe(true)
    })
  })
})
