import { describe, it, expect } from 'bun:test'
import { mount } from '@vue/test-utils'
import ChatMessageItem from './ChatMessageItem.vue'
import MentionDropdown from './MentionDropdown.vue'
import type { Message, Article } from '../../types/chat'

describe('Chat Subcomponents', () => {
  describe('ChatMessageItem.vue', () => {
    it('renders user message bubble with text and attachments', () => {
      const userMessage: Message = {
        id: 'msg-1',
        role: 'user',
        content: 'Explain the difference between Raft and Paxos.',
        created_at: '2026-09-21T12:00:00Z',
        attachments: [
          { id: 101, title: 'Raft Paper Notes' },
          { id: 102, title: 'Paxos Made Simple' },
        ],
      }

      const wrapper = mount(ChatMessageItem, {
        props: {
          message: userMessage,
        },
      })

      expect(wrapper.text()).toContain('Explain the difference between Raft and Paxos.')
      expect(wrapper.text()).toContain('📄 Raft Paper Notes')
      expect(wrapper.text()).toContain('📄 Paxos Made Simple')
      expect(wrapper.text()).not.toContain('Readr Assistant')
    })

    it('renders assistant message with sanitized markdown and code blocks', () => {
      const assistantMessage: Message = {
        id: 'msg-2',
        role: 'assistant',
        content: '### Key Differences\n\n- **Raft**: Understandable leader election\n- **Paxos**: Symmetric consensus\n\n```go\nfunc main() {}\n```',
        created_at: '2026-09-21T12:00:05Z',
      }

      const wrapper = mount(ChatMessageItem, {
        props: {
          message: assistantMessage,
        },
      })

      expect(wrapper.text()).toContain('Readr Assistant')
      expect(wrapper.text()).toContain('Key Differences')
      expect(wrapper.text()).toContain('Understandable leader election')
      expect(wrapper.find('pre code').exists()).toBe(true)
      expect(wrapper.find('.text-emerald-600').exists()).toBe(false)
    })

    it('shows streaming indicator when isStreaming and isLast are true', () => {
      const streamingMessage: Message = {
        id: 'msg-3',
        role: 'assistant',
        content: 'Generating summary...',
        created_at: '2026-09-21T12:00:10Z',
      }

      const wrapper = mount(ChatMessageItem, {
        props: {
          message: streamingMessage,
          isStreaming: true,
          isLast: true,
        },
      })

      expect(wrapper.text()).toContain('Generating response...')
      expect(wrapper.find('.animate-pulse').exists()).toBe(true)
    })
  })

  describe('MentionDropdown.vue', () => {
    const sampleArticles: Article[] = [
      { ID: 1, title: 'Distributed Systems' },
      { ID: 2, title: 'Kubernetes Networking' },
      { ID: 3, title: 'Database Sharding' },
    ]

    it('renders mention header and list of article candidates', () => {
      const wrapper = mount(MentionDropdown, {
        props: {
          articles: sampleArticles,
          selectedIndex: 0,
        },
      })

      expect(wrapper.text()).toContain('Mention an article')
      expect(wrapper.text()).toContain('Distributed Systems')
      expect(wrapper.text()).toContain('Kubernetes Networking')
      expect(wrapper.text()).toContain('Database Sharding')
    })

    it('highlights the item at selectedIndex', () => {
      const wrapper = mount(MentionDropdown, {
        props: {
          articles: sampleArticles,
          selectedIndex: 1,
        },
      })

      const rows = wrapper.findAll('.cursor-pointer')
      expect(rows.length).toBe(3)
      expect(rows[1].classes()).toContain('font-medium')
      expect(rows[0].classes()).not.toContain('font-medium')
    })

    it('emits select event with clicked article', async () => {
      const wrapper = mount(MentionDropdown, {
        props: {
          articles: sampleArticles,
          selectedIndex: 0,
        },
      })

      const rows = wrapper.findAll('.cursor-pointer')
      await rows[2].trigger('click')

      expect(wrapper.emitted('select')).toBeDefined()
      expect(wrapper.emitted('select')?.[0]).toEqual([sampleArticles[2]])
    })
  })
})
