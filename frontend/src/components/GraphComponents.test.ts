import { describe, it, expect } from 'bun:test'
import { mount } from '@vue/test-utils'
import GraphZoomControls from './GraphZoomControls.vue'
import ArticleHoverPreview from './ArticleHoverPreview.vue'

describe('Graph UI Components', () => {
  describe('GraphZoomControls.vue', () => {
    it('renders zoom in, zoom out, and fit buttons', () => {
      const wrapper = mount(GraphZoomControls)
      const buttons = wrapper.findAll('button')
      expect(buttons.length).toBe(3)
      expect(buttons[0].attributes('title')).toBe('Zoom In')
      expect(buttons[1].attributes('title')).toBe('Zoom Out')
      expect(buttons[2].attributes('title')).toBe('Fit View')
    })

    it('emits zoom-in when + is clicked', async () => {
      const wrapper = mount(GraphZoomControls)
      const buttons = wrapper.findAll('button')
      await buttons[0].trigger('click')
      expect(wrapper.emitted('zoom-in')).toBeDefined()
      expect(wrapper.emitted('zoom-in')?.length).toBe(1)
    })

    it('emits zoom-out when - is clicked', async () => {
      const wrapper = mount(GraphZoomControls)
      const buttons = wrapper.findAll('button')
      await buttons[1].trigger('click')
      expect(wrapper.emitted('zoom-out')).toBeDefined()
      expect(wrapper.emitted('zoom-out')?.length).toBe(1)
    })

    it('emits fit when fit view icon is clicked', async () => {
      const wrapper = mount(GraphZoomControls)
      const buttons = wrapper.findAll('button')
      await buttons[2].trigger('click')
      expect(wrapper.emitted('fit')).toBeDefined()
      expect(wrapper.emitted('fit')?.length).toBe(1)
    })
  })

  describe('ArticleHoverPreview.vue', () => {
    const sampleArticle = {
      id: 1,
      title: 'Advanced Raft Consensus',
      tags: 'raft, distributed, consensus',
      image: 'https://example.com/raft.png',
    }

    it('does not render preview when show is false', () => {
      const wrapper = mount(ArticleHoverPreview, {
        props: {
          show: false,
          article: sampleArticle,
          pos: { top: 100, left: 150 },
        },
      })
      expect(wrapper.find('.preview-card').exists()).toBe(false)
    })

    it('renders article preview with image and tags when show is true', () => {
      const wrapper = mount(ArticleHoverPreview, {
        props: {
          show: true,
          article: sampleArticle,
          pos: { top: 100, left: 150 },
        },
      })

      expect(wrapper.find('.preview-card').exists()).toBe(true)
      expect(wrapper.text()).toContain('Advanced Raft Consensus')
      expect(wrapper.find('img').attributes('src')).toBe('https://example.com/raft.png')
      expect(wrapper.text()).toContain('raft')
      expect(wrapper.text()).toContain('distributed')
      expect(wrapper.text()).toContain('consensus')
    })

    it('renders fallback label when article has no tags', () => {
      const wrapper = mount(ArticleHoverPreview, {
        props: {
          show: true,
          article: {
            id: 2,
            title: 'Note Without Tags',
          },
          pos: { top: 50, left: 50 },
        },
      })

      expect(wrapper.text()).toContain('Note Without Tags')
      expect(wrapper.text()).toContain('Click to read')
      expect(wrapper.find('img').exists()).toBe(false)
    })

    it('emits mouseenter and mouseleave on hover interaction', async () => {
      const wrapper = mount(ArticleHoverPreview, {
        props: {
          show: true,
          article: sampleArticle,
          pos: { top: 100, left: 150 },
        },
      })

      const card = wrapper.find('.preview-card')
      await card.trigger('mouseenter')
      expect(wrapper.emitted('mouseenter')).toBeDefined()

      await card.trigger('mouseleave')
      expect(wrapper.emitted('mouseleave')).toBeDefined()
    })
  })
})
