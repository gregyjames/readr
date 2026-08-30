<script setup lang="ts">
import { ref, computed, onMounted, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import axios from 'axios'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'

interface Article {
  ID: number
  title: string
  article?: string
  image?: string
  tags?: string
}

interface Attachment {
  id: number
  title: string
}

interface Message {
  role: 'user' | 'assistant' | 'system'
  content: string
  attachments?: Attachment[]
}

interface ChatSession {
  id: string
  title: string
  created_at: string
  updated_at: string
  messages: Message[]
}

const route = useRoute()
const router = useRouter()

const sessions = ref<ChatSession[]>([])
const currentSession = ref<ChatSession | null>(null)
const isLoadingSessions = ref(false)
const isLoadingMessages = ref(false)
const isStreaming = ref(false)
const errorMessage = ref('')

const inputContent = ref('')
const attachments = ref<Attachment[]>([])
const articles = ref<Article[]>([])
const messageContainer = ref<HTMLElement | null>(null)
const textareaRef = ref<HTMLTextAreaElement | null>(null)

// @ mention autocomplete state
const showMentionDropdown = ref(false)
const mentionQuery = ref('')
const mentionIndex = ref(0)
const selectedMentionIdx = ref(0)

const apiKey = ref('')
const hasApiKey = computed(() => Boolean(apiKey.value.trim()))

const filteredMentionArticles = computed(() => {
  const q = mentionQuery.value.toLowerCase().trim()
  const attachedIds = new Set(attachments.value.map(a => a.id))
  return articles.value
    .filter(a => !attachedIds.has(a.ID) && (!q || a.title.toLowerCase().includes(q)))
    .slice(0, 8)
})

onMounted(async () => {
  apiKey.value = localStorage.getItem('OPENROUTER_API_KEY') || ''
  await Promise.all([fetchSessions(), fetchArticles()])

  const sessionId = route.params.id as string | undefined
  if (sessionId) {
    await selectSession(sessionId)
  } else if (sessions.value.length > 0) {
    await selectSession(sessions.value[0].id)
  }
})

watch(() => route.params.id, async (newId) => {
  if (newId && typeof newId === 'string' && currentSession.value?.id !== newId) {
    await selectSession(newId)
  }
})

const fetchArticles = async () => {
  try {
    const res = await axios.get('/api/getarticles')
    articles.value = res.data || []
  } catch (err) {
    console.error('Failed to fetch articles', err)
  }
}

const fetchSessions = async () => {
  isLoadingSessions.value = true
  try {
    const res = await axios.get('/api/chats')
    sessions.value = res.data || []
  } catch (err) {
    console.error('Failed to fetch chat sessions', err)
  } finally {
    isLoadingSessions.value = false
  }
}

const selectSession = async (id: string) => {
  isLoadingMessages.value = true
  errorMessage.value = ''
  try {
    const res = await axios.get(`/api/chats/${id}`)
    currentSession.value = res.data
    if (route.params.id !== id) {
      router.push(`/chat/${id}`)
    }
    await nextTick()
    scrollToBottom()
    highlightCodeBlocks()
  } catch (err) {
    console.error('Failed to get chat session', err)
    errorMessage.value = 'Failed to load conversation.'
  } finally {
    isLoadingMessages.value = false
  }
}

const createNewChat = async () => {
  try {
    const res = await axios.post('/api/chats', { title: 'New Chat' })
    const newSession = res.data
    sessions.value.unshift(newSession)
    currentSession.value = newSession
    router.push(`/chat/${newSession.id}`)
    inputContent.value = ''
    attachments.value = []
    await nextTick()
    textareaRef.value?.focus()
  } catch (err) {
    console.error('Failed to create new chat', err)
  }
}

const deleteSession = async (id: string, e: Event) => {
  e.stopPropagation()
  if (!confirm('Are you sure you want to delete this chat?')) return

  try {
    await axios.delete(`/api/chats/${id}`)
    sessions.value = sessions.value.filter(s => s.id !== id)
    if (currentSession.value?.id === id) {
      if (sessions.value.length > 0) {
        await selectSession(sessions.value[0].id)
      } else {
        currentSession.value = null
        router.push('/chat')
      }
    }
  } catch (err) {
    console.error('Failed to delete chat session', err)
  }
}

const addAttachment = (article: Article) => {
  if (!attachments.value.some(a => a.id === article.ID)) {
    attachments.value.push({
      id: article.ID,
      title: article.title
    })
  }

  // Remove the '@query' from input content
  if (showMentionDropdown.value) {
    const beforeMention = inputContent.value.substring(0, mentionIndex.value)
    const afterMention = inputContent.value.substring(mentionIndex.value + 1 + mentionQuery.value.length)
    inputContent.value = beforeMention + afterMention
  }

  showMentionDropdown.value = false
  mentionQuery.value = ''
  selectedMentionIdx.value = 0
  textareaRef.value?.focus()
}

const removeAttachment = (id: number) => {
  attachments.value = attachments.value.filter(a => a.id !== id)
}

const handleInput = (e: Event) => {
  const target = e.target as HTMLTextAreaElement
  const val = target.value
  const cursorPos = target.selectionStart || 0

  // Check if cursor is immediately after or near an '@'
  const textBeforeCursor = val.substring(0, cursorPos)
  const lastAtIndex = textBeforeCursor.lastIndexOf('@')

  if (lastAtIndex !== -1) {
    const queryCandidate = textBeforeCursor.substring(lastAtIndex + 1)
    // Only consider it an active mention if there are no newlines in the query
    if (!queryCandidate.includes('\n') && queryCandidate.length <= 40) {
      showMentionDropdown.value = true
      mentionQuery.value = queryCandidate
      mentionIndex.value = lastAtIndex
      selectedMentionIdx.value = 0
      return
    }
  }

  showMentionDropdown.value = false
}

const handleKeydown = (e: KeyboardEvent) => {
  if (showMentionDropdown.value && filteredMentionArticles.value.length > 0) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      selectedMentionIdx.value = (selectedMentionIdx.value + 1) % filteredMentionArticles.value.length
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      selectedMentionIdx.value = (selectedMentionIdx.value - 1 + filteredMentionArticles.value.length) % filteredMentionArticles.value.length
      return
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      const selected = filteredMentionArticles.value[selectedMentionIdx.value]
      if (selected) {
        addAttachment(selected)
      }
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      showMentionDropdown.value = false
      return
    }
  }

  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

