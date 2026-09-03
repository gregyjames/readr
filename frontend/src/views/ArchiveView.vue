<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import axios from 'axios'
import { settings, setViewMode as saveGlobalViewMode } from '../store/settings'

interface Article {
  ID: number
  title: string
  article: string
  image: string
  tags: string
  parsedTags: string[]
}

const articles = ref<Article[]>([])
const isLoading = ref(true)
const feedbackMessage = ref<string | null>(null)
let feedbackTimeout: ReturnType<typeof setTimeout> | null = null

type ViewMode = 'card' | 'list' | 'studio' | 'ledger' | 'timeline'
const initialMode = (localStorage.getItem('readr_viewMode') || settings.view_mode || 'card') as ViewMode
const viewMode = ref<ViewMode>(initialMode)

const changeViewMode = (mode: 'card' | 'list') => {
  viewMode.value = mode
  try {
    localStorage.setItem('readr_viewMode', mode)
  } catch {}
  saveGlobalViewMode(mode)
  nextTick(initReveal)
}

const searchQuery = ref('')
const selectedTag = ref<string | null>(null)
const sortOrder = ref<'latest' | 'oldest' | 'title'>('latest')

const failedImages = ref<Record<number, boolean>>({})

const onImageError = (id: number) => {
  failedImages.value[id] = true
}

const hasValidImage = (article: Article | null | undefined) => {
  if (!article || !article.image) return false
  return !failedImages.value[article.ID]
}

const getProceduralGradient = (id: number) => {
  const gradients = [
    'from-emerald-950/60 via-[#121620] to-slate-950',
    'from-blue-950/60 via-[#121620] to-slate-950',
    'from-teal-950/60 via-[#121620] to-zinc-950',
    'from-indigo-950/50 via-[#121620] to-slate-950',
    'from-cyan-950/50 via-[#121620] to-neutral-950',
    'from-amber-950/40 via-[#121620] to-stone-950',
  ]
  return gradients[Math.abs(Number(id) || 0) % gradients.length]
}

const showFeedback = (msg: string) => {
  feedbackMessage.value = msg
  if (feedbackTimeout) clearTimeout(feedbackTimeout)
  feedbackTimeout = setTimeout(() => {
    feedbackMessage.value = null
  }, 3000)
}

const fetchArchivedArticles = async () => {
  isLoading.value = true
  try {
    const res = await axios.get('/api/getarticles?archived=true')
    articles.value = (res.data || []).map((article: any) => ({
      ...article,
      parsedTags: article.tags ? article.tags.split(',').map((tag: string) => tag.trim()) : []
    }))
    await nextTick()
    initReveal()
  } catch (err) {
    console.error('Failed to load archived articles', err)
  } finally {
    isLoading.value = false
  }
}

// Scroll-reveal via IntersectionObserver
let observer: IntersectionObserver | null = null

function initReveal() {
  if (observer) observer.disconnect()
  observer = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          entry.target.classList.add('is-visible')
          observer?.unobserve(entry.target)
        }
      })
    },
    { threshold: 0.1, rootMargin: '0px 0px -20px 0px' }
  )
  document.querySelectorAll('.reveal-item').forEach((el) => observer!.observe(el))
}

function formatDate(unixTs: number): string {
  if (unixTs < 1_000_000_000) return ''
  const ms = unixTs < 100_000_000_000 ? unixTs * 1000 : unixTs
  const d = new Date(ms)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

function formatShortDate(unixTs: number): string {
  if (unixTs < 1_000_000_000) return ''
  const ms = unixTs < 100_000_000_000 ? unixTs * 1000 : unixTs
  const d = new Date(ms)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })
}

function formatTimelineDate(unixTs: number): { month: string; day: string; year: string } {
  if (unixTs < 1_000_000_000) return { month: 'NOTE', day: '•', year: '' }
  const ms = unixTs < 100_000_000_000 ? unixTs * 1000 : unixTs
  const d = new Date(ms)
  const month = d.toLocaleDateString('en-US', { month: 'short' }).toUpperCase()
  const day = d.toLocaleDateString('en-US', { day: '2-digit' })
  const year = d.toLocaleDateString('en-US', { year: 'numeric' })
  return { month, day, year }
}

