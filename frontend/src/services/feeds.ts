export interface RssFeed {
  id: number
  url: string
  title: string
  siteUrl: string
  createdAt: string
}

export interface TimelineItem {
  feedId: number
  feedTitle: string
  title: string
  url: string
  description: string
  published: string
}

export const feedsAPI = {
  async getFeeds(): Promise<RssFeed[]> {
    const res = await fetch('/api/feeds')
    if (!res.ok) {
      let errMsg = `Failed to fetch feeds: ${res.statusText}`
      try {
        const data = await res.json()
        if (data.error) errMsg = data.error
      } catch {}
      throw new Error(errMsg)
    }
    return res.json()
  },

  async addFeed(url: string): Promise<RssFeed> {
    const res = await fetch('/api/feeds', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ url }),
    })
    if (!res.ok) {
      let errMsg = `Failed to add feed: ${res.statusText}`
      try {
        const data = await res.json()
        if (data.error) errMsg = data.error
      } catch {}
      throw new Error(errMsg)
    }
    return res.json()
  },

  async removeFeed(id: number): Promise<any> {
    const res = await fetch(`/api/feeds/${id}`, {
      method: 'DELETE',
    })
    if (!res.ok) {
      let errMsg = `Failed to remove feed: ${res.statusText}`
      try {
        const data = await res.json()
        if (data.error) errMsg = data.error
      } catch {}
      throw new Error(errMsg)
    }
    try {
      return await res.json()
    } catch {
      return { status: 'success' }
    }
  },

  async getTimeline(feedId?: number): Promise<TimelineItem[]> {
    const endpoint = feedId ? `/api/feeds/timeline?feed_id=${feedId}` : '/api/feeds/timeline'
    const res = await fetch(endpoint)
    if (!res.ok) {
      let errMsg = `Failed to fetch timeline: ${res.statusText}`
      try {
        const data = await res.json()
        if (data.error) errMsg = data.error
      } catch {}
      throw new Error(errMsg)
    }
    return res.json()
  },
}
