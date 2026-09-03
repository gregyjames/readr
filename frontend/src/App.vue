<script setup lang="ts">
import BookmarkIcon from './assets/book.svg'
import HomeIcon from './assets/home.svg'
import AddIcon from './assets/add.svg'
import GraphIcon from './assets/graph.svg'
import CommandPalette from './components/CommandPalette.vue'
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import axios from 'axios'
import emitter from './event-bus.ts'
import { isSettingsLoaded, initSettings, settings, toggleTheme } from './store/settings'
import { authState, logout } from './store/auth'

interface TemplateInfo {
  name: string
  filename: string
}

const route = useRoute()
const router = useRouter()

const handleLogout = async () => {
  const success = await logout()
  if (success) {
    router.push('/login')
  }
}

const showModal = ref(false)
const url = ref('')
const tags = ref<string[]>([])
const tagInput = ref('')
const isSubmitting = ref(false)
const availableTemplates = ref<TemplateInfo[]>([])
const selectedTemplate = ref<string>('auto')

const fetchTemplates = async () => {
  try {
    const res = await axios.get('/api/templates')
    availableTemplates.value = res.data || []
  } catch (err) {
    console.error('Failed to fetch templates', err)
  }
}

const matchedTemplate = computed(() => {
  if (!url.value) return null
  try {
    const raw = url.value.startsWith('http') ? url.value : `https://${url.value}`
    const host = new URL(raw).hostname.toLowerCase()
    const parts = host.split('.')
    for (let i = 0; i < parts.length - 1; i++) {
      const candidate = parts.slice(i).join('.')
      const found = availableTemplates.value.find(t => t.name === candidate)
      if (found) return found
    }
  } catch {
    return null
  }
  return null
})

const isDark = computed(() => settings.theme === 'dark')

const handleGlobalKeydown = (e: KeyboardEvent) => {
  // Ignore if user is currently typing in an input or textarea
  const target = e.target as HTMLElement
  if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable)) {
    return
  }

  if (e.metaKey || e.ctrlKey) {
    if (e.key === '1') {
      e.preventDefault()
      router.push('/')
    } else if (e.key === '2') {
      e.preventDefault()
      router.push('/graph')
    } else if (e.key === '3') {
      e.preventDefault()
      router.push('/chat')
    } else if (e.key === '4') {
      e.preventDefault()
      router.push('/archive')
    } else if (e.key === ',') {
      e.preventDefault()
      router.push('/settings')
    } else if (e.key === 'n' || e.key === 'N') {
      e.preventDefault()
      openModal()
    }
  }
}

onMounted(async () => {
  await initSettings()
  fetchTemplates()
  window.addEventListener('keydown', handleGlobalKeydown)
  emitter.on('open-add-modal', openModal)

  let evtSource: EventSource | null = null
  const connectSSE = () => {
    try {
      evtSource = new EventSource('/api/events')
      evtSource.onmessage = (event) => {
        const msg = (event.data || '').trim()
        if (msg === 'graph-updated') {
          emitter.emit('article-added')
          emitter.emit('graph-updated')
        }
      }
      evtSource.onerror = () => {
        evtSource?.close()
        setTimeout(connectSSE, 3000)
      }
    } catch (e) {
      console.warn('EventSource failed:', e)
    }
  }
  connectSSE()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  emitter.off('open-add-modal', openModal)
})

const submitForm = async () => {
  isSubmitting.value = true
  try {
    const chosenTemplate = selectedTemplate.value === 'auto'
      ? (matchedTemplate.value?.name || '')
      : (selectedTemplate.value === 'none' ? 'none' : selectedTemplate.value)

    const res = await fetch('/api/add', {
      method: 'POST',
      headers: { 
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ 
        url: url.value,
        Tags: tags.value,
        template: chosenTemplate
      }),
    })
    
    if (!res.ok) {
      throw new Error(`Failed to add article: ${res.statusText}`)
    }

    emitter.emit('article-added')
    showModal.value = false
    url.value = ''
    tags.value = []
    tagInput.value = ''
    selectedTemplate.value = 'auto'
  } catch (err) {
    console.error('Submit failed', err)
  } finally {
    isSubmitting.value = false
  }
}

function openModal() {
  showModal.value = true
  fetchTemplates()
}

function closeModal() {
  showModal.value = false
  selectedTemplate.value = 'auto'
}

function addTag() {
  const trimmed = tagInput.value.trim()
  if (trimmed && !tags.value.includes(trimmed)) {
    tags.value.push(trimmed)
  }
  tagInput.value = ''
}

function removeTag(tag: string) {
  tags.value = tags.value.filter(t => t !== tag)
}
</script>