function getReadingTime(text: string): string {
  if (!text) return '1 min'
  const words = text.trim().split(/\s+/).length
  const minutes = Math.max(1, Math.ceil(words / 200))
  return `${minutes} min read`
}

function getDomain(article: Article): string {
  if (article.image && article.image.startsWith('http')) {
    try {
      return new URL(article.image).hostname.replace(/^www\./, '')
    } catch {
      // ignore
    }
  }
  return 'vault note'
}

const restoreArticle = async (id: number) => {
  try {
    const res = await axios.post(`/api/articles/${id}/unarchive`)
    if (res.status === 200 || res.data?.success) {
      articles.value = articles.value.filter(a => a.ID !== id)
      showFeedback('Article restored to vault')
    }
  } catch (err) {
    console.error('Failed to restore article', err)
    showFeedback('Failed to restore article')
  }
}

const deleteArticlePermanently = async (id: number) => {
  if (!confirm('Are you sure you want to permanently delete this note from the vault?')) return
  try {
    await axios.delete(`/api/delete/${id}`)
    articles.value = articles.value.filter(article => article.ID !== id)
    showFeedback('Article permanently deleted')
  } catch (err) {
    console.error('Failed to permanently delete article', err)
    showFeedback('Failed to delete article')
  }
}

onMounted(async () => {
  const stored = localStorage.getItem('readr_viewMode') as ViewMode | null
  if (stored === 'card' || stored === 'list' || stored === 'studio' || stored === 'ledger') {
    viewMode.value = (stored === 'list' || stored === 'ledger') ? 'list' : 'card'
  } else if (settings.view_mode) {
    viewMode.value = (settings.view_mode === 'list' || settings.view_mode === 'ledger') ? 'list' : 'card'
  }
  await fetchArchivedArticles()
})

onBeforeUnmount(() => {
  if (feedbackTimeout) clearTimeout(feedbackTimeout)
  observer?.disconnect()
})

watch(() => settings.view_mode, (newMode) => {
  if (newMode === 'card' || newMode === 'list' || newMode === 'studio' || newMode === 'ledger') {
    viewMode.value = (newMode === 'list' || newMode === 'ledger') ? 'list' : 'card'
    nextTick(initReveal)
  }
})

watch([viewMode, sortOrder, selectedTag, searchQuery], () => {
  nextTick(initReveal)
})

const filteredArticles = computed(() => {
  let list = [...articles.value]

  // Filter by search query
  if (searchQuery.value.trim()) {
    const q = searchQuery.value.toLowerCase().trim()
    list = list.filter(a =>
      (a.title && a.title.toLowerCase().includes(q)) ||
      (a.article && a.article.toLowerCase().includes(q)) ||
      (a.tags && a.tags.toLowerCase().includes(q))
    )
  }

  // Filter by tag
  if (selectedTag.value) {
    list = list.filter(a => a.parsedTags.includes(selectedTag.value!))
  }

  // Sorting
  if (sortOrder.value === 'latest') {
    list.sort((a, b) => b.ID - a.ID)
  } else if (sortOrder.value === 'oldest') {
    list.sort((a, b) => a.ID - b.ID)
  } else if (sortOrder.value === 'title') {
    list.sort((a, b) => a.title.localeCompare(b.title))
  }

  return list
})

const leadArticle = computed(() => {
  if (filteredArticles.value.length === 0) return null
  return filteredArticles.value[0]
})

const secondaryArticles = computed(() => {
  if (filteredArticles.value.length <= 1) return []
  return filteredArticles.value.slice(1)
})
</script>

