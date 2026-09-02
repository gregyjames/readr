<template>
  <transition name="fade-blur">
    <div
      v-if="isOpen"
      @click.self="close"
      class="fixed inset-0 bg-[#0a0a0a]/60 backdrop-blur-md flex justify-center items-start pt-[15vh] z-[100] p-4 transition-all duration-300"
    >
      <div class="bg-white dark:bg-[#111] rounded-2xl shadow-[0_16px_64px_rgba(0,0,0,0.15)] dark:shadow-[0_16px_64px_rgba(0,0,0,0.5)] border border-gray-100 dark:border-gray-800 w-full max-w-2xl overflow-hidden flex flex-col transform transition-all duration-300">
        
        <!-- Search Input -->
        <div class="flex items-center px-4 py-3 border-b border-gray-100 dark:border-gray-800">
          <svg class="w-5 h-5 text-gray-400 dark:text-gray-500 mr-3 shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="11" cy="11" r="8"></circle>
            <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
          </svg>
          <input
            ref="searchInput"
            v-model="query"
            @keydown.down.prevent="selectNext"
            @keydown.up.prevent="selectPrev"
            @keydown.enter.prevent="goToSelected"
            @keydown.esc.prevent="close"
            type="text"
            class="flex-1 bg-transparent border-none focus:outline-none text-gray-900 dark:text-gray-100 text-lg placeholder:text-gray-400 dark:placeholder:text-gray-600"
            placeholder="Search articles..."
          />
          <div class="flex items-center gap-1.5 ml-3 shrink-0 text-xs text-gray-400 dark:text-gray-500 font-mono">
            <kbd class="px-1.5 py-0.5 rounded-md bg-gray-100 dark:bg-gray-800 border border-gray-200 dark:border-gray-700">ESC</kbd>
            <span>to close</span>
          </div>
        </div>

        <!-- Results List -->
        <div class="max-h-[60vh] overflow-y-auto p-2" v-if="results.length > 0">
          <button
            v-for="(result, index) in results"
            :key="result.id"
            @click="goTo(result.id)"
            @mouseenter="selectedIndex = index"
            :class="[
              'w-full text-left p-4 rounded-xl transition-colors duration-150 flex flex-col gap-2 cursor-pointer',
              selectedIndex === index 
                ? 'bg-gray-50 dark:bg-gray-800/60' 
                : 'hover:bg-gray-50 dark:hover:bg-gray-800/40'
            ]"
          >
            <div class="flex items-center gap-3">
              <svg class="w-4 h-4 text-emerald-500 shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
                <polyline points="14 2 14 8 20 8"></polyline>
                <line x1="16" y1="13" x2="8" y2="13"></line>
                <line x1="16" y1="17" x2="8" y2="17"></line>
                <polyline points="10 9 9 9 8 9"></polyline>
              </svg>
              <h3 class="font-semibold text-gray-900 dark:text-gray-100 line-clamp-1 flex-1">{{ result.title }}</h3>
              <span v-if="selectedIndex === index" class="text-xs text-gray-400 shrink-0 font-mono">↵ to jump</span>
            </div>
            
            <p 
              class="text-sm text-gray-500 dark:text-gray-400 line-clamp-2 leading-relaxed ml-7 search-excerpt"
              v-html="result.excerpt"
            ></p>
          </button>
        </div>
        
        <div v-else-if="query.length > 0 && !isLoading" class="p-8 text-center text-gray-500 dark:text-gray-400">
          No matching articles found.
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useMagicKeys, watchDebounced } from '@vueuse/core'
import axios from 'axios'
import DOMPurify from 'dompurify'
import emitter from '../event-bus'
import { authState } from '../store/auth'

interface SearchResult {
  id: number
  title: string
  excerpt: string
}

const router = useRouter()
const route = useRoute()
const isOpen = ref(false)
const query = ref('')

const openSearch = () => {
  if (!authState.isAuthenticated || route.path === '/login') {
    return
  }
  isOpen.value = true
  query.value = ''
  results.value = []
  nextTick(() => {
    searchInput.value?.focus()
  })
}

onMounted(() => {
  emitter.on('open-search', openSearch)
})

onUnmounted(() => {
  emitter.off('open-search', openSearch)
})

watch(() => route.path, (newPath) => {
  if (newPath === '/login' && isOpen.value) {
    isOpen.value = false
  }
})
const results = ref<SearchResult[]>([])
const isLoading = ref(false)
const selectedIndex = ref(0)
const searchInput = ref<HTMLInputElement | null>(null)

// Setup global Cmd+K listener
const keys = useMagicKeys()
const cmdK = keys['Meta+K']
const ctrlK = keys['Control+K']

watch([cmdK, ctrlK], ([cmd, ctrl]) => {
  if (cmd || ctrl) {
    if (!authState.isAuthenticated || route.path === '/login') {
      return
    }
    // Prevent browser default behavior (like search bar focus)
    isOpen.value = !isOpen.value
    if (isOpen.value) {
      query.value = ''
      results.value = []
      nextTick(() => {
        searchInput.value?.focus()
      })
    }
  }
})

const close = () => {
  isOpen.value = false
}

const selectNext = () => {
  if (selectedIndex.value < results.value.length - 1) {
    selectedIndex.value++
  }
}

const selectPrev = () => {
  if (selectedIndex.value > 0) {
    selectedIndex.value--
  }
}

const goToSelected = () => {
  if (results.value.length > 0 && results.value[selectedIndex.value]) {
    goTo(results.value[selectedIndex.value].id)
  }
}

const goTo = (id: number) => {
  isOpen.value = false
  router.push(`/articles/${id}`)
}

watchDebounced(query, async (newQuery) => {
  if (!newQuery.trim()) {
    results.value = []
    return
  }
  
  isLoading.value = true
  try {
    const res = await axios.get<SearchResult[]>(`/api/search?q=${encodeURIComponent(newQuery)}`)
    // The excerpt is server-built HTML (FTS5 snippet); keep only the <mark> highlight.
    results.value = (res.data || []).map(r => ({
      ...r,
      excerpt: DOMPurify.sanitize(r.excerpt ?? '', { ALLOWED_TAGS: ['mark'], ALLOWED_ATTR: [] })
    }))
    selectedIndex.value = 0
  } catch (err) {
    console.error('Search failed:', err)
  } finally {
    isLoading.value = false
  }
}, { debounce: 300 })
</script>

<style>
.search-excerpt mark {
  background-color: rgba(16, 185, 129, 0.2); /* Emerald 500 w/ opacity */
  color: inherit;
  border-radius: 2px;
  padding: 0 2px;
  font-weight: 600;
}
.dark .search-excerpt mark {
  background-color: rgba(16, 185, 129, 0.25);
  color: #fff;
}
</style>