<template>
  <div class="min-h-screen w-full flex flex-col md:flex-row bg-[#FAFAFA] dark:bg-[#08090C] text-gray-900 dark:text-[#E6EDF3] antialiased transition-colors duration-200 selection:bg-emerald-500/20 selection:text-emerald-700 dark:selection:bg-emerald-500/30 dark:selection:text-emerald-300">
    
    <!-- Left Navigation Rail (Desktop) -->
    <aside
      v-if="authState.isAuthenticated && route.path !== '/login'"
      class="hidden md:flex flex-col items-center justify-between w-16 h-screen sticky top-0 left-0 py-5 bg-white/80 dark:bg-[#0C0E14]/80 backdrop-blur-xl border-r border-black/[0.06] dark:border-white/[0.06] z-40 select-none flex-shrink-0"
    >
      <!-- Top Section -->
      <div class="flex flex-col items-center gap-4 w-full">
        <!-- Brand Glyph with pulse dot -->
        <router-link
          to="/"
          class="relative group p-2 rounded-xl bg-gray-100 dark:bg-white/[0.05] hover:bg-emerald-500/10 hover:border-emerald-500/30 border border-transparent transition-all"
          title="Readr Vault (⌘1)"
        >
          <BookmarkIcon class="w-5 h-5 text-emerald-600 dark:text-emerald-400 group-hover:scale-105 transition-transform" />
          <span class="absolute -bottom-0.5 -right-0.5 w-2 h-2 rounded-full bg-emerald-500 ring-2 ring-white dark:ring-[#0C0E14]"></span>
        </router-link>

        <!-- Quick Ingest Action Button -->
        <button
          @click="openModal"
          class="w-10 h-10 rounded-xl bg-emerald-500 hover:bg-emerald-600 text-white flex items-center justify-center transition-all duration-200 active:scale-95 shadow-sm shadow-emerald-500/25 group cursor-pointer"
          title="Ingest Article (⌘N)"
        >
          <AddIcon class="w-4 h-4 text-white group-hover:rotate-90 transition-transform duration-200" />
        </button>

        <div class="w-8 h-[1px] bg-black/[0.06] dark:bg-white/[0.06] my-1"></div>

        <!-- Navigation Icons Rail -->
        <nav class="flex flex-col items-center gap-1.5 w-full px-2">
          <!-- Articles / Feed -->
          <router-link
            to="/"
            class="relative p-2.5 rounded-xl transition-all group cursor-pointer"
            :class="route.path === '/' ? 'bg-gray-100 dark:bg-white/[0.08] text-gray-950 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100/60 dark:hover:bg-white/[0.04]'"
            title="Library (⌘1)"
          >
            <HomeIcon class="w-5 h-5" />
            <span v-if="route.path === '/'" class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-4 bg-emerald-500 rounded-r-full"></span>
          </router-link>

          <!-- Graph -->
          <router-link
            to="/graph"
            class="relative p-2.5 rounded-xl transition-all group cursor-pointer"
            :class="route.path === '/graph' ? 'bg-gray-100 dark:bg-white/[0.08] text-gray-950 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100/60 dark:hover:bg-white/[0.04]'"
            title="Knowledge Graph (⌘2)"
          >
            <GraphIcon class="w-5 h-5" />
            <span v-if="route.path === '/graph'" class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-4 bg-emerald-500 rounded-r-full"></span>
          </router-link>

          <!-- AI Chat -->
          <router-link
            to="/chat"
            class="relative p-2.5 rounded-xl transition-all group cursor-pointer"
            :class="route.path.startsWith('/chat') ? 'bg-gray-100 dark:bg-white/[0.08] text-gray-950 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100/60 dark:hover:bg-white/[0.04]'"
            title="AI Chat (⌘3)"
          >
            <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
            </svg>
            <span v-if="route.path.startsWith('/chat')" class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-4 bg-emerald-500 rounded-r-full"></span>
          </router-link>

          <!-- Archive -->
          <router-link
            to="/archive"
            class="relative p-2.5 rounded-xl transition-all group cursor-pointer"
            :class="route.path === '/archive' ? 'bg-gray-100 dark:bg-white/[0.08] text-gray-950 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100/60 dark:hover:bg-white/[0.04]'"
            title="Archive (⌘4)"
          >
            <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect width="20" height="5" x="2" y="3" rx="1"/>
              <path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/>
              <path d="M10 12h4"/>
            </svg>
            <span v-if="route.path === '/archive'" class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-4 bg-emerald-500 rounded-r-full"></span>
          </router-link>

          <!-- Search Trigger (⌘K) -->
          <button
            @click="emitter.emit('open-search')"
            class="p-2.5 rounded-xl text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100/60 dark:hover:bg-white/[0.04] transition-all cursor-pointer"
            title="Quick Search (⌘K)"
          >
            <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"></circle>
              <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
            </svg>
          </button>
        </nav>
      </div>

      <!-- Bottom Rail Section -->
      <div class="flex flex-col items-center gap-2 w-full px-2">
        <!-- Settings -->
        <router-link
          to="/settings"
          class="relative p-2.5 rounded-xl transition-all group cursor-pointer"
          :class="route.path === '/settings' ? 'bg-gray-100 dark:bg-white/[0.08] text-gray-950 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100/60 dark:hover:bg-white/[0.04]'"
          title="Settings & Telemetry (⌘,)"
        >
          <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="3"></circle>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
          </svg>
          <span v-if="route.path === '/settings'" class="absolute left-0 top-1/2 -translate-y-1/2 w-1 h-4 bg-emerald-500 rounded-r-full"></span>
        </router-link>

        <!-- Theme Toggle -->
        <button
          @click="toggleTheme"
          class="p-2.5 rounded-xl text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100/60 dark:hover:bg-white/[0.04] transition-all cursor-pointer"
          :title="isDark ? 'Switch to light theme' : 'Switch to dark theme'"
        >
          <svg v-if="isDark" class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="4"></circle>
            <path d="M12 2v2"></path>
            <path d="M12 20v2"></path>
            <path d="m4.93 4.93 1.41 1.41"></path>
            <path d="m17.66 17.66 1.41 1.41"></path>
            <path d="M2 12h2"></path>
            <path d="M20 12h2"></path>
            <path d="m6.34 17.66-1.41 1.41"></path>
            <path d="m19.07 4.93-1.41 1.41"></path>
          </svg>
          <svg v-else class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"></path>
          </svg>
        </button>

        <!-- Logout -->
        <button
          @click="handleLogout"
          class="p-2.5 rounded-xl text-gray-400 hover:text-red-500 dark:hover:text-red-400 hover:bg-red-500/10 transition-all cursor-pointer"
          title="Sign out of vault"
        >
          <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path>
            <polyline points="16 17 21 12 16 7"></polyline>
            <line x1="21" y1="12" x2="9" y2="12"></line>
          </svg>
        </button>
      </div>
    </aside>

    <!-- Mobile Top Navigation Header -->
    <header
      v-if="authState.isAuthenticated && route.path !== '/login'"
      class="md:hidden flex items-center justify-between px-4 h-14 bg-white/80 dark:bg-[#0C0E14]/80 backdrop-blur-md border-b border-black/[0.06] dark:border-white/[0.06] sticky top-0 z-40"
    >
      <router-link to="/" class="flex items-center gap-2">
        <BookmarkIcon class="w-5 h-5 text-emerald-600 dark:text-emerald-400" />
        <span class="font-semibold text-sm tracking-tight">Readr</span>
      </router-link>
      <div class="flex items-center gap-1">
        <button @click="emitter.emit('open-search')" class="p-2 text-gray-500">
          <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>
        </button>
        <button @click="openModal" class="px-2.5 py-1 text-xs font-medium rounded-lg bg-emerald-500 text-white flex items-center gap-1">
          <AddIcon class="w-3.5 h-3.5" />
          <span>Add</span>
        </button>
      </div>
    </header>

    <!-- Mobile Bottom Navigation Bar -->
    <nav
      v-if="authState.isAuthenticated && route.path !== '/login'"
      class="md:hidden fixed bottom-0 left-0 right-0 h-14 bg-white/90 dark:bg-[#0C0E14]/90 backdrop-blur-xl border-t border-black/[0.06] dark:border-white/[0.06] flex items-center justify-around z-40 px-2"
    >
      <router-link to="/" class="p-2 text-xs flex flex-col items-center gap-0.5" :class="route.path === '/' ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'">
        <HomeIcon class="w-5 h-5" />
      </router-link>
      <router-link to="/graph" class="p-2 text-xs flex flex-col items-center gap-0.5" :class="route.path === '/graph' ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'">
        <GraphIcon class="w-5 h-5" />
      </router-link>
      <router-link to="/chat" class="p-2 text-xs flex flex-col items-center gap-0.5" :class="route.path.startsWith('/chat') ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'">
        <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path></svg>
      </router-link>
      <router-link to="/archive" class="p-2 text-xs flex flex-col items-center gap-0.5" :class="route.path === '/archive' ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'">
        <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect width="20" height="5" x="2" y="3" rx="1"/>
          <path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/>
          <path d="M10 12h4"/>
        </svg>
      </router-link>
      <router-link to="/settings" class="p-2 text-xs flex flex-col items-center gap-0.5" :class="route.path === '/settings' ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-400'">
        <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>
      </router-link>
    </nav>

    <!-- Main Dynamic Workspace -->
    <main class="flex-1 flex flex-col min-w-0 min-h-screen pb-16 md:pb-0">
      <router-view v-if="route.name === 'login'" />
      <div v-else-if="!isSettingsLoaded" class="flex-grow flex items-center justify-center text-gray-400 text-xs font-mono">
        Loading vault...
      </div>
      <router-view v-else />
    </main>
  </div>

  <CommandPalette v-if="authState.isAuthenticated && route.path !== '/login'" />

  <!-- Add Article Modal -->
  <transition name="fade-blur">
    <div v-if="showModal" @click.self="closeModal" class="fixed inset-0 bg-black/40 dark:bg-black/60 backdrop-blur-xs flex justify-center items-center z-50 p-4 transition-all duration-200">
      <div class="bg-white dark:bg-[#12151C] rounded-2xl shadow-xl border border-gray-200/80 dark:border-white/[0.08] w-full max-w-md p-6 relative">
        <button
          @click.self="closeModal"
          :disabled="isSubmitting"
          class="absolute top-5 right-5 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 text-lg leading-none disabled:opacity-20 transition-colors cursor-pointer"
          aria-label="Close"
        >&times;</button>
        
        <h2 class="text-base font-semibold tracking-tight text-gray-900 dark:text-gray-100 mb-5">Add New Article</h2>

        <form @submit.prevent="submitForm" class="space-y-4">
          <div>
            <label for="url" class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1.5">Article URL</label>
            <input
              v-model="url"
              type="url"
              id="url"
              required
              placeholder="https://example.com/article"
              class="w-full px-3.5 py-2 text-sm bg-gray-50/50 dark:bg-white/[0.03] border border-gray-200 dark:border-white/10 rounded-lg focus:bg-white dark:focus:bg-[#12151C] focus:border-emerald-500 dark:focus:border-emerald-400 focus:ring-2 focus:ring-emerald-500/20 focus:outline-none transition-all placeholder:text-gray-400 text-gray-900 dark:text-gray-100"
            />
          </div>

          <div>
            <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1.5">Tags (optional)</label>
            <div v-if="tags.length > 0" class="flex flex-wrap gap-1.5 mb-2">
              <span v-for="tag in tags" :key="tag" class="bg-gray-100 dark:bg-white/[0.06] text-gray-700 dark:text-gray-300 text-xs font-medium px-2.5 py-1 rounded-md flex items-center gap-1 border border-gray-200/60 dark:border-white/[0.05]">
                {{ tag }}
                <button type="button" @click="removeTag(tag)" class="text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 text-xs font-bold ml-0.5">&times;</button>
              </span>
            </div>
            <input
              v-model="tagInput"
              @keydown.enter.prevent="addTag"
              type="text"
              placeholder="Press enter to add tag"
              class="w-full px-3.5 py-2 text-sm bg-gray-50/50 dark:bg-white/[0.03] border border-gray-200 dark:border-white/10 rounded-lg focus:bg-white dark:focus:bg-[#12151C] focus:border-emerald-500 dark:focus:border-emerald-400 focus:ring-2 focus:ring-emerald-500/20 focus:outline-none transition-all placeholder:text-gray-400 text-gray-900 dark:text-gray-100"
            />
          </div>

          <div v-if="availableTemplates.length > 0">
            <label class="block text-xs font-medium text-gray-700 dark:text-gray-300 mb-1.5">
              Markdown Template
            </label>
            <select
              v-model="selectedTemplate"
              class="w-full px-3.5 py-2 text-sm rounded-lg border border-gray-200 dark:border-white/10 bg-white dark:bg-[#12151C] text-gray-900 dark:text-gray-100 focus:outline-none focus:ring-2 focus:ring-emerald-500/20 focus:border-emerald-500"
            >
              <option value="auto">
                Auto {{ matchedTemplate ? `(${matchedTemplate.name})` : '(Default)' }}
              </option>
              <option v-for="tpl in availableTemplates" :key="tpl.name" :value="tpl.name">
                {{ tpl.name }}
              </option>
              <option value="none">Built-in Default</option>
            </select>
          </div>

          <button
            type="submit"
            :disabled="isSubmitting"
            class="w-full mt-2 bg-gray-950 hover:bg-black dark:bg-white dark:hover:bg-gray-100 text-white dark:text-gray-950 py-2.5 px-4 rounded-lg font-medium text-sm transition-all active:scale-[0.99] disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            {{ isSubmitting ? 'Ingesting...' : 'Save & Parse' }}
          </button>
        </form>
      </div>
    </div>
  </transition>
</template>

<style scoped>
.fade-blur-enter-active,
.fade-blur-leave-active {
  transition: opacity 0.3s ease, backdrop-filter 0.3s ease;
}
.fade-blur-enter-from,
.fade-blur-leave-to {
  opacity: 0;
  backdrop-filter: blur(0px);
}
.fade-blur-enter-to,
.fade-blur-leave-from {
  opacity: 1;
  backdrop-filter: blur(8px);
}
</style>