const scrollToBottom = () => {
  if (messageContainer.value) {
    messageContainer.value.scrollTop = messageContainer.value.scrollHeight
  }
}

const highlightCodeBlocks = () => {
  document.querySelectorAll('.chat-prose pre code').forEach(block => {
    hljs.highlightElement(block as HTMLElement)
  })
}

const renderMarkdown = (content: string): string => {
  if (!content) return ''
  try {
    const html = marked.parse(content, { async: false })
    return typeof html === 'string' ? html : ''
  } catch (err) {
    return content
  }
}

const sendMessage = async () => {
  const text = inputContent.value.trim()
  if ((!text && attachments.value.length === 0) || isStreaming.value) return

  if (!hasApiKey.value) {
    errorMessage.value = 'Please configure your OpenRouter API Key in Settings first.'
    return
  }

  errorMessage.value = ''

  // Ensure active session
  if (!currentSession.value) {
    try {
      const res = await axios.post('/api/chats', { title: text.slice(0, 30) || 'New Chat' })
      currentSession.value = res.data
      sessions.value.unshift(res.data)
      router.push(`/chat/${res.data.id}`)
    } catch (err) {
      console.error('Failed to create chat session', err)
      errorMessage.value = 'Failed to create chat session'
      return
    }
  }

  if (!currentSession.value) return

  const session = currentSession.value
  const userMsgAttachments = [...attachments.value]
  const userMsg: Message = {
    role: 'user',
    content: text,
    attachments: userMsgAttachments.length > 0 ? userMsgAttachments : undefined
  }

  session.messages.push(userMsg)

  // Clear inputs
  inputContent.value = ''
  attachments.value = []
  showMentionDropdown.value = false

  // Prepare Assistant message for streaming
  const assistantMsg: Message = {
    role: 'assistant',
    content: ''
  }
  session.messages.push(assistantMsg)
  isStreaming.value = true

  await nextTick()
  scrollToBottom()

  try {
    const response = await fetch(`/api/chats/${session.id}/message`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${apiKey.value.trim()}`
      },
      body: JSON.stringify(userMsg)
    })

    if (!response.ok) {
      const errData = await response.json().catch(() => ({}))
      throw new Error(errData.error || `Request failed with HTTP ${response.status}`)
    }

    if (!response.body) {
      throw new Error('ReadableStream not supported on this response.')
    }

    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''

    while (true) {
      const { done, value } = await reader.read()
      if (done) break

      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() || ''

      for (const line of lines) {
        const trimmed = line.trim()
        if (trimmed.startsWith('data: ')) {
          const data = trimmed.slice(6)
          if (data === '[DONE]') {
            continue
          }
          assistantMsg.content += data
          scrollToBottom()
        } else if (trimmed.startsWith('event: error')) {
          console.error('SSE Error stream event')
        }
      }
    }

    // Refresh session title / list if it was a new chat
    const updatedSessionRes = await axios.get(`/api/chats/${session.id}`).catch(() => null)
    if (updatedSessionRes?.data) {
      const idx = sessions.value.findIndex(s => s.id === session.id)
      if (idx !== -1) {
        sessions.value[idx] = updatedSessionRes.data
      }
      if (currentSession.value) {
        currentSession.value.title = updatedSessionRes.data.title
      }
    }

    await nextTick()
    highlightCodeBlocks()
  } catch (err: any) {
    console.error('Streaming error', err)
    errorMessage.value = err.message || 'An error occurred during communication.'
    if (!assistantMsg.content) {
      assistantMsg.content = `*(Error: ${err.message || 'Failed to stream response'})*`
    }
  } finally {
    isStreaming.value = false
    await nextTick()
    scrollToBottom()
  }
}
</script>

<template>
  <div class="h-[calc(100vh-7.5rem)] flex bg-white dark:bg-[#0e0e0e] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 overflow-hidden shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.25)]">
    
    <!-- Sidebar -->
    <aside class="w-64 sm:w-72 flex-shrink-0 border-r border-gray-200/60 dark:border-gray-800/60 flex flex-col bg-gray-50/50 dark:bg-[#121212]/50">
      
      <!-- New Chat Button -->
      <div class="p-4 border-b border-gray-200/60 dark:border-gray-800/60">
        <button
          @click="createNewChat"
          class="w-full flex items-center justify-center gap-2 bg-[#111] dark:bg-white text-white dark:text-[#111] hover:bg-[#222] dark:hover:bg-gray-100 active:scale-[0.98] px-4 py-2.5 rounded-xl font-medium text-sm transition-all duration-200 shadow-sm cursor-pointer"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <line x1="12" y1="5" x2="12" y2="19"></line>
            <line x1="5" y1="12" x2="19" y2="12"></line>
          </svg>
          New Chat
        </button>
      </div>

      <!-- Sessions List -->
      <div class="flex-1 overflow-y-auto p-3 space-y-1">
        <div v-if="isLoadingSessions" class="text-center py-6 text-xs text-gray-400">
          Loading chats...
        </div>

        <div v-else-if="sessions.length === 0" class="text-center py-8 px-4 text-xs text-gray-400 leading-relaxed">
          No previous conversations yet.<br />Start a new chat to begin.
        </div>

        <div
          v-for="s in sessions"
          :key="s.id"
          @click="selectSession(s.id)"
          class="group flex items-center justify-between px-3 py-2.5 rounded-xl text-sm transition-all duration-150 cursor-pointer"
          :class="currentSession?.id === s.id ? 'bg-white dark:bg-[#1c1c1c] text-gray-900 dark:text-gray-100 shadow-xs border border-gray-200/60 dark:border-gray-700/60 font-medium' : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100/80 dark:hover:bg-[#181818]'"
        >
          <div class="flex items-center gap-2.5 truncate flex-1 min-w-0 pr-2">
            <svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400 flex-shrink-0">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
            </svg>
            <span class="truncate text-xs">{{ s.title || 'Untitled Chat' }}</span>
          </div>

          <button
            @click="deleteSession(s.id, $event)"
            title="Delete conversation"
            class="opacity-0 group-hover:opacity-100 p-1 text-gray-400 hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-950/40 rounded-lg transition-all"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="3 6 5 6 21 6"></polyline>
              <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
            </svg>
          </button>
        </div>
      </div>
    </aside>

    <!-- Main Chat Pane -->
    <main class="flex-1 flex flex-col min-w-0 bg-white dark:bg-[#0e0e0e]">
      
      <!-- Missing API Key Alert -->
      <div v-if="!hasApiKey" class="px-6 py-2.5 bg-amber-50 dark:bg-amber-950/40 border-b border-amber-200/70 dark:border-amber-800/70 flex items-center justify-between text-xs text-amber-800 dark:text-amber-300">
        <div class="flex items-center gap-2">
          <svg xmlns="http://www.w3.org/2000/svg" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"></circle>
            <line x1="12" y1="8" x2="12" y2="12"></line>
            <line x1="12" y1="16" x2="12.01" y2="16"></line>
          </svg>
          <span>OpenRouter API Key not set. Chat features require an API key.</span>
        </div>
        <router-link to="/settings" class="font-medium underline hover:text-amber-900 dark:hover:text-amber-100">
          Go to Settings
        </router-link>
      </div>

      <!-- Messages Stream View -->
      <div
        ref="messageContainer"
        class="flex-1 overflow-y-auto p-4 sm:p-6 space-y-6"
      >
        <!-- Empty State -->
        <div
          v-if="!currentSession || currentSession.messages.length === 0"
          class="h-full flex flex-col items-center justify-center text-center p-6"
        >
          <div class="w-14 h-14 rounded-2xl bg-emerald-50 dark:bg-emerald-950/40 flex items-center justify-center mb-4 text-emerald-600 dark:text-emerald-400 border border-emerald-200/60 dark:border-emerald-800/60">
            <svg xmlns="http://www.w3.org/2000/svg" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
            </svg>
          </div>
          <h2 class="text-xl font-bold text-gray-900 dark:text-gray-100">Chat with Readr AI</h2>
          <p class="text-sm text-gray-500 dark:text-gray-400 max-w-md mt-2 leading-relaxed">
            Ask questions, synthesize themes, or type <span class="text-emerald-600 dark:text-emerald-400 font-mono font-medium">@</span> to attach articles directly into your prompt.
          </p>

          <div class="mt-8 grid grid-cols-1 sm:grid-cols-2 gap-3 max-w-lg w-full text-left">
            <button
              @click="inputContent = 'Summarize key takeaways across my reading list.'"
              class="p-3 rounded-xl border border-gray-200/80 dark:border-gray-800/80 bg-gray-50/50 dark:bg-[#141414] hover:bg-gray-100 dark:hover:bg-[#1a1a1a] transition-all text-xs text-gray-700 dark:text-gray-300"
            >
              💡 "Summarize key takeaways across my reading list."
            </button>
            <button
              @click="inputContent = 'What are the main concepts covered in my attached articles?'"
              class="p-3 rounded-xl border border-gray-200/80 dark:border-gray-800/80 bg-gray-50/50 dark:bg-[#141414] hover:bg-gray-100 dark:hover:bg-[#1a1a1a] transition-all text-xs text-gray-700 dark:text-gray-300"
            >
              💡 "What are the main concepts covered in my attached articles?"
            </button>
          </div>
        </div>

        <!-- Message List -->
        <template v-else>
          <div
            v-for="(m, idx) in currentSession.messages"
            :key="idx"
            class="flex flex-col space-y-2"
          >
            <!-- User Bubble -->
            <div v-if="m.role === 'user'" class="flex justify-end">
              <div class="max-w-2xl bg-[#111] dark:bg-white text-white dark:text-[#111] px-5 py-3.5 rounded-2xl rounded-tr-sm shadow-sm space-y-2">
                <!-- Attachments in User Bubble -->
                <div v-if="m.attachments && m.attachments.length > 0" class="flex flex-wrap gap-1.5 pb-1">
                  <span
                    v-for="att in m.attachments"
                    :key="att.id"
                    class="inline-flex items-center gap-1 bg-white/20 dark:bg-black/15 text-white dark:text-[#111] text-xs px-2.5 py-1 rounded-full font-medium"
                  >
                    📄 {{ att.title }}
                  </span>
                </div>
                <div class="text-sm whitespace-pre-wrap leading-relaxed">{{ m.content }}</div>
              </div>
            </div>

            <!-- Assistant Bubble -->
            <div v-else-if="m.role === 'assistant'" class="flex justify-start">
              <div class="max-w-3xl bg-gray-50/90 dark:bg-[#161616] border border-gray-200/60 dark:border-gray-800/60 text-gray-900 dark:text-gray-100 px-5 py-4 rounded-2xl rounded-tl-sm shadow-xs space-y-2 w-full sm:w-auto">
                <div class="flex items-center gap-2 mb-2 pb-2 border-b border-gray-200/40 dark:border-gray-800/40">
                  <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
                  <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">Readr Assistant</span>
                </div>
                <!-- Rendered Markdown -->
                <div
                  class="chat-prose text-sm leading-relaxed max-w-none text-gray-800 dark:text-gray-200"
                  v-html="renderMarkdown(m.content)"
                ></div>
                <div v-if="isStreaming && idx === currentSession.messages.length - 1" class="flex items-center gap-1.5 pt-1 text-emerald-600 dark:text-emerald-400 text-xs">
                  <span class="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-pulse"></span>
                  <span>Generating response...</span>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>

      <!-- Error Toast / Banner -->
      <div v-if="errorMessage" class="px-6 py-2 bg-red-50 dark:bg-red-950/40 border-t border-red-200 dark:border-red-900/60 text-xs text-red-600 dark:text-red-400 flex items-center justify-between">
        <span>{{ errorMessage }}</span>
        <button @click="errorMessage = ''" class="hover:text-red-800 font-bold">&times;</button>
      </div>

      <!-- Input Area & Attachments -->
      <div class="p-4 border-t border-gray-200/60 dark:border-gray-800/60 bg-gray-50/30 dark:bg-[#121212]/30 relative">
        
        <!-- Floating Mention Autocomplete Dropdown -->
        <div
          v-if="showMentionDropdown && filteredMentionArticles.length > 0"
          class="absolute bottom-full left-4 right-4 mb-2 bg-white dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-2xl shadow-xl overflow-hidden z-50 max-h-56 overflow-y-auto"
        >
          <div class="px-3 py-2 text-[11px] font-semibold text-gray-400 dark:text-gray-500 border-b border-gray-100 dark:border-gray-800 uppercase tracking-wider">
            Mention an article
          </div>
          <div
            v-for="(art, idx) in filteredMentionArticles"
            :key="art.ID"
            @click="addAttachment(art)"
            class="px-3 py-2.5 flex items-center gap-2.5 text-xs text-gray-800 dark:text-gray-200 hover:bg-emerald-50 dark:hover:bg-emerald-950/40 hover:text-emerald-700 dark:hover:text-emerald-300 transition-colors cursor-pointer"
            :class="selectedMentionIdx === idx ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 font-medium' : ''"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400 flex-shrink-0">
              <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
              <polyline points="14 2 14 8 20 8"></polyline>
            </svg>
            <span class="truncate">{{ art.title }}</span>
          </div>
        </div>

        <!-- Attachment Chips -->
        <div v-if="attachments.length > 0" class="flex flex-wrap gap-2 mb-3">
          <span
            v-for="att in attachments"
            :key="att.id"
            class="bg-emerald-50 dark:bg-emerald-950/50 text-emerald-800 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800 text-xs font-medium px-3 py-1.5 rounded-full flex items-center gap-1.5 shadow-xs"
          >
            📄 {{ att.title }}
            <button
              type="button"
              @click="removeAttachment(att.id)"
              class="text-emerald-600 dark:text-emerald-400 hover:text-emerald-900 dark:hover:text-emerald-100 font-bold ml-1 transition-colors"
            >&times;</button>
          </span>
        </div>

        <!-- Input Box & Submit Button -->
        <div class="flex items-end gap-2 bg-white dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-2xl p-2 focus-within:border-gray-300 dark:focus-within:border-gray-700 focus-within:ring-4 focus-within:ring-gray-100 dark:focus-within:ring-gray-800 transition-all">
          <textarea
            ref="textareaRef"
            v-model="inputContent"
            @input="handleInput"
            @keydown="handleKeydown"
            rows="2"
            placeholder="Ask a question, or type @ to mention an article..."
            class="flex-1 bg-transparent border-none resize-none focus:outline-none text-sm text-gray-900 dark:text-gray-100 placeholder:text-gray-400 dark:placeholder:text-gray-600 p-2 min-h-[44px]"
          ></textarea>

          <button
            @click="sendMessage"
            :disabled="(!inputContent.trim() && attachments.length === 0) || isStreaming"
            class="p-2.5 bg-[#111] dark:bg-white text-white dark:text-[#111] rounded-xl hover:bg-[#222] dark:hover:bg-gray-100 active:scale-95 disabled:opacity-40 disabled:cursor-not-allowed transition-all shadow-sm flex-shrink-0 cursor-pointer"
            title="Send Message"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="22" y1="2" x2="11" y2="13"></line>
              <polygon points="22 2 15 22 11 13 2 9 22 2"></polygon>
            </svg>
          </button>
        </div>
      </div>
    </main>
  </div>
</template>

<style>
/* Prose markdown formatting inside chat */
.chat-prose p {
  margin-bottom: 0.75rem;
}
.chat-prose p:last-child {
  margin-bottom: 0;
}
.chat-prose ul, .chat-prose ol {
  margin-left: 1.25rem;
  margin-bottom: 0.75rem;
}
.chat-prose ul {
  list-style-type: disc;
}
.chat-prose ol {
  list-style-type: decimal;
}
.chat-prose li {
  margin-bottom: 0.25rem;
}
.chat-prose code {
  background-color: rgba(120, 120, 120, 0.15);
  padding: 0.15rem 0.35rem;
  border-radius: 0.25rem;
  font-family: monospace;
  font-size: 0.85em;
}
.chat-prose pre {
  background-color: #0d1117;
  color: #c9d1d9;
  padding: 0.75rem 1rem;
  border-radius: 0.75rem;
  overflow-x: auto;
  margin-top: 0.5rem;
  margin-bottom: 0.75rem;
}
.chat-prose pre code {
  background-color: transparent;
  padding: 0;
  color: inherit;
}
.chat-prose h1, .chat-prose h2, .chat-prose h3, .chat-prose h4 {
  font-weight: 600;
  margin-top: 1rem;
  margin-bottom: 0.5rem;
}
.chat-prose blockquote {
  border-left: 3px solid #10b981;
  padding-left: 0.75rem;
  margin-left: 0;
  margin-bottom: 0.75rem;
  opacity: 0.85;
}
</style>
