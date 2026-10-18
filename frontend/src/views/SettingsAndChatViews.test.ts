import { describe, it, expect, beforeEach, afterEach } from 'bun:test'
import { mount, flushPromises } from '@vue/test-utils'
import { createRouter, createMemoryHistory } from 'vue-router'
import axios from 'axios'
import SettingsView from './SettingsView.vue'
import ChatView from './ChatView.vue'
import { authState } from '../store/auth'
import { settings } from '../store/settings'

describe('Views: SettingsView and ChatView', () => {
  const setupRouter = async (initialPath = '/settings') => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/', component: { template: '<div>Library Home</div>' } },
        { path: '/settings', component: { template: '<div>Settings</div>' } },
        { path: '/chat', component: { template: '<div>Chat</div>' } },
        { path: '/chat/:id', component: { template: '<div>Chat Session</div>' } },
      ],
    })
    await router.push(initialPath)
    await router.isReady()
    return router
  }

  describe('SettingsView.vue', () => {
    const origFetch = globalThis.fetch

    beforeEach(() => {
      localStorage.clear()
      localStorage.setItem('readr_token', 'mock-valid-token')
      authState.isLoaded = true
      authState.isAuthenticated = true
      authState.isAuthConfigured = true
    })

    afterEach(() => {
      globalThis.fetch = origFetch
    })

    it('renders settings view sections and loads initial values', async () => {
      const router = await setupRouter('/settings')
      globalThis.fetch = (async (input: RequestInfo | URL) => {
        const url = String(input)
        if (url.includes('/api/keys')) {
          return {
            ok: true,
            json: async () => [
              { id: 1, name: 'Obsidian Sync Key', key_prefix: 'rdr_live_obs', created_at: '2026-09-20T00:00:00Z' },
            ],
          } as unknown as Response
        }
        if (url.includes('/api/diagnostics')) {
          return {
            ok: true,
            json: async () => ({
              queue: {
                total_in_flight: 0,
                pending_jobs: 0,
                active_jobs: 0,
                max_capacity: 100,
              },
              metrics: {
                total_runs: 42,
                success_runs: 40,
                failed_runs: 2,
                avg_latency_ms: 320,
                total_cost_usd: 0.12,
              },
            }),
          } as unknown as Response
        }
        return { ok: true, json: async () => ({}) } as unknown as Response
      }) as typeof fetch

      const wrapper = mount(SettingsView, {
        global: {
          plugins: [router],
        },
      })

      await flushPromises()

      expect(wrapper.text()).toContain('Settings')
      expect(wrapper.text()).toContain('General')
      expect(wrapper.text()).toContain('Pipeline Diagnostics')
      expect(wrapper.text()).toContain('Master Password')
      expect(wrapper.text()).toContain('Obsidian Sync Key')
      expect(wrapper.text()).toContain('rdr_live_obs')
      expect(wrapper.text()).toContain('AI Provider Integration')

      wrapper.unmount()
    })

    it('supports switching AI provider type and updating default base URL and model', async () => {
      const router = await setupRouter('/settings')
      globalThis.fetch = (async () => ({ ok: true, json: async () => ({}) })) as unknown as typeof fetch

      settings.provider_type = 'openrouter'
      settings.provider_base_url = 'https://openrouter.ai/api/v1'
      settings.model = 'openai/gpt-4o-mini'
      settings.api_key = ''

      const wrapper = mount(SettingsView, {
        global: {
          plugins: [router],
        },
      })
      await flushPromises()

      expect(wrapper.text()).toContain('Missing Key')

      const select = wrapper.find<HTMLSelectElement>('#provider-type')
      expect(select.exists()).toBe(true)

      // Change provider to ollama
      await select.setValue('ollama')
      await select.trigger('change')
      await flushPromises()

      expect(settings.provider_type).toBe('ollama')
      expect(settings.provider_base_url).toBe('http://localhost:11434/v1')
      expect(settings.model).toBe('llama3.2')
      expect(wrapper.text()).toContain('Connected')
      expect(wrapper.text()).toContain('Optional for local Ollama')

      // Test manual model input
      const manualInput = wrapper.find<HTMLInputElement>('#manual-model-input')
      expect(manualInput.exists()).toBe(true)
      await manualInput.setValue('mistral:7b')
      await flushPromises()
      expect(settings.model).toBe('mistral:7b')

      wrapper.unmount()
    })

    it('populates Default AI Model dropdown for OpenRouter using session token for Authorization', async () => {
      const router = await setupRouter('/settings')
      let capturedModelsHeaders: Record<string, string> = {}

      settings.provider_type = 'openrouter'
      settings.provider_base_url = 'https://openrouter.ai/api/v1'
      settings.model = 'openai/gpt-4o-mini'
      settings.api_key = 'sk-or-v1-secret-key'

      globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input)
        if (url.includes('/api/models')) {
          capturedModelsHeaders = (init?.headers as Record<string, string>) || {}
          return {
            ok: true,
            json: async () => ({
              data: [
                { id: 'anthropic/claude-3.5-sonnet', name: 'Anthropic: Claude 3.5 Sonnet' },
                { id: 'openai/gpt-4o-mini', name: 'OpenAI: GPT-4o Mini' },
              ],
            }),
          } as unknown as Response
        }
        return { ok: true, json: async () => ({}) } as unknown as Response
      }) as typeof fetch

      const wrapper = mount(SettingsView, {
        global: {
          plugins: [router],
        },
      })
      await flushPromises()

      // Must authenticate with Readr session token, NOT overwrite Authorization with OpenRouter API key
      expect(capturedModelsHeaders['Authorization']).toBe('Bearer mock-valid-token')
      expect(capturedModelsHeaders['X-OpenRouter-Key']).toBe('sk-or-v1-secret-key')
      expect(capturedModelsHeaders['X-Provider-Type']).toBe('openrouter')

      // Open the model dropdown and verify models are populated
      const dropdownTriggers = wrapper.findAll('button')
      const modelDropdownBtn = dropdownTriggers.find(b => b.text().includes('openai/gpt-4o-mini'))
      expect(modelDropdownBtn).toBeDefined()
      await modelDropdownBtn!.trigger('click')
      await flushPromises()

      expect(wrapper.text()).toContain('Anthropic: Claude 3.5 Sonnet')
      expect(wrapper.text()).toContain('OpenAI: GPT-4o Mini')

      wrapper.unmount()
    })
  })

  describe('ChatView.vue', () => {
    const origAxiosGet = axios.get
    let origApiKey: string | undefined
    let origProviderType: any
    const sampleSessions = [
      {
        id: 'sess-1',
        title: 'Distributed Consensus Discussion',
        created_at: '2026-09-21T10:00:00Z',
        updated_at: '2026-09-21T10:05:00Z',
      },
    ]
    const sampleMessages = [
      {
        id: 'msg-1',
        role: 'user',
        content: 'How does Raft leader election work?',
        created_at: '2026-09-21T10:00:00Z',
      },
      {
        id: 'msg-2',
        role: 'assistant',
        content: 'Raft uses randomized election timeouts to ensure a single leader emerges.',
        created_at: '2026-09-21T10:00:05Z',
      },
    ]

    beforeEach(() => {
      origApiKey = settings.api_key
      origProviderType = settings.provider_type
      settings.provider_type = 'openrouter'
      settings.api_key = 'test-mock-openrouter-key'
      axios.get = (async (url: string) => {
        if (url === '/api/chats') {
          return { data: sampleSessions }
        }
        if (url === '/api/getarticles') {
          return { data: [{ ID: 1, title: 'Raft Paper' }] }
        }
        if (url.startsWith('/api/chats/sess-1')) {
          return {
            data: {
              session: sampleSessions[0],
              messages: sampleMessages,
            },
          }
        }
        return { data: {} }
      }) as unknown as typeof axios.get
    })

    afterEach(() => {
      axios.get = origAxiosGet
      settings.api_key = origApiKey
      settings.provider_type = origProviderType
    })

    it('loads chat sessions and displays active session messages', async () => {
      const router = await setupRouter('/chat/sess-1')
      const wrapper = mount(ChatView, {
        global: {
          plugins: [router],
          stubs: {
            ChatMessageItem: {
              template: '<div class="chat-message-stub">{{ message.content }}</div>',
              props: ['message'],
            },
            MentionDropdown: true,
          },
        },
      })

      await flushPromises()

      expect(wrapper.text()).toContain('Distributed Consensus Discussion')
      expect(wrapper.text()).toContain('How does Raft leader election work?')
      expect(wrapper.text()).toContain('Raft uses randomized election timeouts to ensure a single leader emerges.')
    })

    it('shows missing key warning banner when provider requires key and none is set', async () => {
      settings.provider_type = 'openrouter'
      settings.api_key = ''
      const router = await setupRouter('/chat')
      const wrapper = mount(ChatView, {
        global: {
          plugins: [router],
          stubs: {
            ChatMessageItem: true,
            MentionDropdown: true,
          },
        },
      })
      await flushPromises()

      expect(wrapper.text()).toContain('AI Provider not configured.')
    })

    it('does not show missing key warning banner when provider is ollama even without key', async () => {
      settings.provider_type = 'ollama'
      settings.api_key = ''
      const router = await setupRouter('/chat')
      const wrapper = mount(ChatView, {
        global: {
          plugins: [router],
          stubs: {
            ChatMessageItem: true,
            MentionDropdown: true,
          },
        },
      })
      await flushPromises()

      expect(wrapper.text()).not.toContain('AI Provider not configured.')
    })
  })
})
