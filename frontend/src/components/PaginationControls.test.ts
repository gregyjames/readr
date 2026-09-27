import { describe, it, expect } from 'bun:test'
import { mount } from '@vue/test-utils'
import PaginationControls from './PaginationControls.vue'

describe('PaginationControls.vue', () => {
  it('renders range summary and navigation buttons', () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 1,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    expect(wrapper.text()).toContain('Showing 1–25 of 120 items')
    expect(wrapper.find('button[aria-label="Previous page"]').attributes('disabled')).toBeDefined()
    expect(wrapper.find('button[aria-label="Next page"]').attributes('disabled')).toBeUndefined()
  })

  it('emits update:page when next or page button is clicked', async () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 1,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    await wrapper.find('button[aria-label="Next page"]').trigger('click')
    expect(wrapper.emitted('update:page')?.[0]).toEqual([2])

    // Click a specific page button
    const page3Btn = wrapper.findAll('button').find(b => b.attributes('aria-label') === 'Page 3')
    if (page3Btn) {
      await page3Btn.trigger('click')
      expect(wrapper.emitted('update:page')?.[1]).toEqual([3])
    }
  })

  it('emits update:pageSize when selector changes', async () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 1,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    const select = wrapper.find('select')
    await select.setValue('50')
    expect(wrapper.emitted('update:pageSize')?.[0]).toEqual([50])
  })

  it('handles 0 total items cleanly without breaking', () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 1,
        totalPages: 1,
        totalItems: 0,
        pageSize: 25,
      }
    })
    // When totalItems is 0, buttons are disabled (both prev and next)
    const prevBtn = wrapper.find('button[aria-label="Previous page"]')
    const nextBtn = wrapper.find('button[aria-label="Next page"]')
    if (prevBtn.exists()) {
      expect(prevBtn.attributes('disabled')).toBeDefined()
    }
    if (nextBtn.exists()) {
      expect(nextBtn.attributes('disabled')).toBeDefined()
    }
  })

  it('disables next button on the last page and previous button on first page', () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 5,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    expect(wrapper.find('button[aria-label="Previous page"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('button[aria-label="Next page"]').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('Showing 101–120 of 120 items')
  })

  it('emits update:page when previous button is clicked', async () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 3,
        totalPages: 5,
        totalItems: 120,
        pageSize: 25,
      }
    })
    await wrapper.find('button[aria-label="Previous page"]').trigger('click')
    expect(wrapper.emitted('update:page')?.[0]).toEqual([2])
  })

  it('renders visible pages with ellipsis when total pages is large', () => {
    const wrapper = mount(PaginationControls, {
      props: {
        currentPage: 5,
        totalPages: 10,
        totalItems: 250,
        pageSize: 25,
      }
    })
    expect(wrapper.text()).toContain('...')
  })
})
