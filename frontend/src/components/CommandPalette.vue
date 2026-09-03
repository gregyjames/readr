<template>
  <transition name="fade-blur">
    <div
      v-if="isOpen"
      @click.self="close"
      class="fixed inset-0 bg-black/40 dark:bg-black/60 backdrop-blur-xs flex justify-center items-start pt-[12vh] z-[100] p-4 transition-all duration-200"
    >
      <div class="bg-white dark:bg-[#12151C] rounded-xl shadow-xl border border-gray-200/80 dark:border-white/[0.08] w-full max-w-xl overflow-hidden flex flex-col">
        
        <!-- Search Input -->
        <div class="flex items-center px-4 py-3 border-b border-gray-100 dark:border-white/[0.06]">
          <svg class="w-4 h-4 text-gray-400 dark:text-gray-500 mr-3 shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
            class="flex-1 bg-transparent border-none focus:outline-none text-gray-900 dark:text-gray-100 text-sm placeholder:text-gray-400 font-sans"
            placeholder="Search vault notes, tags, and articles..."
          />
          <div class="flex items-center gap-1 ml-3 shrink-0 text-[11px] text-gray-400 dark:text-gray-500 font-mono">
            <kbd class="px-1.5 py-0.5 rounded bg-gray-100 dark:bg-white/[0.06] border border-gray-200/80 dark:border-white/10">ESC</kbd>
          </div>
        </div>

        <!-- Navigation & Quick Commands when query is empty or matches commands -->
        <div class="p-2 border-b border-gray-100 dark:border-white/[0.06]" v-if="filteredCommands.length > 0">
          <div class="px-3 py-1.5 text-[10px] font-mono uppercase tracking-wider text-gray-400 dark:text-gray-500 font-semibold">
            Navigation
          </div>
          <button
            v-for="(cmd, index) in filteredCommands"
            :key="cmd.name"
            @click="executeCommand(cmd)"
            @mouseenter="selectedCommandIndex = index; selectedIndex = -1"
            :class="[
              'w-full text-left px-3 py-2 rounded-lg transition-colors duration-150 flex items-center justify-between cursor-pointer',
              selectedCommandIndex === index && selectedIndex === -1
                ? 'bg-gray-50 dark:bg-gray-800/60 text-gray-900 dark:text-gray-100'
                : 'hover:bg-gray-50 dark:hover:bg-gray-800/40 text-gray-700 dark:text-gray-300'
            ]"
          >
            <div class="flex items-center gap-2.5">
              <span v-html="cmd.icon" class="w-4 h-4 flex items-center justify-center text-gray-400 dark:text-gray-500"></span>
              <span class="text-xs font-medium">{{ cmd.name }}</span>
            </div>
            <span class="text-[10px] font-mono text-gray-400 dark:text-gray-500 bg-gray-100 dark:bg-white/[0.06] px-1.5 py-0.5 rounded">
              {{ cmd.shortcut }}
            </span>
          </button>
        </div>

        <!-- Results List -->
        <div class="max-h-[60vh] overflow-y-auto p-2" v-if="results.length > 0">
          <div class="px-3 py-1.5 text-[10px] font-mono uppercase tracking-wider text-gray-400 dark:text-gray-500 font-semibold">
            Articles
          </div>
          <button
            v-for="(result, index) in results"
            :key="result.id"
            @click="goTo(result.id)"
            @mouseenter="selectedIndex = index; selectedCommandIndex = -1"
            :class="[
              'w-full text-left p-4 rounded-xl transition-colors duration-150 flex flex-col gap-2 cursor-pointer',
              selectedIndex === index && selectedCommandIndex === -1
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
        
        <div v-else-if="query.length > 0 && !isLoading && filteredCommands.length === 0" class="p-8 text-center text-gray-500 dark:text-gray-400">
          No matching articles or commands found.
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
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
  selectedCommandIndex.value = 0
  selectedIndex.value = -1
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
interface CommandItem {
  name: string
  path: string
  shortcut: string
  icon: string
}

const commands: CommandItem[] = [
  {
    name: 'Go to Library',
    path: '/',
    shortcut: '⌘1',
    icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-4 h-4"><path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"></path><polyline points="9 22 9 12 15 12 15 22"></polyline></svg>',
  },
  {
    name: 'Go to Knowledge Graph',
    path: '/graph',
    shortcut: '⌘2',
    icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-4 h-4"><circle cx="18" cy="5" r="3"></circle><circle cx="6" cy="12" r="3"></circle><circle cx="18" cy="19" r="3"></circle><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"></line><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"></line></svg>',
  },
  {
    name: 'Go to AI Chat',
    path: '/chat',
    shortcut: '⌘3',
    icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-4 h-4"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path></svg>',
  },
  {
    name: 'Go to Archive',
    path: '/archive',
    shortcut: '⌘4',
    icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-4 h-4"><rect width="20" height="5" x="2" y="3" rx="1"/><path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/><path d="M10 12h4"/></svg>',
  },
  {
    name: 'Go to Settings',
    path: '/settings',
    shortcut: '⌘,',
    icon: '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="w-4 h-4"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>',
  },
]

const filteredCommands = computed(() => {
  if (!query.value.trim()) {
    return commands
  }
  const q = query.value.toLowerCase().trim()
  return commands.filter(c => c.name.toLowerCase().includes(q) || c.path.toLowerCase().includes(q))
})

const selectedCommandIndex = ref(0)

const executeCommand = (cmd: CommandItem) => {
  isOpen.value = false
  router.push(cmd.path)
}

const results = ref<SearchResult[]>([])
const isLoading = ref(false)
const selectedIndex = ref(-1)
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
  if (selectedCommandIndex.value >= 0 && selectedCommandIndex.value < filteredCommands.value.length - 1) {
    selectedCommandIndex.value++
  } else if (selectedCommandIndex.value === filteredCommands.value.length - 1) {
    if (results.value.length > 0) {
      selectedCommandIndex.value = -1
      selectedIndex.value = 0
    }
  } else if (selectedIndex.value >= 0 && selectedIndex.value < results.value.length - 1) {
    selectedIndex.value++
  }
}

const selectPrev = () => {
  if (selectedIndex.value > 0) {
    selectedIndex.value--
  } else if (selectedIndex.value === 0) {
    if (filteredCommands.value.length > 0) {
      selectedIndex.value = -1
      selectedCommandIndex.value = filteredCommands.value.length - 1
    }
  } else if (selectedCommandIndex.value > 0) {
    selectedCommandIndex.value--
  }
}

const goToSelected = () => {
  if (selectedCommandIndex.value >= 0 && filteredCommands.value[selectedCommandIndex.value]) {
    executeCommand(filteredCommands.value[selectedCommandIndex.value])
  } else if (selectedIndex.value >= 0 && results.value[selectedIndex.value]) {
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
    if (filteredCommands.value.length > 0) {
      selectedCommandIndex.value = 0
      selectedIndex.value = -1
    } else if (results.value.length > 0) {
      selectedCommandIndex.value = -1
      selectedIndex.value = 0
    } else {
      selectedCommandIndex.value = -1
      selectedIndex.value = -1
    }
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
