<script setup lang="ts">
import BookmarkIcon from './assets/book.svg'
import HomeIcon from './assets/home.svg'
import AddIcon from './assets/add.svg'
import GraphIcon from './assets/graph.svg'
import CommandPalette from './components/CommandPalette.vue'
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import axios from 'axios'
import emitter from './event-bus.ts'
import { isSettingsLoaded, initSettings } from './store/settings'
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

onMounted(async () => {
  await initSettings();
  fetchTemplates();
  let evtSource: EventSource | null = null;
  const connectSSE = () => {
    try {
      evtSource = new EventSource('/api/events');
      evtSource.onmessage = (event) => {
        const msg = (event.data || '').trim();
        if (msg === 'graph-updated') {
          emitter.emit('article-added');
          emitter.emit('graph-updated');
        }
      };
      evtSource.onerror = () => {
        evtSource?.close();
        setTimeout(connectSSE, 3000);
      };
    } catch (e) {
      console.warn('EventSource failed:', e);
    }
  };
  connectSSE();
})

const submitForm = async () => {
  isSubmitting.value = true
  try{
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
  }
  catch (err) {
    console.error('Submit failed', err)
  }
  finally{
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
  <nav class="bg-white/70 dark:bg-[#0a0a0a]/70 backdrop-blur-xl border-b border-gray-200/50 dark:border-gray-800/50 w-full fixed top-0 left-0 z-50 transition-colors duration-300">
    <div class="max-w-6xl mx-auto px-6">
      <div class="flex justify-between items-center h-20">
        <div class="flex items-center space-x-2">
          <router-link to="/" class="flex items-center text-gray-900 dark:text-gray-100 group">
            <div class="p-2 bg-gray-100 dark:bg-gray-800/60 rounded-lg group-hover:bg-gray-200 dark:group-hover:bg-gray-700 transition-colors duration-300">
              <BookmarkIcon class="w-5 h-5 text-gray-900 dark:text-gray-100 transition-colors duration-300" />
            </div>
            <span class="ml-3 text-gray-900 dark:text-gray-100 text-xl font-bold tracking-tight transition-colors duration-300">Readr</span>
          </router-link>
        </div>

        <!-- Menu -->
        <div v-if="authState.isAuthenticated && route.path !== '/login'" class="flex items-center space-x-4">

          
          <button @click="emitter.emit('open-search')" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors cursor-pointer" title="Search (Cmd+K)">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"></circle>
              <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
            </svg>
          </button>


          <router-link to="/" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors" title="Home">
            <HomeIcon class="w-5 h-5" />
          </router-link>

          <router-link to="/graph" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors" title="Knowledge Graph">
            <GraphIcon class="w-5 h-5" />
          </router-link>

          <router-link to="/chat" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors" title="AI Chat">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
            </svg>
          </router-link>

          <router-link to="/settings" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors" title="Settings">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="3"></circle>
              <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
            </svg>
          </router-link>

          <button
            @click="handleLogout"
            class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors cursor-pointer"
            title="Log out"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path>
              <polyline points="16 17 21 12 16 7"></polyline>
              <line x1="21" y1="12" x2="9" y2="12"></line>
            </svg>
          </button>
          

          
          <button @click="openModal" class="flex items-center gap-2 bg-[#111] dark:bg-white hover:bg-[#222] dark:hover:bg-gray-100 active:scale-95 text-white dark:text-[#111] px-4 py-2 rounded-full transition-all duration-300 shadow-sm ml-2 cursor-pointer">
            <AddIcon class="w-4 h-4 text-white dark:text-[#111]" />
            <span class="font-medium text-sm">Add Article</span>
          </button>
        </div>
      </div>
    </div>
  </nav>

  <CommandPalette v-if="authState.isAuthenticated && route.path !== '/login'" />

  <transition name="fade-blur">
    <div v-if="showModal" @click.self="closeModal" class="fixed inset-0 bg-[#0a0a0a]/60 backdrop-blur-md flex justify-center items-center z-50 transition-all duration-300 ease-out p-4">
      <!-- Modal content -->
      <div class="bg-white dark:bg-[#111] rounded-3xl shadow-[0_8px_32px_rgba(0,0,0,0.08)] dark:shadow-[0_8px_32px_rgba(0,0,0,0.4)] border border-gray-100 dark:border-gray-800 w-full max-w-md p-8 relative transform transition-all duration-300 scale-100 opacity-100">
        <button
          @click.self="closeModal"
          :disabled="isSubmitting"
          class="absolute top-6 right-6 text-gray-400 dark:text-gray-500 hover:text-gray-900 dark:hover:text-gray-300 text-2xl leading-none font-light disabled:opacity-10 transition-colors"
          aria-label="Close">&times;</button>
        <h2 class="text-2xl font-semibold mb-8 tracking-tight text-gray-900 dark:text-gray-100">Add an article</h2>

        <form @submit.prevent="submitForm" class="space-y-6">
          <div>
            <label for="url" class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">URL</label>
            <input
              v-model="url"
              type="url"
              id="url"
              required
              placeholder="https://example.com/article"
              class="w-full px-4 py-3 bg-gray-50 dark:bg-[#1a1a1a] border-transparent rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-gray-200 dark:focus:border-gray-700 focus:ring-4 focus:ring-gray-100 dark:focus:ring-gray-800 focus:outline-none transition-all placeholder:text-gray-400 dark:placeholder:text-gray-600 text-gray-900 dark:text-gray-100"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">Tags</label>
            <div v-if="tags.length > 0" class="flex flex-wrap gap-2 mb-3">
              <span v-for="tag in tags" :key="tag" class="bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 text-xs font-medium px-3 py-1.5 rounded-full flex items-center gap-1.5 border border-gray-200/60 dark:border-gray-700/60">
                {{ tag }}
                <button type="button" @click="removeTag(tag)" class="text-gray-400 dark:text-gray-500 hover:text-gray-700 dark:hover:text-gray-300 transition-colors text-xs font-bold">&times;</button>
              </span>
            </div>
            <input
              v-model="tagInput"
              @keydown.enter.prevent="addTag"
              type="text"
              placeholder="Type tag and press Enter"
              class="w-full px-4 py-3 bg-gray-50 dark:bg-[#1a1a1a] border-transparent rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-gray-200 dark:focus:border-gray-700 focus:ring-4 focus:ring-gray-100 dark:focus:ring-gray-800 focus:outline-none transition-all placeholder:text-gray-400 dark:placeholder:text-gray-600 text-gray-900 dark:text-gray-100"
            />
          </div>
          <div v-if="availableTemplates.length > 0" class="mt-4">
            <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-1.5">
              Markdown Template
            </label>
            <select
              v-model="selectedTemplate"
              class="w-full px-3 py-2 text-sm rounded-lg border border-gray-200 dark:border-white/10 bg-white dark:bg-black/40 text-gray-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-emerald-500"
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
            class="bg-[#111] dark:bg-white text-white dark:text-[#111] px-4 py-3.5 rounded-xl hover:bg-[#222] dark:hover:bg-gray-100 active:scale-[0.98] w-full font-medium transition-all duration-300 disabled:opacity-50 disabled:cursor-not-allowed mt-4"
          >
            {{ isSubmitting ? 'Saving...' : 'Save article' }}
          </button>
        </form>
      </div>
    </div>
  </transition>
  <main :class="route.name === 'graph' ? 'w-full flex-grow' : ((route.name === 'chat' || route.name === 'chat-session') ? 'w-full max-w-6xl mx-auto px-6 pt-24 pb-6 flex-grow flex flex-col' : 'w-full max-w-6xl mx-auto px-6 pt-32 pb-16 flex-grow')">
    <div v-if="!isSettingsLoaded" class="flex-grow flex items-center justify-center text-gray-500">
      Loading settings...
    </div>
    <router-view v-else />
  </main>
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