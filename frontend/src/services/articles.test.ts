import { describe, it, expect, beforeEach, afterEach, mock } from 'bun:test'
import axios from 'axios'
import { articlesAPI, type ArticleItem, type ArticlesPaginatedResponse } from './articles'

describe('articlesAPI service', () => {
  const origGet = axios.get
  const origPost = axios.post
  const origDelete = axios.delete

  beforeEach(() => {
    localStorage.clear()
  })

  afterEach(() => {
    axios.get = origGet
    axios.post = origPost
    axios.delete = origDelete
    localStorage.clear()
  })

  describe('getArticles', () => {
    it('fetches articles with default parameters', async () => {
      let calledUrl = ''
      let capturedConfig: any = null

      axios.get = (async (url: string, config: any) => {
        calledUrl = url
        capturedConfig = config
        return {
          status: 200,
          data: {
            data: [
              {
                ID: 1,
                title: 'Test Note',
                article: 'Content',
                image: '',
                tags: 'golang, tech',
              },
            ],
            page: 1,
            limit: 25,
            total: 1,
            total_pages: 1,
            total_notes: 1,
            total_mocs: 0,
          },
        }
      }) as unknown as typeof axios.get

      const response = await articlesAPI.getArticles()
      expect(calledUrl).toBe('/api/getarticles')
      expect(capturedConfig.params).toEqual({})
      expect(response.page).toBe(1)
      expect(response.limit).toBe(25)
      expect(response.total).toBe(1)
      expect(response.total_pages).toBe(1)
      expect(response.total_notes).toBe(1)
      expect(response.total_mocs).toBe(0)
      expect(response.data.length).toBe(1)
      expect(response.data[0].ID).toBe(1)
      expect(response.data[0].parsedTags).toEqual(['golang', 'tech'])
    })

    it('passes custom query parameters (page, limit, sort, tag, moc_only, topic, archived, all)', async () => {
      let capturedParams: any = null

      axios.get = (async (_url: string, config: any) => {
        capturedParams = config?.params
        return {
          status: 200,
          data: {
            data: [],
            page: 2,
            limit: 10,
            total: 15,
            total_pages: 2,
            total_notes: 10,
            total_mocs: 5,
          },
        }
      }) as unknown as typeof axios.get

      const params = {
        page: 2,
        limit: 10,
        sort: 'title' as const,
        tag: 'obsidian',
        moc_only: true,
        topic: 'tech',
        archived: false,
        all: false,
      }

      const response = await articlesAPI.getArticles(params)
      expect(capturedParams).toEqual({
        page: 2,
        limit: 10,
        sort: 'title',
        tag: 'obsidian',
        moc_only: true,
        topic: 'tech',
        archived: false,
        all: false,
      })
      expect(response.page).toBe(2)
      expect(response.limit).toBe(10)
      expect(response.total).toBe(15)
    })

    it('attaches Authorization header when auth token exists', async () => {
      localStorage.setItem('readr_token', 'test-article-token')
      let capturedHeaders: any = null

      axios.get = (async (_url: string, config: any) => {
        capturedHeaders = config?.headers
        return {
          status: 200,
          data: {
            data: [],
            page: 1,
            limit: 25,
            total: 0,
            total_pages: 1,
            total_notes: 0,
            total_mocs: 0,
          },
        }
      }) as unknown as typeof axios.get

      await articlesAPI.getArticles()
      expect(capturedHeaders['Authorization']).toBe('Bearer test-article-token')
    })

    it('forwards AbortSignal for request cancellation', async () => {
      const controller = new AbortController()
      let capturedSignal: any = null

      axios.get = (async (_url: string, config: any) => {
        capturedSignal = config?.signal
        return {
          status: 200,
          data: {
            data: [],
            page: 1,
            limit: 25,
            total: 0,
            total_pages: 1,
            total_notes: 0,
            total_mocs: 0,
          },
        }
      }) as unknown as typeof axios.get

      await articlesAPI.getArticles(undefined, controller.signal)
      expect(capturedSignal).toBe(controller.signal)
    })

    it('handles legacy flat array responses gracefully', async () => {
      axios.get = (async () => {
        return {
          status: 200,
          data: [
            {
              ID: 5,
              title: 'Legacy Note',
              article: 'Legacy Content',
              tags: 'alpha, beta',
            },
          ],
        }
      }) as unknown as typeof axios.get

      const response = await articlesAPI.getArticles()
      expect(response.data.length).toBe(1)
      expect(response.data[0].ID).toBe(5)
      expect(response.data[0].parsedTags).toEqual(['alpha', 'beta'])
      expect(response.total).toBe(1)
      expect(response.page).toBe(1)
      expect(response.limit).toBe(1)
      expect(response.total_pages).toBe(1)
    })

    it('propagates errors when request fails', async () => {
      axios.get = (async () => {
        throw new Error('Network error loading articles')
      }) as unknown as typeof axios.get

      await expect(articlesAPI.getArticles()).rejects.toThrow('Network error loading articles')
    })
  })

  describe('lifecycle methods', () => {
    it('archiveArticle sends POST to /api/articles/:id/archive with auth header', async () => {
      localStorage.setItem('readr_token', 'test-token')
      let calledUrl = ''
      let capturedBody: any = undefined
      let capturedHeaders: any = null

      axios.post = (async (url: string, data: any, config: any) => {
        calledUrl = url
        capturedBody = data
        capturedHeaders = config?.headers
        return { status: 200, data: { success: true } }
      }) as unknown as typeof axios.post

      await articlesAPI.archiveArticle(42)
      expect(calledUrl).toBe('/api/articles/42/archive')
      expect(capturedBody).toBeNull()
      expect(capturedHeaders['Authorization']).toBe('Bearer test-token')
    })

    it('unarchiveArticle sends POST to /api/articles/:id/unarchive with auth header', async () => {
      localStorage.setItem('readr_token', 'test-token')
      let calledUrl = ''
      let capturedBody: any = undefined
      let capturedHeaders: any = null

      axios.post = (async (url: string, data: any, config: any) => {
        calledUrl = url
        capturedBody = data
        capturedHeaders = config?.headers
        return { status: 200, data: { success: true } }
      }) as unknown as typeof axios.post

      await articlesAPI.unarchiveArticle(42)
      expect(calledUrl).toBe('/api/articles/42/unarchive')
      expect(capturedBody).toBeNull()
      expect(capturedHeaders['Authorization']).toBe('Bearer test-token')
    })

    it('deleteArticle sends DELETE to /api/delete/:id with auth header', async () => {
      localStorage.setItem('readr_token', 'test-token')
      let calledUrl = ''
      let capturedHeaders: any = null

      axios.delete = (async (url: string, config: any) => {
        calledUrl = url
        capturedHeaders = config?.headers
        return { status: 200, data: { success: true } }
      }) as unknown as typeof axios.delete

      await articlesAPI.deleteArticle(42)
      expect(calledUrl).toBe('/api/delete/42')
      expect(capturedHeaders['Authorization']).toBe('Bearer test-token')
    })

    it('propagates errors when lifecycle method fails', async () => {
      axios.post = (async () => {
        throw new Error('Failed to archive article')
      }) as unknown as typeof axios.post

      await expect(articlesAPI.archiveArticle(999)).rejects.toThrow('Failed to archive article')
    })
  })
})
