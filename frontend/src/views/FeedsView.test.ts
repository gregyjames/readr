import { describe, it, expect, beforeEach, afterEach } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import FeedsView from './FeedsView.vue'
import { feedsAPI, ingestAPI, type RssFeed, type TimelineItem } from '../services/feeds'

describe('FeedsView.vue', () => {
  const origGetFeeds = feedsAPI.getFeeds
  const origGetTimeline = feedsAPI.getTimeline
  const origAddFeed = feedsAPI.addFeed
  const origRemoveFeed = feedsAPI.removeFeed
  const origIngestUrl = ingestAPI.ingestUrl

  const mockFeeds: RssFeed[] = [
    {
      id: 1,
      url: 'https://news.ycombinator.com/rss',
      title: 'Hacker News',
      siteUrl: 'https://news.ycombinator.com',
      createdAt: '2026-09-20T00:00:00Z',
    },
    {
      id: 2,
      url: 'https://lobste.rs/rss',
      title: 'Lobsters',
      siteUrl: 'https://lobste.rs',
      createdAt: '2026-09-21T00:00:00Z',
    },
  ]

  const mockTimelineAll: TimelineItem[] = [
    {
      feedId: 1,
      feedTitle: 'Hacker News',
      title: 'Show HN: Readr - Self-hosted reader',
      url: 'https://example.com/readr-launch',
      description: 'An open-source self-hosted reader and research vault.',
      published: '2026-09-27T04:00:00Z',
    },
    {
      feedId: 2,
      feedTitle: 'Lobsters',
      title: 'SQLite in Production: WAL and Concurrency',
      url: 'https://example.com/sqlite-wal',
      description: 'Understanding SQLite concurrency modes for multi-threaded apps.',
      published: '2026-09-27T03:00:00Z',
    },
  ]

  const mockTimelineFiltered: TimelineItem[] = [
    {
      feedId: 1,
      feedTitle: 'Hacker News',
      title: 'Show HN: Readr - Self-hosted reader',
      url: 'https://example.com/readr-launch',
      description: 'An open-source self-hosted reader and research vault.',
      published: '2026-09-27T04:00:00Z',
    },
  ]

  beforeEach(() => {
    feedsAPI.getFeeds = async () => [...mockFeeds]
    feedsAPI.getTimeline = async (feedId?: number) => {
      if (feedId === 1) return [...mockTimelineFiltered]
      return [...mockTimelineAll]
    }
    feedsAPI.addFeed = async (url: string) => ({
      id: 3,
      url,
      title: 'New Added Feed',
      siteUrl: 'https://newfeed.com',
      createdAt: '2026-09-27T05:00:00Z',
    })
    feedsAPI.removeFeed = async (_id: number) => ({ status: 'success', success: true })
    ingestAPI.ingestUrl = async (_url: string) => ({ status: 'success', id: 42 })
  })

  afterEach(() => {
    feedsAPI.getFeeds = origGetFeeds
    feedsAPI.getTimeline = origGetTimeline
    feedsAPI.addFeed = origAddFeed
    feedsAPI.removeFeed = origRemoveFeed
    ingestAPI.ingestUrl = origIngestUrl
  })

  it('renders feeds list and timeline items on initial load', async () => {
    const wrapper = mount(FeedsView)
    await flushPromises()

    // Verify sidebar feeds
    expect(wrapper.text()).toContain('Feeds')
    expect(wrapper.text()).toContain('Hacker News')
    expect(wrapper.text()).toContain('Lobsters')
    expect(wrapper.text()).toContain('All Feeds')

    // Verify timeline items
    const cards = wrapper.findAll('[data-testid="timeline-card"]')
    expect(cards.length).toBe(2)
    expect(wrapper.text()).toContain('Show HN: Readr - Self-hosted reader')
    expect(wrapper.text()).toContain('SQLite in Production: WAL and Concurrency')
    expect(wrapper.text()).toContain('An open-source self-hosted reader and research vault.')
    expect(wrapper.text()).toContain('Save to Vault')

    wrapper.unmount()
  })

  it('clicking a feed filters the timeline', async () => {
    let requestedFeedId: number | undefined = -1

    feedsAPI.getTimeline = async (feedId?: number) => {
      requestedFeedId = feedId
      if (feedId === 1) return [...mockTimelineFiltered]
      return [...mockTimelineAll]
    }

    const wrapper = mount(FeedsView)
    await flushPromises()

    // Click on feed item 1 (Hacker News)
    const feedItem1 = wrapper.find('[data-testid="feed-item-1"]')
    expect(feedItem1.exists()).toBe(true)
    await feedItem1.trigger('click')
    await flushPromises()

    expect(requestedFeedId).toBe(1)
    const cards = wrapper.findAll('[data-testid="timeline-card"]')
    expect(cards.length).toBe(1)
    expect(wrapper.text()).toContain('Show HN: Readr - Self-hosted reader')
    expect(wrapper.text()).not.toContain('SQLite in Production: WAL and Concurrency')

    // Click "All Feeds" to reset filter
    const allFeedsBtn = wrapper.find('[data-testid="all-feeds-btn"]')
    await allFeedsBtn.trigger('click')
    await flushPromises()

    expect(requestedFeedId).toBeUndefined()
    const allCards = wrapper.findAll('[data-testid="timeline-card"]')
    expect(allCards.length).toBe(2)

    wrapper.unmount()
  })

  it('adding a feed calls addFeed and updates the list', async () => {
    let addedUrl = ''
    feedsAPI.addFeed = async (url: string) => {
      addedUrl = url
      return {
        id: 3,
        url,
        title: 'New Added Feed',
        siteUrl: 'https://newfeed.com',
        createdAt: '2026-09-27T05:00:00Z',
      }
    }

    const wrapper = mount(FeedsView)
    await flushPromises()

    const input = wrapper.find('input[type="url"]')
    expect(input.exists()).toBe(true)
    await input.setValue('https://newfeed.com/rss')

    const form = wrapper.find('form')
    await form.trigger('submit.prevent')
    await flushPromises()

    expect(addedUrl).toBe('https://newfeed.com/rss')
    // After adding, input should be cleared
    expect((input.element as HTMLInputElement).value).toBe('')

    wrapper.unmount()
  })

  it('displays error when addFeed rejects', async () => {
    feedsAPI.addFeed = async () => {
      throw new Error('Invalid feed: not a valid RSS/Atom feed')
    }

    const wrapper = mount(FeedsView)
    await flushPromises()

    const input = wrapper.find('input[type="url"]')
    await input.setValue('https://invalid-site.com/not-a-feed')

    const form = wrapper.find('form')
    await form.trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.text()).toContain('Invalid feed: not a valid RSS/Atom feed')

    wrapper.unmount()
  })

  it('"Save to Vault" calls ingestAPI.ingestUrl and shows saved state', async () => {
    let ingestedUrl = ''
    ingestAPI.ingestUrl = async (url: string) => {
      ingestedUrl = url
      return { status: 'success', id: 42 }
    }

    const wrapper = mount(FeedsView)
    await flushPromises()

    const saveButtons = wrapper.findAll('[data-testid="save-to-vault-btn"]')
    expect(saveButtons.length).toBe(2)

    // Click Save on first item
    await saveButtons[0].trigger('click')
    await flushPromises()

    expect(ingestedUrl).toBe('https://example.com/readr-launch')
    expect(wrapper.text()).toContain('Saved!')

    wrapper.unmount()
  })

  it('clicking remove feed calls removeFeed', async () => {
    let removedId = -1
    feedsAPI.removeFeed = async (id: number) => {
      removedId = id
      return { status: 'success', success: true }
    }

    const wrapper = mount(FeedsView)
    await flushPromises()

    const removeBtn = wrapper.find('[data-testid="remove-feed-1"]')
    expect(removeBtn.exists()).toBe(true)

    await removeBtn.trigger('click')
    await flushPromises()

    expect(removedId).toBe(1)

    wrapper.unmount()
  })

  it('renders empty state when no feeds are subscribed', async () => {
    feedsAPI.getFeeds = async () => []
    feedsAPI.getTimeline = async () => []

    const wrapper = mount(FeedsView)
    await flushPromises()

    expect(wrapper.text()).toContain('No subscribed feeds')
    expect(wrapper.text()).toContain('No feeds subscribed yet')

    wrapper.unmount()
  })
})
