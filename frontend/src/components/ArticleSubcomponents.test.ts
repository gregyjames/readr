import { describe, it, expect } from 'bun:test'
import { mount } from '@vue/test-utils'
import ArticleProperties from './ArticleProperties.vue'
import ArticleResumeToast from './ArticleResumeToast.vue'
import ArticleBacklinks from './ArticleBacklinks.vue'
import ArticleInlineLinker from './ArticleInlineLinker.vue'
import ArticleProgressLabel from './ArticleProgressLabel.vue'

describe('Article Subcomponents', () => {
  describe('ArticleProperties.vue', () => {
    it('renders reading time and word count correctly', () => {
      const wrapper = mount(ArticleProperties, {
        props: {
          properties: {},
          readingTime: '5 min read',
          wordCount: 1250,
        },
      })

      expect(wrapper.text()).toContain('5 min read')
      expect(wrapper.text()).toContain('1250 words')
      expect(wrapper.find('a').exists()).toBe(false)
    })

    it('renders tags and original source when provided', () => {
      const wrapper = mount(ArticleProperties, {
        props: {
          properties: {
            date: '2026-09-21',
            tags: ['kubernetes', 'devops'],
            source: 'https://example.com/k8s-guide',
          },
          readingTime: '3 min read',
          wordCount: 750,
        },
      })

      expect(wrapper.text()).toContain('#kubernetes')
      expect(wrapper.text()).toContain('#devops')
      const link = wrapper.find('a')
      expect(link.exists()).toBe(true)
      expect(link.attributes('href')).toBe('https://example.com/k8s-guide')
      expect(link.attributes('target')).toBe('_blank')
    })
  })

  describe('ArticleResumeToast.vue', () => {
    it('renders reading progress when show is true', () => {
      const wrapper = mount(ArticleResumeToast, {
        props: {
          show: true,
          percent: 68,
        },
        global: {
          stubs: {
            ArticleStatusRing: true,
          },
        },
      })

      expect(wrapper.text()).toContain('Resumed at 68%')
      expect(wrapper.find('button').exists()).toBe(true)
    })

    it('emits backToTop when clicking back to top button', async () => {
      const wrapper = mount(ArticleResumeToast, {
        props: {
          show: true,
          percent: 50,
        },
        global: {
          stubs: {
            ArticleStatusRing: true,
          },
        },
      })

      const buttons = wrapper.findAll('button')
      await buttons[0].trigger('click')

      expect(wrapper.emitted('backToTop')).toBeDefined()
      expect(wrapper.emitted('backToTop')?.length).toBe(1)
    })

    it('emits dismiss when clicking close button', async () => {
      const wrapper = mount(ArticleResumeToast, {
        props: {
          show: true,
          percent: 50,
        },
        global: {
          stubs: {
            ArticleStatusRing: true,
          },
        },
      })

      const buttons = wrapper.findAll('button')
      await buttons[1].trigger('click')

      expect(wrapper.emitted('dismiss')).toBeDefined()
      expect(wrapper.emitted('dismiss')?.length).toBe(1)
    })

    it('does not render content when show is false', () => {
      const wrapper = mount(ArticleResumeToast, {
        props: {
          show: false,
          percent: 40,
        },
        global: {
          stubs: {
            ArticleStatusRing: true,
          },
        },
      })

      expect(wrapper.find('div[role="status"]').exists()).toBe(false)
    })
  })

  describe('ArticleBacklinks.vue', () => {
    it('displays empty state when backlinks are empty', () => {
      const wrapper = mount(ArticleBacklinks, {
        props: {
          backlinks: [],
        },
        global: {
          stubs: {
            RouterLink: true,
          },
        },
      })

      expect(wrapper.text()).toContain('No incoming backlinks')
      expect(wrapper.text()).toContain('0')
    })

    it('renders list of backlinks with target routes', () => {
      const wrapper = mount(ArticleBacklinks, {
        props: {
          backlinks: [
            { ID: 10, title: 'Note A' },
            { ID: 25, title: 'Note B' },
          ],
        },
        global: {
          stubs: {
            RouterLink: {
              template: '<a :data-id="to" class="backlink-stub"><slot /></a>',
              props: ['to'],
            },
          },
        },
      })

      expect(wrapper.text()).toContain('2')
      expect(wrapper.text()).toContain('Note A')
      expect(wrapper.text()).toContain('Note B')
    })
  })

  describe('ArticleInlineLinker.vue', () => {
    const sampleArticles = [
      { ID: 1, title: 'Intro to Distributed Systems' },
      { ID: 2, title: 'Kubernetes Architecture' },
      { ID: 3, title: 'Database Internals' },
    ]

    it('does not render popup when show is false', () => {
      const wrapper = mount(ArticleInlineLinker, {
        props: {
          show: false,
          pos: { top: 100, left: 200 },
          articles: sampleArticles,
        },
      })

      expect(wrapper.find('.linker-popup').exists()).toBe(false)
    })

    it('renders and excludes the current article from candidates', () => {
      const wrapper = mount(ArticleInlineLinker, {
        props: {
          show: true,
          pos: { top: 100, left: 200 },
          articles: sampleArticles,
          currentId: 2,
        },
      })

      expect(wrapper.find('.linker-popup').exists()).toBe(true)
      expect(wrapper.text()).toContain('Intro to Distributed Systems')
      expect(wrapper.text()).toContain('Database Internals')
      // Excluded currentId 2
      expect(wrapper.text()).not.toContain('Kubernetes Architecture')
    })

    it('filters candidate articles on search query and emits link on selection', async () => {
      const wrapper = mount(ArticleInlineLinker, {
        props: {
          show: true,
          pos: { top: 100, left: 200 },
          articles: sampleArticles,
        },
      })

      const input = wrapper.find('input')
      await input.setValue('distributed')

      const buttons = wrapper.findAll('button')
      expect(buttons.length).toBe(1)
      expect(buttons[0].text()).toBe('Intro to Distributed Systems')

      await buttons[0].trigger('click')
      expect(wrapper.emitted('link')).toEqual([[1]])
    })

    it('displays message when query has no matches', async () => {
      const wrapper = mount(ArticleInlineLinker, {
        props: {
          show: true,
          pos: { top: 100, left: 200 },
          articles: sampleArticles,
        },
      })

      const input = wrapper.find('input')
      await input.setValue('nonexistent topic')

      expect(wrapper.text()).toContain('No matching articles')
    })
  })

  describe('ArticleProgressLabel.vue', () => {
    it('shows "New!" when article is not started', () => {
      const wrapper = mount(ArticleProgressLabel, {
        props: {
          status: 'not_started',
          progress: 0,
        },
        global: {
          stubs: {
            ArticleStatusRing: true,
          },
        },
      })

      expect(wrapper.text()).toContain('New!')
    })

    it('shows percentage when article is in progress', () => {
      const wrapper = mount(ArticleProgressLabel, {
        props: {
          status: 'not_finished',
          progress: 45.6,
        },
        global: {
          stubs: {
            ArticleStatusRing: true,
          },
        },
      })

      expect(wrapper.text()).toContain('46%')
    })

    it('shows "Read" when article is finished', () => {
      const wrapper = mount(ArticleProgressLabel, {
        props: {
          status: 'finished',
          progress: 100,
        },
        global: {
          stubs: {
            ArticleStatusRing: true,
          },
        },
      })

      expect(wrapper.text()).toContain('Read')
    })
  })
})
