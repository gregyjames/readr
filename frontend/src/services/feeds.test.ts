import { describe, it, expect, beforeEach, afterEach } from 'bun:test'
import { feedsAPI } from './feeds'

describe('feedsAPI service', () => {
  const origFetch = globalThis.fetch

  beforeEach(() => {
    // reset fetch
  })

  afterEach(() => {
    globalThis.fetch = origFetch
  })

  it('getFeeds returns list of rss feeds', async () => {
    const mockFeeds = [
      {
        id: 1,
        url: 'https://example.com/rss.xml',
        title: 'Example Feed',
        siteUrl: 'https://example.com',
        createdAt: '2026-09-27T00:00:00Z',
      },
    ]

    globalThis.fetch = (async (url: string) => {
      expect(url).toBe('/api/feeds')
      return {
        ok: true,
        json: async () => mockFeeds,
      } as Response
    }) as unknown as typeof fetch

    const result = await feedsAPI.getFeeds()
    expect(result).toEqual(mockFeeds)
  })

  it('getFeeds throws error on failed response', async () => {
    globalThis.fetch = (async () => {
      return {
        ok: false,
        statusText: 'Internal Server Error',
        json: async () => ({ error: 'Database unavailable' }),
      } as Response
    }) as unknown as typeof fetch

    expect(feedsAPI.getFeeds()).rejects.toThrow('Database unavailable')
  })

  it('addFeed sends POST with feed url and returns created feed', async () => {
    const newFeed = {
      id: 2,
      url: 'https://news.ycombinator.com/rss',
      title: 'Hacker News',
      siteUrl: 'https://news.ycombinator.com',
      createdAt: '2026-09-27T00:00:00Z',
    }

    let requestBody = ''
    let requestMethod = ''

    globalThis.fetch = (async (url: string, init?: RequestInit) => {
      expect(url).toBe('/api/feeds')
      requestMethod = init?.method || ''
      requestBody = (init?.body as string) || ''
      return {
        ok: true,
        json: async () => newFeed,
      } as Response
    }) as unknown as typeof fetch

    const result = await feedsAPI.addFeed('https://news.ycombinator.com/rss')
    expect(requestMethod).toBe('POST')
    expect(JSON.parse(requestBody)).toEqual({ url: 'https://news.ycombinator.com/rss' })
    expect(result).toEqual(newFeed)
  })

  it('addFeed throws on error with server message', async () => {
    globalThis.fetch = (async () => {
      return {
        ok: false,
        statusText: 'Conflict',
        json: async () => ({ error: 'feed already exists' }),
      } as Response
    }) as unknown as typeof fetch

    expect(feedsAPI.addFeed('https://news.ycombinator.com/rss')).rejects.toThrow('feed already exists')
  })

  it('removeFeed sends DELETE request to /api/feeds/:id', async () => {
    let calledUrl = ''
    let calledMethod = ''

    globalThis.fetch = (async (url: string, init?: RequestInit) => {
      calledUrl = url
      calledMethod = init?.method || ''
      return {
        ok: true,
        json: async () => ({ status: 'success' }),
      } as Response
    }) as unknown as typeof fetch

    const res = await feedsAPI.removeFeed(42)
    expect(calledUrl).toBe('/api/feeds/42')
    expect(calledMethod).toBe('DELETE')
    expect(res).toEqual({ status: 'success' })
  })

  it('removeFeed throws on error', async () => {
    globalThis.fetch = (async () => {
      return {
        ok: false,
        statusText: 'Not Found',
        json: async () => ({ error: 'Feed not found' }),
      } as Response
    }) as unknown as typeof fetch

    expect(feedsAPI.removeFeed(999)).rejects.toThrow('Feed not found')
  })

  it('getTimeline fetches all timeline items when feedId is omitted', async () => {
    const mockTimeline = [
      {
        feedId: 1,
        feedTitle: 'Feed 1',
        title: 'Article 1',
        url: 'https://example.com/1',
        description: 'Desc 1',
        published: '2026-09-27T00:00:00Z',
      },
    ]

    globalThis.fetch = (async (url: string) => {
      expect(url).toBe('/api/feeds/timeline')
      return {
        ok: true,
        json: async () => mockTimeline,
      } as Response
    }) as unknown as typeof fetch

    const result = await feedsAPI.getTimeline()
    expect(result).toEqual(mockTimeline)
  })

  it('getTimeline appends feed_id query param when specified', async () => {
    const mockTimeline = [
      {
        feedId: 5,
        feedTitle: 'Specific Feed',
        title: 'Article 5',
        url: 'https://example.com/5',
        description: 'Desc 5',
        published: '2026-09-27T00:00:00Z',
      },
    ]

    globalThis.fetch = (async (url: string) => {
      expect(url).toBe('/api/feeds/timeline?feed_id=5')
      return {
        ok: true,
        json: async () => mockTimeline,
      } as Response
    }) as unknown as typeof fetch

    const result = await feedsAPI.getTimeline(5)
    expect(result).toEqual(mockTimeline)
  })

  it('getTimeline throws on server error', async () => {
    globalThis.fetch = (async () => {
      return {
        ok: false,
        statusText: 'Internal Server Error',
        json: async () => ({ error: 'Failed to retrieve feed' }),
      } as Response
    }) as unknown as typeof fetch

    expect(feedsAPI.getTimeline(999)).rejects.toThrow('Failed to retrieve feed')
  })
})