<template>
  <div class="max-w-6xl mx-auto px-5 sm:px-8 py-8 md:py-10 space-y-8">
    
    <!-- Feedback Toast -->
    <transition name="fade">
      <div
        v-if="feedbackMessage"
        class="fixed bottom-6 right-6 z-50 px-4 py-2.5 rounded-xl bg-gray-950/90 dark:bg-white/95 text-white dark:text-gray-950 text-xs font-mono font-medium shadow-lg backdrop-blur-md flex items-center gap-2 border border-white/10 dark:border-black/10"
      >
        <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
        <span>{{ feedbackMessage }}</span>
      </div>
    </transition>

    <!-- Clean Header -->
    <header class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-5 border-b border-black/[0.06] dark:border-white/[0.06]">
      
      <!-- Left: Title & Counter -->
      <div class="flex items-center gap-4">
        <div class="flex items-center gap-2.5">
          <div class="p-2 rounded-xl bg-amber-500/10 text-amber-600 dark:text-amber-400">
            <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect width="20" height="5" x="2" y="3" rx="1"/>
              <path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/>
              <path d="M10 12h4"/>
            </svg>
          </div>
          <div>
            <div class="flex items-baseline gap-2">
              <h1 class="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-100 font-['Outfit']">
                Archive
              </h1>
              <span class="text-xs font-mono text-gray-400 dark:text-gray-500 font-medium">
                {{ filteredArticles.length }}
              </span>
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400">Archived notes from your vault</p>
          </div>
        </div>
      </div>

      <!-- Right: Search, Sort & View Controls -->
      <div class="flex flex-wrap items-center gap-2">
        
        <!-- Search input -->
        <div class="relative flex items-center">
          <svg class="w-3.5 h-3.5 absolute left-2.5 text-gray-400 pointer-events-none" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="11" cy="11" r="8"/>
            <line x1="21" y1="21" x2="16.65" y2="16.65"/>
          </svg>
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Filter archive..."
            class="pl-8 pr-3 py-1 text-xs bg-gray-100/70 dark:bg-white/[0.04] border border-gray-200/50 dark:border-white/[0.05] rounded-lg focus:outline-none focus:ring-1 focus:ring-emerald-500 text-gray-900 dark:text-gray-100 placeholder:text-gray-400 w-36 sm:w-44 transition-all"
          />
          <button
            v-if="searchQuery"
            @click="searchQuery = ''"
            class="absolute right-2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 text-xs"
          >&times;</button>
        </div>

        <!-- Sort Control -->
        <div class="flex items-center bg-gray-100/70 dark:bg-white/[0.04] p-0.5 rounded-lg border border-gray-200/50 dark:border-white/[0.05]">
          <button
            @click="sortOrder = sortOrder === 'latest' ? 'oldest' : (sortOrder === 'oldest' ? 'title' : 'latest')"
            class="px-2.5 py-1 text-xs font-medium text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white transition-colors cursor-pointer flex items-center gap-1"
            :title="`Sort order: ${sortOrder}`"
          >
            <span>{{ sortOrder === 'latest' ? 'Latest' : (sortOrder === 'oldest' ? 'Oldest' : 'Title') }}</span>
            <svg class="w-3 h-3 text-gray-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="m6 9 6 6 6-6"/>
            </svg>
          </button>
        </div>

        <!-- View Mode Switcher -->
        <div class="flex items-center bg-gray-100/70 dark:bg-white/[0.04] p-0.5 rounded-lg border border-gray-200/50 dark:border-white/[0.05]">
          <button
            @click="changeViewMode('card')"
            class="p-1.5 rounded-md transition-all cursor-pointer"
            :class="(viewMode === 'card' || viewMode === 'studio') ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'"
            title="Card View"
          >
            <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect width="7" height="7" x="3" y="3" rx="1"></rect>
              <rect width="7" height="7" x="14" y="3" rx="1"></rect>
              <rect width="7" height="7" x="14" y="14" rx="1"></rect>
              <rect width="7" height="7" x="3" y="14" rx="1"></rect>
            </svg>
          </button>
          <button
            @click="changeViewMode('list')"
            class="p-1.5 rounded-md transition-all cursor-pointer"
            :class="(viewMode === 'list' || viewMode === 'ledger') ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'"
            title="Timeline Stream"
          >
            <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="6" y1="3" x2="6" y2="21"></line>
              <circle cx="6" cy="8" r="2" fill="currentColor"></circle>
              <circle cx="6" cy="16" r="2" fill="currentColor"></circle>
              <line x1="12" y1="8" x2="20" y2="8"></line>
              <line x1="12" y1="16" x2="18" y2="16"></line>
            </svg>
          </button>
        </div>
      </div>

    </header>

    <!-- Active Tag Filter Sub-banner (only if tag selected) -->
    <div v-if="selectedTag" class="flex items-center gap-2 -mt-4">
      <span class="text-xs text-gray-400 font-mono">Filter:</span>
      <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-md bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 text-xs font-mono font-medium border border-emerald-500/20">
        #{{ selectedTag }}
        <button @click="selectedTag = null" class="hover:text-emerald-900 dark:hover:text-white cursor-pointer">&times;</button>
      </span>
    </div>

    <!-- Empty Archive State -->
    <div v-if="filteredArticles.length === 0 && !isLoading" class="flex flex-col items-center justify-center py-32 px-4 text-center">
      <div class="w-12 h-12 bg-gray-100 dark:bg-white/[0.05] rounded-xl flex items-center justify-center mb-4 border border-gray-200/80 dark:border-white/[0.08]">
        <svg class="w-6 h-6 text-gray-400 dark:text-gray-500" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <rect width="20" height="5" x="2" y="3" rx="1"/>
          <path d="M4 8v11a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8"/>
          <path d="M10 12h4"/>
        </svg>
      </div>
      <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100 mb-1">
        {{ searchQuery || selectedTag ? 'No matching archived notes' : 'No archived articles' }}
      </h3>
      <p class="text-xs text-gray-500 dark:text-gray-400 max-w-sm leading-relaxed font-mono">
        {{ searchQuery || selectedTag ? 'Try adjusting your search query or tag filter.' : 'Notes you archive from the home screen will appear here.' }}
      </p>
    </div>

    <!-- Loading State -->
    <div v-else-if="isLoading" class="flex flex-col items-center justify-center py-32 px-4 text-center font-mono text-xs text-gray-400">
      Loading archived vault notes...
    </div>

    <!-- VIEW MODE 1: CARD VIEW -->
    <div v-else-if="viewMode === 'card' || viewMode === 'studio'" class="space-y-8">
      
      <!-- Lead Spotlight Card -->
      <div v-if="leadArticle" class="reveal-item">
        <div class="relative group bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 transition-all duration-300 overflow-hidden shadow-xs hover:shadow-md">
          
          <div class="flex flex-col lg:flex-row items-stretch">
            
            <!-- Left Text Content -->
            <div class="flex-1 p-6 sm:p-8 flex flex-col justify-between">
              <div>
                <!-- Index & Meta -->
                <div class="flex flex-wrap items-center gap-2.5 text-xs font-mono text-gray-400 dark:text-gray-500 mb-3">
                  <span class="text-amber-600 dark:text-amber-400 font-semibold tracking-wide">// ARCHIVED</span>
                  <span>•</span>
                  <span class="px-2 py-0.5 rounded-full bg-gray-100 dark:bg-white/[0.05] text-[10px] text-gray-600 dark:text-gray-300 font-mono font-medium">{{ getDomain(leadArticle) }}</span>
                  <span>•</span>
                  <span>{{ formatDate(leadArticle.ID) || 'Recent' }}</span>
                  <span>•</span>
                  <span>{{ getReadingTime(leadArticle.article) }}</span>
                </div>

                <!-- Tags -->
                <div v-if="leadArticle.parsedTags.length > 0" class="flex flex-wrap items-center gap-1.5 mb-3">
                  <button
                    v-for="tag in leadArticle.parsedTags.slice(0, 4)"
                    :key="tag"
                    @click="selectedTag = tag"
                    class="text-[11px] font-mono px-2 py-0.5 rounded bg-gray-100 dark:bg-white/[0.05] hover:bg-emerald-500/10 hover:text-emerald-700 dark:hover:text-emerald-300 text-gray-600 dark:text-gray-400 transition-colors cursor-pointer"
                  >
                    #{{ tag }}
                  </button>
                </div>

                <!-- Title -->
                <h2 class="text-xl sm:text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors mb-3 leading-snug font-['Outfit']">
                  <router-link :to="`/articles/${leadArticle.ID}`">
                    {{ leadArticle.title }}
                  </router-link>
                </h2>

                <!-- Excerpt -->
                <p class="text-xs sm:text-sm text-gray-600 dark:text-gray-400 line-clamp-3 leading-relaxed max-w-[65ch]">
                  {{ leadArticle.article }}
                </p>
              </div>

              <!-- Action Bar -->
              <div class="flex items-center justify-between pt-6 mt-6 border-t border-gray-100 dark:border-white/[0.04]">
                <router-link
                  :to="`/articles/${leadArticle.ID}`"
                  class="inline-flex items-center gap-1.5 text-xs font-mono font-medium text-emerald-600 dark:text-emerald-400 group-hover:translate-x-0.5 transition-transform"
                >
                  <span>Open article</span>
                  <span>&rarr;</span>
                </router-link>

                <div class="flex items-center gap-2">
                  <button
                    @click="restoreArticle(leadArticle.ID)"
                    class="inline-flex items-center gap-1 px-2.5 py-1 rounded-lg bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 text-xs font-mono font-medium transition-colors cursor-pointer"
                    title="Restore article to vault"
                  >
                    <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="9 14 4 9 9 4"/>
                      <path d="M20 20v-7a4 4 0 0 0-4-4H4"/>
                    </svg>
                    <span>Restore</span>
                  </button>

                  <button
                    @click="deleteArticlePermanently(leadArticle.ID)"
                    class="text-gray-400 hover:text-red-500 p-1 rounded transition-colors text-xs font-mono cursor-pointer"
                    title="Permanently delete article"
                  >
                    Delete Permanently
                  </button>
                </div>
              </div>
            </div>

            <!-- Right Cover Media -->
            <div
              v-if="hasValidImage(leadArticle)"
              class="lg:w-96 xl:w-[420px] h-56 lg:h-auto overflow-hidden bg-gray-100 dark:bg-[#0A0C10] border-t lg:border-t-0 lg:border-l border-gray-100 dark:border-white/[0.06] flex-shrink-0 relative"
            >
              <img
                :src="leadArticle.image"
                :alt="leadArticle.title"
                @error="onImageError(leadArticle.ID)"
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-700 ease-out grayscale-[30%] group-hover:grayscale-0"
                loading="lazy"
              />
              <div class="absolute inset-0 bg-gradient-to-t from-black/40 via-transparent to-transparent pointer-events-none"></div>
            </div>
            <div
              v-else
              class="lg:w-80 h-44 lg:h-auto overflow-hidden p-6 flex flex-col justify-between border-t lg:border-t-0 lg:border-l border-gray-100 dark:border-white/[0.06] flex-shrink-0 relative bg-gradient-to-br"
              :class="getProceduralGradient(leadArticle.ID)"
            >
              <div class="absolute inset-0 opacity-10 bg-[radial-gradient(#fff_1px,transparent_1px)] [background-size:14px_14px]"></div>
              <div class="relative z-10 flex justify-end">
                <span class="px-2 py-0.5 rounded-full text-[10px] font-mono text-white/70 bg-white/10 backdrop-blur-md border border-white/10">Archived</span>
              </div>
              <div class="relative z-10 font-mono text-4xl font-black text-white/10 uppercase select-none">
                #{{ leadArticle.ID }}
              </div>
            </div>

          </div>
        </div>
      </div>

      <!-- Secondary Articles Grid -->
      <div v-if="secondaryArticles.length > 0" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        <article
          v-for="article in secondaryArticles"
          :key="article.ID"
          class="reveal-item group relative bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 transition-all duration-300 shadow-2xs hover:shadow-md overflow-hidden flex flex-col justify-between"
        >
          <!-- Top Cover Media -->
          <div class="relative h-48 w-full overflow-hidden bg-gray-100 dark:bg-[#0A0C10] border-b border-gray-100 dark:border-white/[0.06]">
            <template v-if="hasValidImage(article)">
              <img
                :src="article.image"
                :alt="article.title"
                @error="onImageError(article.ID)"
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500 ease-out grayscale-[30%] group-hover:grayscale-0"
                loading="lazy"
              />
              <div class="absolute inset-0 bg-gradient-to-t from-black/60 via-black/15 to-black/20 pointer-events-none"></div>
            </template>

            <div
              v-else
              class="w-full h-full p-4 flex flex-col justify-between relative bg-gradient-to-br"
              :class="getProceduralGradient(article.ID)"
            >
              <div class="absolute inset-0 opacity-10 bg-[radial-gradient(#fff_1px,transparent_1px)] [background-size:12px_12px]"></div>
              <div class="relative z-10 flex items-center justify-between">
                <span class="px-2 py-0.5 rounded-full text-[10px] font-mono font-medium bg-black/40 dark:bg-white/10 backdrop-blur-md text-white/90 border border-white/10 flex items-center gap-1">
                  <span class="w-1 h-1 rounded-full bg-amber-400"></span>
                  {{ getDomain(article) }}
                </span>
                <span class="text-[10px] font-mono text-white/70">
                  {{ getReadingTime(article.article) }}
                </span>
              </div>
              <div class="relative z-10 flex items-baseline justify-between select-none">
                <span class="text-3xl font-bold tracking-tighter text-white/10 dark:text-white/[0.08] font-mono">
                  #{{ article.ID }}
                </span>
              </div>
            </div>

            <!-- Overlay Badges -->
            <div v-if="hasValidImage(article)" class="absolute inset-x-3 top-3 flex items-center justify-between pointer-events-none z-10">
              <span class="px-2.5 py-0.5 rounded-full text-[10px] font-mono font-medium bg-black/60 backdrop-blur-md text-white/95 border border-white/15 shadow-xs flex items-center gap-1.5">
                <span class="w-1 h-1 rounded-full bg-amber-400"></span>
                {{ getDomain(article) }}
              </span>
              <span class="px-2 py-0.5 rounded-full text-[10px] font-mono bg-black/60 backdrop-blur-md text-white/85 border border-white/15 shadow-xs">
                {{ getReadingTime(article.article) }}
              </span>
            </div>

            <div v-if="hasValidImage(article)" class="absolute inset-x-3 bottom-2.5 flex items-center justify-between pointer-events-none z-10">
              <span class="text-[10px] font-mono text-white/90 font-medium drop-shadow-sm">
                {{ formatShortDate(article.ID) || `ID #${article.ID}` }}
              </span>
            </div>
          </div>

          <!-- Card Content Body -->
          <div class="p-5 flex flex-col justify-between flex-1">
            <div>
              <!-- Tags -->
              <div v-if="article.parsedTags.length > 0" class="flex flex-wrap items-center gap-1.5 mb-2.5">
                <button
                  v-for="tag in article.parsedTags.slice(0, 3)"
                  :key="tag"
                  @click="selectedTag = tag"
                  class="text-[10px] font-mono px-2 py-0.5 rounded-md bg-gray-100 dark:bg-white/[0.04] text-gray-600 dark:text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 hover:bg-emerald-500/10 transition-colors cursor-pointer"
                >
                  #{{ tag }}
                </button>
              </div>

              <!-- Title -->
              <h3 class="text-base font-semibold tracking-tight text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors mb-2 line-clamp-2 leading-snug font-['Outfit']">
                <router-link :to="`/articles/${article.ID}`">
                  {{ article.title }}
                </router-link>
              </h3>

              <!-- Excerpt -->
              <p class="text-xs text-gray-500 dark:text-gray-400 line-clamp-2 leading-relaxed mb-4">
                {{ article.article }}
              </p>
            </div>

            <!-- Footer Action Bar -->
            <div class="pt-3 border-t border-gray-100 dark:border-white/[0.04] flex items-center justify-between text-[11px] font-mono text-gray-400 dark:text-gray-500">
              <button
                @click="restoreArticle(article.ID)"
                class="p-1 rounded-md text-emerald-600 dark:text-emerald-400 hover:bg-emerald-500/10 transition-colors cursor-pointer"
                title="Restore note to vault"
                aria-label="Restore note to vault"
              >
                <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="9 14 4 9 9 4"/>
                  <path d="M20 20v-7a4 4 0 0 0-4-4H4"/>
                </svg>
              </button>
              <div class="flex items-center gap-1">
                <button
                  @click="deleteArticlePermanently(article.ID)"
                  class="p-1 rounded-md text-gray-400 hover:text-red-500 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-colors cursor-pointer"
                  title="Permanently delete note"
                  aria-label="Permanently delete note"
                >
                  <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="3 6 5 6 21 6"></polyline>
                    <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                  </svg>
                </button>
              </div>
            </div>
          </div>
        </article>
      </div>

    </div>

    <!-- VIEW MODE 2: TIMELINE STREAM -->
    <div v-else class="max-w-4xl mx-auto relative pl-6 sm:pl-32 py-4">

      <!-- Spine -->
      <div class="absolute left-6 sm:left-[5.5rem] top-3 bottom-6 w-[2px] bg-gradient-to-b from-amber-500/50 via-gray-200/80 dark:via-white/[0.08] to-transparent pointer-events-none"></div>

      <div class="space-y-6 sm:space-y-8">
        <div
          v-for="(article, idx) in filteredArticles"
          :key="article.ID"
          class="reveal-item relative group"
        >
          <!-- Desktop Timestamp -->
          <div class="hidden sm:flex absolute -left-[5.5rem] top-5 w-20 flex-col items-end pr-4 text-right select-none">
            <span class="text-[10px] font-mono font-bold tracking-widest text-amber-600 dark:text-amber-400 uppercase">
              {{ formatTimelineDate(article.ID).month }}
            </span>
            <span class="text-xl font-bold tracking-tight text-gray-900 dark:text-gray-100 font-['Outfit'] -my-0.5">
              {{ formatTimelineDate(article.ID).day }}
            </span>
            <span class="text-[10px] font-mono text-gray-400 dark:text-gray-500">
              {{ formatTimelineDate(article.ID).year }}
            </span>
          </div>

          <!-- Interactive Node Pin -->
          <div class="absolute -left-[1.375rem] top-7 -translate-x-1/2 w-4 h-4 rounded-full bg-[#FAFAFA] dark:bg-[#0C0E12] border-2 border-gray-300 dark:border-white/20 group-hover:border-amber-500 group-hover:scale-125 transition-all duration-300 flex items-center justify-center z-10 shadow-xs">
            <div class="w-1.5 h-1.5 rounded-full bg-gray-400 dark:bg-white/40 group-hover:bg-amber-500 group-hover:shadow-[0_0_10px_rgba(245,158,11,0.9)] transition-all duration-300"></div>
          </div>

          <!-- Capsule -->
          <div class="ml-2 sm:ml-4 relative bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 hover:shadow-xl transition-all duration-300 p-4 sm:p-5 flex flex-col md:flex-row items-start md:items-center gap-4 sm:gap-5 group-hover:-translate-y-0.5">
            
            <!-- Thumbnail -->
            <div class="w-full md:w-36 h-36 md:h-24 rounded-xl overflow-hidden bg-gray-100 dark:bg-[#0A0C10] border border-gray-100 dark:border-white/[0.06] flex-shrink-0 relative group/thumb">
              <template v-if="hasValidImage(article)">
                <img
                  :src="article.image"
                  :alt="article.title"
                  @error="onImageError(article.ID)"
                  class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500 ease-out grayscale-[30%]"
                  loading="lazy"
                />
                <div class="absolute inset-0 bg-gradient-to-t from-black/50 via-transparent to-transparent pointer-events-none"></div>
              </template>
              <div
                v-else
                class="w-full h-full p-3 flex flex-col justify-between relative bg-gradient-to-br"
                :class="getProceduralGradient(article.ID)"
              >
                <div class="absolute inset-0 opacity-10 bg-[radial-gradient(#fff_1px,transparent_1px)] [background-size:10px_10px]"></div>
                <span class="relative z-10 text-[9px] font-mono text-amber-400/70 font-semibold tracking-wider uppercase truncate">
                  {{ getDomain(article).split('.')[0] }}
                </span>
                <span class="relative z-10 font-mono text-2xl font-bold text-white/10 select-none">
                  #{{ String(idx + 1).padStart(2, '0') }}
                </span>
              </div>
            </div>

            <!-- Content Details -->
            <div class="flex-grow min-w-0 pr-2">
              <div class="flex flex-wrap items-center gap-2 mb-1.5 text-xs font-mono">
                <span class="sm:hidden text-amber-600 dark:text-amber-400 font-semibold">
                  {{ formatShortDate(article.ID) }}
                </span>
                <span class="sm:hidden text-gray-300 dark:text-gray-600">•</span>

                <span class="px-2 py-0.5 rounded-full text-[10px] bg-gray-100 dark:bg-white/[0.05] text-gray-600 dark:text-gray-300 border border-gray-200/60 dark:border-white/[0.06] flex items-center gap-1.5 font-medium">
                  <span class="w-1 h-1 rounded-full bg-amber-500"></span>
                  {{ getDomain(article) }}
                </span>

                <span class="text-[11px] text-gray-400 dark:text-gray-500 font-mono">
                  {{ getReadingTime(article.article) }}
                </span>

                <button
                  v-for="tag in article.parsedTags.slice(0, 2)"
                  :key="tag"
                  @click="selectedTag = tag"
                  class="text-[10px] font-mono px-1.5 py-0.5 rounded-md bg-gray-100/70 dark:bg-white/[0.03] text-gray-500 dark:text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 hover:bg-emerald-500/10 transition-colors cursor-pointer"
                >
                  #{{ tag }}
                </button>
              </div>

              <h3 class="text-base sm:text-lg font-semibold tracking-tight text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors mb-1.5 leading-snug font-['Outfit']">
                <router-link :to="`/articles/${article.ID}`">
                  {{ article.title }}
                </router-link>
              </h3>

              <p class="text-xs text-gray-500 dark:text-gray-400 line-clamp-2 leading-relaxed font-normal">
                {{ article.article }}
              </p>
            </div>

            <!-- Trailing Interactive Actions -->
            <div class="flex items-center gap-3 self-end md:self-center flex-shrink-0 pt-2 md:pt-0">
              <button
                @click="restoreArticle(article.ID)"
                class="inline-flex items-center gap-1 px-3 py-2 rounded-xl bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-700 dark:text-emerald-300 text-xs font-mono font-medium transition-all cursor-pointer"
                title="Restore note to vault"
              >
                <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="9 14 4 9 9 4"/>
                  <path d="M20 20v-7a4 4 0 0 0-4-4H4"/>
                </svg>
                <span class="hidden sm:inline">Restore</span>
              </button>

              <button
                @click="deleteArticlePermanently(article.ID)"
                class="hover:text-red-500 text-gray-400 p-2 rounded-lg transition-all cursor-pointer text-xs font-mono"
                title="Permanently delete note"
              >
                <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="3 6 5 6 21 6"></polyline>
                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                </svg>
              </button>

              <router-link
                :to="`/articles/${article.ID}`"
                class="w-10 h-10 rounded-xl bg-gray-100 dark:bg-white/[0.05] group-hover:bg-emerald-500 text-gray-400 group-hover:text-white transition-all duration-300 flex items-center justify-center shadow-2xs group-hover:shadow-emerald-500/25 group-hover:scale-105 active:scale-95"
                title="Read article"
              >
                <svg class="w-4 h-4 group-hover:translate-x-0.5 transition-transform" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="5" y1="12" x2="19" y2="12"></line>
                  <polyline points="12 5 19 12 12 19"></polyline>
                </svg>
              </router-link>
            </div>

          </div>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
/* ─── Scroll reveal ──────────────────────────────────── */
.reveal-item {
  opacity: 0;
  transform: translateY(22px);
  transition: opacity 0.55s ease, transform 0.55s cubic-bezier(0.22, 1, 0.36, 1);
}

.reveal-item.is-visible {
  opacity: 1;
  transform: translateY(0);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
