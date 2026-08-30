<script setup lang="ts">
import BookmarkIcon from './assets/book.svg'
import HomeIcon from './assets/home.svg'
import AddIcon from './assets/add.svg'
import CardIcon from './assets/card.svg'
import ListIcon from './assets/list.svg'
import GraphIcon from './assets/graph.svg'
import CommandPalette from './components/CommandPalette.vue'
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import emitter from './event-bus.ts'

const router = useRouter()
const route = useRoute()

const showModal = ref(false)
const url = ref('')
const viewMode = ref<'card' | 'list'>('card')
const tags = ref<string[]>([])
const tagInput = ref('')
const isSubmitting = ref(false)

const submitForm = async () => {
  isSubmitting.value = true
  try{
    await fetch('/api/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ 
        url: url.value,
        Tags: tags.value
      }),
    })
    emitter.emit('article-added')
    showModal.value = false
    url.value = ''
    tags.value = []
    tagInput.value = ''
  }
  catch (err) {
    console.error('Submit failed', err)
  }
  finally{
    isSubmitting.value = false
  }
}

function closeModal() {
  showModal.value = false
}

const toggleViewMode = () => {
  viewMode.value = viewMode.value === 'card' ? 'list' : 'card'
  localStorage.setItem('viewMode', viewMode.value)
  router.push({ name: 'home', query: { view: viewMode.value } })
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

const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  const root = document.documentElement
  if (root.classList.contains('dark')) {
    root.classList.remove('dark')
    localStorage.setItem('theme', 'light')
    isDark.value = false
  } else {
    root.classList.add('dark')
    localStorage.setItem('theme', 'dark')
    isDark.value = true
  }
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
        <div class="flex items-center space-x-4">
          <button @click="toggleTheme" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors">
            <svg v-if="!isDark" xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>
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
          
          <button @click="toggleViewMode" v-if="route.name === 'home'" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors" title="Toggle layout view">
            <span v-if="viewMode === 'card'">
              <ListIcon class="w-5 h-5"/>
            </span>
            <span v-else>
              <CardIcon class="w-5 h-5"/>
            </span>
          </button>
          
          <button @click="showModal = true" class="flex items-center gap-2 bg-[#111] dark:bg-white hover:bg-[#222] dark:hover:bg-gray-100 active:scale-95 text-white dark:text-[#111] px-4 py-2 rounded-full transition-all duration-300 shadow-sm ml-2 cursor-pointer">
            <AddIcon class="w-4 h-4 text-white dark:text-[#111]" />
            <span class="font-medium text-sm">Add Article</span>
          </button>
        </div>
      </div>
    </div>
  </nav>

  <CommandPalette />

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
    <router-view />
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