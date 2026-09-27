import axios, { type AxiosRequestConfig } from 'axios'
import type { MocProgress } from '../utils/moc'
import { getStoredToken } from '../store/auth'

export interface ArticleItem {
  ID: number
  title: string
  article: string
  image: string
  tags: string
  parsedTags: string[]
  reading_status?: string
  reading_progress?: number
  reading_time?: string
  word_count?: number
  moc_progress?: MocProgress | null
  is_archived?: boolean
}

export interface GetArticlesParams {
  page?: number
  limit?: number
  sort?: 'latest' | 'oldest' | 'title'
  tag?: string
  moc_only?: boolean
  topic?: string
  archived?: boolean
  all?: boolean
}

export interface ArticlesPaginatedResponse {
  data: ArticleItem[]
  page: number
  limit: number
  total: number
  total_pages: number
  total_notes: number
  total_mocs: number
}

function getAuthHeaders(extraHeaders: Record<string, string> = {}): Record<string, string> {
  const headers: Record<string, string> = { ...extraHeaders }
  const token = getStoredToken()
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  return headers
}

function parseArticleTags(tags: string | string[] | undefined | null): string[] {
  if (!tags) return []
  if (Array.isArray(tags)) return tags
  return tags
    .split(',')
    .map(t => t.trim())
    .filter(Boolean)
}

function formatArticleItem(raw: any): ArticleItem {
  return {
    ...raw,
    parsedTags: parseArticleTags(raw.tags),
  }
}

export const articlesAPI = {
  async getArticles(params?: GetArticlesParams, signal?: AbortSignal): Promise<ArticlesPaginatedResponse> {
    const queryParams: Record<string, any> = {}
    if (params) {
      if (params.page !== undefined) queryParams.page = params.page
      if (params.limit !== undefined) queryParams.limit = params.limit
      if (params.sort !== undefined) queryParams.sort = params.sort
      if (params.tag !== undefined) queryParams.tag = params.tag
      if (params.moc_only !== undefined) queryParams.moc_only = params.moc_only
      if (params.topic !== undefined) queryParams.topic = params.topic
      if (params.archived !== undefined) queryParams.archived = params.archived
      if (params.all !== undefined) queryParams.all = params.all
    }

    const config: AxiosRequestConfig = {
      params: queryParams,
      headers: getAuthHeaders(),
      signal,
    }

    const res = await axios.get('/api/getarticles', config)
    const rawData = res.data

    if (Array.isArray(rawData)) {
      const items = rawData.map(formatArticleItem)
      return {
        data: items,
        page: 1,
        limit: items.length || 1,
        total: items.length,
        total_pages: 1,
        total_notes: items.length,
        total_mocs: 0,
      }
    }

    const rawList: any[] = Array.isArray(rawData?.data) ? rawData.data : []
    const items = rawList.map(formatArticleItem)

    return {
      data: items,
      page: Number(rawData?.page) || 1,
      limit: Number(rawData?.limit) || (items.length || 25),
      total: Number(rawData?.total) || items.length,
      total_pages: Number(rawData?.total_pages) || 1,
      total_notes: Number(rawData?.total_notes) ?? items.length,
      total_mocs: Number(rawData?.total_mocs) ?? 0,
    }
  },

  async archiveArticle(id: number): Promise<void> {
    await axios.post(`/api/articles/${id}/archive`, null, {
      headers: getAuthHeaders(),
    })
  },

  async unarchiveArticle(id: number): Promise<void> {
    await axios.post(`/api/articles/${id}/unarchive`, null, {
      headers: getAuthHeaders(),
    })
  },

  async deleteArticle(id: number): Promise<void> {
    await axios.delete(`/api/delete/${id}`, {
      headers: getAuthHeaders(),
    })
  },
}
