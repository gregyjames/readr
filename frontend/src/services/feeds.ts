import { getStoredToken } from '../store/auth'

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

export interface IngestResponse {
  status?: string
  message?: string
  id?: number
  [key: string]: any
}

function getAuthHeaders(extraHeaders: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = { ...extraHeaders }
  const token = getStoredToken()
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  return headers
}

export const feedsAPI = {
  async getFeeds(): Promise<RssFeed[]> {
    const res = await fetch('/api/feeds', {
      headers: getAuthHeaders(),
    })
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
      headers: getAuthHeaders({
        'Content-Type': 'application/json',
      }),
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

  async removeFeed(id: number): Promise<{ status?: string; success?: boolean }> {
    const res = await fetch(`/api/feeds/${id}`, {
      method: 'DELETE',
      headers: getAuthHeaders(),
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

  async getTimeline(feedId?: number, refresh?: boolean): Promise<TimelineItem[]> {
    const params = new URLSearchParams()
    if (feedId) params.append('feed_id', feedId.toString())
    if (refresh) params.append('refresh', 'true')
    const qs = params.toString() ? `?${params.toString()}` : ''
    const endpoint = `/api/feeds/timeline${qs}`
    const res = await fetch(endpoint, {
      headers: getAuthHeaders(),
    })
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

export const ingestAPI = {
  async ingestUrl(url: string, tags: string[] = [], template?: string): Promise<IngestResponse> {
    const res = await fetch('/api/add', {
      method: 'POST',
      headers: getAuthHeaders({
        'Content-Type': 'application/json',
      }),
      body: JSON.stringify({ url, tags, template }),
    })
    if (!res.ok) {
      let errMsg = `Failed to ingest URL: ${res.statusText}`
      try {
        const data = await res.json()
        if (data.error) errMsg = data.error
        else if (data.message) errMsg = data.message
      } catch {
        try {
          const text = await res.text()
          if (text) errMsg = text
        } catch {}
      }
      throw new Error(errMsg)
    }
    return res.json()
  },
}
