<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import axios from 'axios'
import emitter from '../event-bus.ts'
import BookmarkIcon from '../assets/book.svg'
import { settings, setViewMode as saveGlobalViewMode } from '../store/settings'

interface Article {
  ID: number
  title: string
  article: string
  image: string
  tags: string
  parsedTags: string[]
}

defineProps<{ msg?: string }>()

const articles = ref<Article[]>([])

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

const selectedTag = ref<string | null>(null)
const filterMocOnly = ref(false)
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

const fetchArticles = async () => {
  try {
    const res = await axios.get('/api/getarticles')
    articles.value = res.data.map((article: any) => ({
      ...article,
      parsedTags: article.tags ? article.tags.split(',').map((tag: string) => tag.trim()) : []
    }))
    await nextTick()
    initReveal()
  } catch (err) {
    console.error('Failed to load articles', err)
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

function getReadingTime(text: string): string {
  if (!text) return '1 min'
  const words = text.trim().split(/\s+/).length
  const minutes = Math.max(1, Math.ceil(words / 200))
  return `${minutes} min read`
}

function isMocArticle(article: Article): boolean {
  if (!article) return false
  const title = (article.title || '').toLowerCase().trim()
  if (title.startsWith('moc -') || title.startsWith('moc:') || title.startsWith('moc ') || title === 'moc') {
    return true
  }
  return article.parsedTags.some(t => t.toLowerCase() === 'moc')
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


const deleteArticle = async (id: number) => {
  if (!confirm('Are you sure you want to permanently delete this note from the vault?')) return
  try {
    await axios.delete(`/api/delete/${id}`)
    articles.value = articles.value.filter(article => article.ID !== id)
  } catch (err) {
    console.error('Failed to delete article', err)
  }
}

onMounted(async () => {
  const stored = localStorage.getItem('readr_viewMode') as ViewMode | null
  if (stored === 'card' || stored === 'list' || stored === 'studio' || stored === 'ledger') {
    viewMode.value = (stored === 'list' || stored === 'ledger') ? 'list' : 'card'
  } else if (settings.view_mode) {
    viewMode.value = (settings.view_mode === 'list' || settings.view_mode === 'ledger') ? 'list' : 'card'
  }
  await fetchArticles()
  emitter.on('article-added', fetchArticles)
})

onBeforeUnmount(() => {
  emitter.off('article-added', fetchArticles)
  observer?.disconnect()
})

watch(() => settings.view_mode, (newMode) => {
  if (newMode === 'card' || newMode === 'list' || newMode === 'studio' || newMode === 'ledger') {
    viewMode.value = (newMode === 'list' || newMode === 'ledger') ? 'list' : 'card'
    nextTick(initReveal)
  }
})

watch([viewMode, sortOrder, selectedTag, filterMocOnly], () => {
  nextTick(initReveal)
})

const totalMocs = computed(() => articles.value.filter(isMocArticle).length)
const totalNotes = computed(() => articles.value.filter(a => !isMocArticle(a)).length)

const filteredArticles = computed(() => {
  let list = [...articles.value]

  // Filter by tag
  if (selectedTag.value) {
    list = list.filter(a => a.parsedTags.includes(selectedTag.value!))
  }

  // Filter MOCs: Hubs tab shows ONLY MOCs; Notes tab shows ONLY non-MOC articles
  if (filterMocOnly.value) {
    list = list.filter(isMocArticle)
  } else {
    list = list.filter(a => !isMocArticle(a))
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
    
    <!-- Clean Minimalist Studio Header -->
    <header class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 pb-5 border-b border-black/[0.06] dark:border-white/[0.06]">
      
      <!-- Left: Title & Filter Tabs -->
      <div class="flex items-center gap-5">
        <div class="flex items-baseline gap-2.5">
          <h1 class="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-100 font-['Outfit']">
            Vault
          </h1>
          <span class="text-xs font-mono text-gray-400 dark:text-gray-500 font-medium">
            {{ filteredArticles.length }}
          </span>
        </div>

        <!-- Filter Tabs: Notes vs Hubs -->
        <div class="flex items-center bg-gray-100/70 dark:bg-white/[0.04] p-0.5 rounded-lg border border-gray-200/50 dark:border-white/[0.05]">
          <button
            @click="filterMocOnly = false"
            :class="[
              'px-3 py-1 text-xs font-medium rounded-md transition-all cursor-pointer flex items-center gap-1.5',
              !filterMocOnly
                ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs'
                : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
            ]"
          >
            <span>Notes</span>
            <span class="text-[10px] font-mono opacity-70 ml-0.5">{{ totalNotes }}</span>
          </button>
          <button
            @click="filterMocOnly = true"
            :class="[
              'px-3 py-1 text-xs font-medium rounded-md transition-all cursor-pointer flex items-center gap-1.5',
              filterMocOnly
                ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs'
                : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'
            ]"
          >
            <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
            <span>Hubs</span>
            <span class="text-[10px] font-mono opacity-70 ml-0.5">{{ totalMocs }}</span>
          </button>
        </div>
      </div>

      <!-- Right: Sort & View Controls -->
      <div class="flex items-center gap-2">

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
            title="List View"
          >
            <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="3" y1="6" x2="21" y2="6"></line>
              <line x1="3" y1="12" x2="21" y2="12"></line>
              <line x1="3" y1="18" x2="21" y2="18"></line>
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

    <!-- Empty Vault State -->
    <div v-if="filteredArticles.length === 0" class="flex flex-col items-center justify-center py-32 px-4 text-center">
      <div class="w-12 h-12 bg-gray-100 dark:bg-white/[0.05] rounded-xl flex items-center justify-center mb-4 border border-gray-200/80 dark:border-white/[0.08]">
        <BookmarkIcon class="w-6 h-6 text-gray-400 dark:text-gray-500" />
      </div>
      <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100 mb-1">No notes found</h3>
      <p class="text-xs text-gray-500 dark:text-gray-400 max-w-xs leading-relaxed font-mono">
        {{ selectedTag ? 'No notes have the selected tag.' : (filterMocOnly ? 'No MOC Hubs found in vault.' : 'Your vault is ready. Ingest your first article to begin.') }}
      </p>
    </div>

    <!-- VIEW MODE 1: CARD VIEW -->
    <div v-else-if="viewMode === 'card' || viewMode === 'studio'" class="space-y-8">
      
      <!-- Lead Spotlight Card (Hero Ingestion) -->
      <div v-if="leadArticle" class="reveal-item">
        <div class="relative group bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 transition-all duration-300 overflow-hidden shadow-xs hover:shadow-md">
          
          <div class="flex flex-col lg:flex-row items-stretch">
            
            <!-- Left Editorial Text Content -->
            <div class="flex-1 p-6 sm:p-8 flex flex-col justify-between">
              <div>
                <!-- Editorial Index & Meta -->
                <div class="flex flex-wrap items-center gap-2.5 text-xs font-mono text-gray-400 dark:text-gray-500 mb-3">
                  <span class="text-emerald-600 dark:text-emerald-400 font-semibold tracking-wide">// 01 · LATEST NOTE</span>
                  <span>•</span>
                  <span class="px-2 py-0.5 rounded-full bg-gray-100 dark:bg-white/[0.05] text-[10px] text-gray-600 dark:text-gray-300 font-mono font-medium">{{ getDomain(leadArticle) }}</span>
                  <span>•</span>
                  <span>{{ formatDate(leadArticle.ID) || 'Recent' }}</span>
                  <span>•</span>
                  <span>{{ getReadingTime(leadArticle.article) }}</span>
                </div>

                <!-- Tags / MOC Badge -->
                <div class="flex flex-wrap items-center gap-1.5 mb-3">
                  <span
                    v-if="isMocArticle(leadArticle)"
                    class="px-2 py-0.5 rounded text-[11px] font-mono bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-500/20 font-medium"
                  >
                    ★ MOC HUB
                  </span>
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

                <button
                  @click="deleteArticle(leadArticle.ID)"
                  class="text-gray-400 hover:text-red-500 p-1 rounded transition-colors text-xs font-mono cursor-pointer"
                  title="Delete article"
                >
                  Delete
                </button>
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
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-700 ease-out"
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
                <span class="px-2 py-0.5 rounded-full text-[10px] font-mono text-white/70 bg-white/10 backdrop-blur-md border border-white/10">Readr Ingest</span>
              </div>
              <div class="relative z-10 font-mono text-4xl font-black text-white/10 uppercase select-none">
                #01
              </div>
            </div>

          </div>
        </div>
      </div>

      <!-- Secondary Articles Masonry / Grid -->
      <div v-if="secondaryArticles.length > 0" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
        <article
          v-for="(article, idx) in secondaryArticles"
          :key="article.ID"
          class="reveal-item group relative bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 transition-all duration-300 shadow-2xs hover:shadow-md overflow-hidden flex flex-col justify-between"
        >
          <!-- Top Cover Media -->
          <div class="relative h-48 w-full overflow-hidden bg-gray-100 dark:bg-[#0A0C10] border-b border-gray-100 dark:border-white/[0.06]">
            <!-- Actual Image -->
            <template v-if="hasValidImage(article)">
              <img
                :src="article.image"
                :alt="article.title"
                @error="onImageError(article.ID)"
                class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500 ease-out"
                loading="lazy"
              />
              <!-- Subtle Cinematic Scrim -->
              <div class="absolute inset-0 bg-gradient-to-t from-black/60 via-black/15 to-black/20 pointer-events-none"></div>
            </template>

            <!-- Generative Curated Fallback Banner -->
            <div
              v-else
              class="w-full h-full p-4 flex flex-col justify-between relative bg-gradient-to-br"
              :class="getProceduralGradient(article.ID)"
            >
              <div class="absolute inset-0 opacity-10 bg-[radial-gradient(#fff_1px,transparent_1px)] [background-size:12px_12px]"></div>
              <div class="relative z-10 flex items-center justify-between">
                <span class="px-2 py-0.5 rounded-full text-[10px] font-mono font-medium bg-black/40 dark:bg-white/10 backdrop-blur-md text-white/90 border border-white/10 flex items-center gap-1">
                  <span class="w-1 h-1 rounded-full bg-emerald-400"></span>
                  {{ getDomain(article) }}
                </span>
                <span class="text-[10px] font-mono text-white/70">
                  {{ getReadingTime(article.article) }}
                </span>
              </div>
              <div class="relative z-10 flex items-baseline justify-between select-none">
                <span class="text-3xl font-bold tracking-tighter text-white/10 dark:text-white/[0.08] font-mono">
                  #{{ String(idx + 2).padStart(2, '0') }}
                </span>
                <span class="text-xs font-mono uppercase tracking-widest text-emerald-400/30 font-semibold">
                  {{ getDomain(article).split('.')[0].slice(0, 10) }}
                </span>
              </div>
            </div>

            <!-- Floating Overlay Badges (When Image Is Present) -->
            <div v-if="hasValidImage(article)" class="absolute inset-x-3 top-3 flex items-center justify-between pointer-events-none z-10">
              <span class="px-2.5 py-0.5 rounded-full text-[10px] font-mono font-medium bg-black/60 backdrop-blur-md text-white/95 border border-white/15 shadow-xs flex items-center gap-1.5">
                <span class="w-1 h-1 rounded-full bg-emerald-400"></span>
                {{ getDomain(article) }}
              </span>
              <span class="px-2 py-0.5 rounded-full text-[10px] font-mono bg-black/60 backdrop-blur-md text-white/85 border border-white/15 shadow-xs">
                {{ getReadingTime(article.article) }}
              </span>
            </div>

            <!-- Inset Bottom Bar over Image -->
            <div v-if="hasValidImage(article)" class="absolute inset-x-3 bottom-2.5 flex items-center justify-between pointer-events-none z-10">
              <span class="text-[10px] font-mono text-white/90 font-medium drop-shadow-sm">
                {{ formatShortDate(article.ID) || `ID #${article.ID}` }}
              </span>
              <span class="text-[10px] font-mono text-emerald-300 font-semibold drop-shadow-sm group-hover:translate-x-0.5 transition-transform">
                Read &rarr;
              </span>
            </div>
          </div>

          <!-- Card Content Body -->
          <div class="p-5 flex flex-col justify-between flex-1">
            <div>
              <!-- Tags / MOC Hub Indicator -->
              <div v-if="article.parsedTags.length > 0 || isMocArticle(article)" class="flex flex-wrap items-center gap-1.5 mb-2.5">
                <span
                  v-if="isMocArticle(article)"
                  class="text-[10px] font-mono px-2 py-0.5 rounded bg-amber-500/10 text-amber-700 dark:text-amber-300 font-medium border border-amber-500/20"
                >
                  ★ MOC
                </span>
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

            <!-- Footer Action & Meta -->
            <div class="pt-3 border-t border-gray-100 dark:border-white/[0.04] flex items-center justify-between text-[11px] font-mono text-gray-400 dark:text-gray-500">
              <span class="flex items-center gap-1.5">
                <span class="w-1 h-1 rounded-full bg-emerald-500/70"></span>
                <span>// {{ String(idx + 2).padStart(2, '0') }}</span>
              </span>
              <div class="flex items-center gap-3">
                <button
                  @click="deleteArticle(article.ID)"
                  class="opacity-0 group-hover:opacity-100 hover:text-red-500 transition-opacity cursor-pointer text-xs"
                  title="Delete note"
                >
                  &times; Delete
                </button>
                <router-link
                  :to="`/articles/${article.ID}`"
                  class="hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors font-medium"
                >
                  Read &rarr;
                </router-link>
              </div>
            </div>
          </div>
        </article>
      </div>

    </div>

    <!-- VIEW MODE 2: LIST VIEW -->
    <div v-else-if="viewMode === 'list' || viewMode === 'ledger'" class="reveal-item">
      <div class="bg-white dark:bg-[#12151C] rounded-xl border border-gray-200/80 dark:border-white/[0.08] overflow-hidden shadow-2xs">
        <table class="w-full text-left border-collapse text-xs">
          <thead>
            <tr class="border-b border-gray-100 dark:border-white/[0.06] bg-gray-50/50 dark:bg-white/[0.02] font-mono text-[11px] text-gray-400 uppercase">
              <th class="py-2.5 px-4 font-normal w-12">#</th>
              <th class="py-2.5 px-4 font-normal">Title</th>
              <th class="py-2.5 px-4 font-normal hidden sm:table-cell">Tags</th>
              <th class="py-2.5 px-4 font-normal hidden md:table-cell">Read Time</th>
              <th class="py-2.5 px-4 font-normal text-right">Date</th>
              <th class="py-2.5 px-4 font-normal text-right w-16">Action</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="(article, idx) in filteredArticles"
              :key="article.ID"
              class="group border-b last:border-0 border-gray-100 dark:border-white/[0.04] hover:bg-gray-50/70 dark:hover:bg-white/[0.03] transition-colors"
            >
              <td class="py-3 px-4 font-mono text-gray-400">
                {{ String(idx + 1).padStart(2, '0') }}
              </td>
              <td class="py-3 px-4 font-medium text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors">
                <router-link :to="`/articles/${article.ID}`" class="flex items-center gap-2">
                  <span v-if="isMocArticle(article)" class="text-amber-500 font-mono text-[10px]">★</span>
                  <span class="truncate max-w-md">{{ article.title }}</span>
                </router-link>
              </td>
              <td class="py-3 px-4 hidden sm:table-cell">
                <div class="flex items-center gap-1.5">
                  <span
                    v-for="tag in article.parsedTags.slice(0, 2)"
                    :key="tag"
                    class="font-mono text-[10px] px-1.5 py-0.5 rounded bg-gray-100 dark:bg-white/[0.05] text-gray-500 dark:text-gray-400"
                  >
                    #{{ tag }}
                  </span>
                </div>
              </td>
              <td class="py-3 px-4 hidden md:table-cell font-mono text-gray-400 text-[11px]">
                {{ getReadingTime(article.article) }}
              </td>
              <td class="py-3 px-4 text-right font-mono text-gray-400 text-[11px]">
                {{ formatShortDate(article.ID) }}
              </td>
              <td class="py-3 px-4 text-right">
                <button
                  @click="deleteArticle(article.ID)"
                  class="text-gray-400 hover:text-red-500 p-1 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer font-mono"
                  title="Delete"
                >
                  &times;
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- VIEW MODE 3: TIMELINE STREAM -->
    <div v-else class="max-w-3xl mx-auto space-y-4">
      <article
        v-for="article in filteredArticles"
        :key="article.ID"
        class="reveal-item group relative bg-white dark:bg-[#12151C] rounded-xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 transition-all p-5 flex flex-col sm:flex-row items-start gap-4 shadow-2xs"
      >
        <div class="flex-grow min-w-0 pr-6">
          <div class="flex items-center gap-2 mb-1.5 font-mono text-[11px] text-gray-400">
            <span>{{ formatDate(article.ID) }}</span>
            <span>•</span>
            <span>{{ getReadingTime(article.article) }}</span>
            <span v-if="isMocArticle(article)" class="text-amber-500 font-semibold">★ MOC</span>
          </div>

          <h3 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors mb-1.5 truncate">
            <router-link :to="`/articles/${article.ID}`">
              {{ article.title }}
            </router-link>
          </h3>

          <p class="text-xs text-gray-500 dark:text-gray-400 line-clamp-2 leading-relaxed">
            {{ article.article }}
          </p>
        </div>

        <button
          @click="deleteArticle(article.ID)"
          class="opacity-0 group-hover:opacity-100 text-gray-400 hover:text-red-500 p-1 transition-opacity cursor-pointer"
          title="Delete article"
        >
          &times;
        </button>
      </article>
    </div>

  </div>
</template>

<style scoped>
/* ─── Timeline spine ─────────────────────────────────── */
.timeline-root {
  position: relative;
  padding-left: 3rem;
}

.timeline-spine {
  position: absolute;
  left: 0.75rem;
  top: 0.5rem;
  bottom: 0.5rem;
  width: 1.5px;
  background: linear-gradient(
    to bottom,
    transparent,
    rgb(209 213 219 / 0.8) 4%,
    rgb(209 213 219 / 0.8) 96%,
    transparent
  );
}

:global(.dark) .timeline-spine {
  background: linear-gradient(
    to bottom,
    transparent,
    rgb(55 65 81 / 0.7) 4%,
    rgb(55 65 81 / 0.7) 96%,
    transparent
  );
}

/* ─── Timeline entry ─────────────────────────────────── */
.timeline-entry {
  position: relative;
  margin-bottom: 2.5rem;
}

/* ─── Dot ────────────────────────────────────────────── */
.timeline-marker {
  position: absolute;
  left: -2.25rem;
  top: 1.6rem;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1rem;
  height: 1rem;
  z-index: 2;
}

.timeline-dot {
  display: block;
  width: 0.625rem;
  height: 0.625rem;
  border-radius: 9999px;
  background: #d1d5db;
  border: 2px solid white;
  box-shadow: 0 0 0 3px rgb(209 213 219 / 0.35);
  transition: background 0.3s ease, box-shadow 0.3s ease;
}

:global(.dark) .timeline-dot {
  background: #374151;
  border-color: #0a0a0a;
  box-shadow: 0 0 0 3px rgb(55 65 81 / 0.35);
}

.timeline-entry:hover .timeline-dot {
  background: #10b981;
  box-shadow: 0 0 0 4px rgb(16 185 129 / 0.2);
}

/* ─── Date label ─────────────────────────────────────── */
.timeline-date {
  position: absolute;
  left: -2.75rem;
  top: -1.25rem;
  font-size: 0.65rem;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: #9ca3af;
  white-space: nowrap;
  transform: rotate(-90deg);
  transform-origin: right center;
  right: auto;
  display: none; /* show on wider screens only */
}

@media (min-width: 768px) {
  .timeline-root {
    padding-left: 5rem;
  }

  .timeline-spine {
    left: 1.5rem;
  }

  .timeline-marker {
    left: -3rem;
  }

  .timeline-date {
    display: block;
    position: absolute;
    left: auto;
    right: calc(100% + 4.25rem);
    top: 50%;
    transform: translateY(-50%);
    text-align: right;
    font-size: 0.7rem;
    white-space: nowrap;
    writing-mode: horizontal-tb;
    color: #9ca3af;
  }

  :global(.dark) .timeline-date {
    color: #6b7280;
  }
}

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
</style>
