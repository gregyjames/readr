<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import axios from 'axios'
import emitter from '../event-bus.ts'
import BookmarkIcon from '../assets/book.svg'
import { settings, setViewMode as saveGlobalViewMode } from '../store/settings'
import ArticleProgressLabel from './ArticleProgressLabel.vue'
import { isMoc } from '../utils/moc'

interface Article {
  ID: number
  title: string
  article: string
  image: string
  tags: string
  parsedTags: string[]
  reading_status?: string
  reading_progress?: number
  reading_time?: string
  word_count?: number
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

function formatTimelineDate(unixTs: number): { month: string; day: string; year: string } {
  if (unixTs < 1_000_000_000) return { month: 'NOTE', day: '•', year: '' }
  const ms = unixTs < 100_000_000_000 ? unixTs * 1000 : unixTs
  const d = new Date(ms)
  const month = d.toLocaleDateString('en-US', { month: 'short' }).toUpperCase()
  const day = d.toLocaleDateString('en-US', { day: '2-digit' })
  const year = d.toLocaleDateString('en-US', { year: 'numeric' })
  return { month, day, year }
}

function getReadingTime(articleOrText?: Article | string | null): string {
  if (!articleOrText) return '1 min read'
  if (typeof articleOrText === 'object') {
    return articleOrText.reading_time || '1 min read'
  }
  const words = articleOrText.trim().split(/\s+/).filter(Boolean).length
  const minutes = Math.max(1, Math.ceil(words / 200))
  return `${minutes} min read`
}

function isMocArticle(article: Article): boolean {
  if (!article) return false
  return isMoc(article.title, article.parsedTags)
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


interface ToastNotification {
  visible: boolean
  message: string
  articleId: number | null
  article: Article | null
  timer: any
}

const toast = ref<ToastNotification>({
  visible: false,
  message: '',
  articleId: null,
  article: null,
  timer: null
})

const showToast = (message: string, articleId: number, article: Article | null) => {
  if (toast.value.timer) {
    clearTimeout(toast.value.timer)
  }
  toast.value = {
    visible: true,
    message,
    articleId,
    article,
    timer: setTimeout(() => {
      toast.value.visible = false
      toast.value.articleId = null
      toast.value.article = null
    }, 6000)
  }
}

const dismissToast = () => {
  if (toast.value.timer) {
    clearTimeout(toast.value.timer)
  }
  toast.value.visible = false
  toast.value.articleId = null
  toast.value.article = null
}

const archivingId = ref<number | null>(null)

function spawnArchiveParticles(id: number) {
  const el = document.querySelector(`[data-article-id="${id}"]`) as HTMLElement
  if (!el) return

  const rect = el.getBoundingClientRect()
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2

  // ── Scan line sweep ────────────────────────────────────
  const scanLine = document.createElement('div')
  scanLine.style.cssText = `
    position:fixed;left:${rect.left}px;top:${rect.top}px;
    width:${rect.width}px;height:3px;pointer-events:none;z-index:9998;
    background:linear-gradient(90deg,transparent 0%,rgba(245,158,11,0.0) 5%,rgba(245,158,11,0.95) 30%,rgba(255,255,255,0.95) 50%,rgba(245,158,11,0.95) 70%,rgba(245,158,11,0.0) 95%,transparent 100%);
    box-shadow:0 0 14px rgba(245,158,11,0.9),0 0 28px rgba(245,158,11,0.5),0 0 56px rgba(245,158,11,0.25);
    border-radius:2px;
  `
  document.body.appendChild(scanLine)
  scanLine.animate(
    [
      { top: `${rect.top - 4}px`, opacity: 0 },
      { top: `${rect.top}px`, opacity: 1, offset: 0.04 },
      { top: `${rect.bottom - 3}px`, opacity: 1, offset: 0.88 },
      { top: `${rect.bottom}px`, opacity: 0 },
    ],
    { duration: 180, easing: 'ease-in-out', fill: 'forwards' }
  ).onfinish = () => scanLine.remove()

  // ── Expanding burst ring ───────────────────────────────
  const ring = document.createElement('div')
  const maxDim = Math.max(rect.width, rect.height)
  ring.style.cssText = `
    position:fixed;left:${cx}px;top:${cy}px;
    width:8px;height:8px;margin-left:-4px;margin-top:-4px;
    border-radius:50%;border:2px solid rgba(245,158,11,0.95);
    pointer-events:none;z-index:9999;
    box-shadow:0 0 12px rgba(245,158,11,0.7),inset 0 0 6px rgba(245,158,11,0.4);
  `
  document.body.appendChild(ring)
  setTimeout(() => {
    ring.animate(
      [
        { transform: 'scale(1)', opacity: 0.95, borderWidth: '2px' },
        { transform: `scale(${maxDim / 6})`, opacity: 0, borderWidth: '1px' },
      ],
      { duration: 240, easing: 'ease-out', fill: 'forwards' }
    ).onfinish = () => ring.remove()
  }, 40)

  // ── Particle shower ────────────────────────────────────
  const palette = [
    '#f59e0b', '#fbbf24', '#fcd34d', '#fffbeb',
    '#10b981', '#34d399', '#6ee7b7',
    '#ffffff', '#e5e7eb',
  ]
  const shapes = ['50%', '50%', '50%', '2px', '2px', '4px']

  for (let i = 0; i < 22; i++) {
    const p = document.createElement('div')
    const size = Math.random() * 7 + 2
    const px = rect.left + Math.random() * rect.width
    const py = rect.top + Math.random() * rect.height
    const color = palette[Math.floor(Math.random() * palette.length)]
    const shape = shapes[Math.floor(Math.random() * shapes.length)]
    const dur = 240 + Math.random() * 200
    const dx = (Math.random() - 0.5) * 160
    const dy = -(Math.random() * 140 + 30)
    const rot = (Math.random() - 0.5) * 540
    const delay = Math.random() * 50

    p.style.cssText = `
      position:fixed;left:${px}px;top:${py}px;
      width:${size}px;height:${size}px;
      background:${color};border-radius:${shape};
      pointer-events:none;z-index:9999;
      box-shadow:0 0 ${size * 2}px ${color};
    `
    document.body.appendChild(p)
    setTimeout(() => {
      p.animate(
        [
          { opacity: 1, transform: 'scale(1) translate(0,0) rotate(0deg)' },
          { opacity: 0, transform: `scale(0) translate(${dx}px,${dy}px) rotate(${rot}deg)` },
        ],
        { duration: dur, easing: 'cubic-bezier(0.22,1,0.36,1)', fill: 'forwards' }
      ).onfinish = () => p.remove()
    }, delay)
  }
}

const archiveArticle = async (id: number) => {
  const targetArticle = articles.value.find(a => a.ID === id) || null
  archivingId.value = id
  spawnArchiveParticles(id)
  await new Promise(resolve => setTimeout(resolve, 320))
  try {
    await axios.post(`/api/articles/${id}/archive`)
    articles.value = articles.value.filter(article => article.ID !== id)
    showToast('Article moved to archive', id, targetArticle)
  } catch (err) {
    console.error('Failed to archive article', err)
  } finally {
    archivingId.value = null
  }
}

const undoArchive = async () => {
  const id = toast.value.articleId
  const cachedArticle = toast.value.article
  dismissToast()
  if (!id) return

  try {
    await axios.post(`/api/articles/${id}/unarchive`)
    if (cachedArticle && !articles.value.some(a => a.ID === id)) {
      articles.value = [cachedArticle, ...articles.value]
      nextTick(initReveal)
    } else {
      await fetchArticles()
    }
  } catch (err) {
    console.error('Failed to unarchive article', err)
  }
}



const deletingId = ref<number | null>(null)

function spawnDeleteParticles(id: number) {
  const el = document.querySelector(`[data-article-id="${id}"]`) as HTMLElement
  if (!el) return

  const rect = el.getBoundingClientRect()
  const cx = rect.left + rect.width / 2
  const cy = rect.top + rect.height / 2

  // ── Radial crack flash ─────────────────────────────────
  // Two expanding rings — inner tight (red), outer wide (crimson fade)
  ;[
    { delay: 0, scale: rect.width / 5, color: 'rgba(239,68,68,0.95)', dur: 220 },
    { delay: 20, scale: rect.width / 2.5, color: 'rgba(185,28,28,0.6)', dur: 280 },
  ].forEach(({ delay, scale, color, dur }) => {
    const ring = document.createElement('div')
    ring.style.cssText = `
      position:fixed;left:${cx}px;top:${cy}px;
      width:10px;height:10px;margin-left:-5px;margin-top:-5px;
      border-radius:50%;border:2px solid ${color};
      pointer-events:none;z-index:9999;
      box-shadow:0 0 16px ${color};
    `
    document.body.appendChild(ring)
    setTimeout(() => {
      ring.animate(
        [
          { transform: 'scale(1)', opacity: 1, borderWidth: '3px' },
          { transform: `scale(${scale})`, opacity: 0, borderWidth: '1px' },
        ],
        { duration: dur, easing: 'ease-out', fill: 'forwards' }
      ).onfinish = () => ring.remove()
    }, delay)
  })

  // ── Flash overlay ─────────────────────────────────────
  const flash = document.createElement('div')
  flash.style.cssText = `
    position:fixed;
    left:${rect.left}px;top:${rect.top}px;
    width:${rect.width}px;height:${rect.height}px;
    background:radial-gradient(circle at center, rgba(239,68,68,0.35) 0%, transparent 70%);
    border-radius:inherit;pointer-events:none;z-index:9997;
    border:1.5px solid rgba(239,68,68,0.7);
    box-shadow:0 0 24px rgba(239,68,68,0.5),inset 0 0 40px rgba(239,68,68,0.1);
  `
  document.body.appendChild(flash)
  flash.animate(
    [{ opacity: 1 }, { opacity: 0 }],
    { duration: 200, easing: 'ease-out', fill: 'forwards' }
  ).onfinish = () => flash.remove()

  // ── Particle shatter ──────────────────────────────────
  const palette = [
    '#ef4444', '#f87171', '#fca5a5', '#fee2e2',
    '#dc2626', '#b91c1c', '#7f1d1d',
    '#ffffff', '#fde8e8',
  ]
  const shapes = ['50%', '50%', '2px', '3px', '1px 4px']

  for (let i = 0; i < 26; i++) {
    const p = document.createElement('div')
    const size = Math.random() * 8 + 2
    const angle = (i / 26) * Math.PI * 2 + (Math.random() - 0.5) * 0.6
    // Start near center, scatter outward radially
    const startR = Math.random() * (Math.min(rect.width, rect.height) * 0.4)
    const px = cx + Math.cos(angle) * startR
    const py = cy + Math.sin(angle) * startR
    const color = palette[Math.floor(Math.random() * palette.length)]
    const shape = shapes[Math.floor(Math.random() * shapes.length)]
    const dur = 220 + Math.random() * 200
    // Scatter radially outward — much wider than archive
    const dist = 120 + Math.random() * 180
    const dx = Math.cos(angle) * dist + (Math.random() - 0.5) * 60
    const dy = Math.sin(angle) * dist - Math.random() * 80 // bias upward
    const rot = (Math.random() - 0.5) * 720
    const delay = Math.random() * 40

    p.style.cssText = `
      position:fixed;left:${px}px;top:${py}px;
      width:${size}px;height:${size}px;
      background:${color};border-radius:${shape};
      pointer-events:none;z-index:9999;
      box-shadow:0 0 ${size * 2}px ${color};
    `
    document.body.appendChild(p)
    setTimeout(() => {
      p.animate(
        [
          { opacity: 1, transform: 'scale(1) translate(0,0) rotate(0deg)' },
          { opacity: 0.8, transform: `scale(1.4) translate(${dx * 0.4}px,${dy * 0.4}px) rotate(${rot * 0.4}deg)`, offset: 0.25 },
          { opacity: 0, transform: `scale(0) translate(${dx}px,${dy}px) rotate(${rot}deg)` },
        ],
        { duration: dur, easing: 'cubic-bezier(0.22,1,0.36,1)', fill: 'forwards' }
      ).onfinish = () => p.remove()
    }, delay)
  }
}

const deleteArticle = async (id: number) => {
  if (!confirm('Are you sure you want to permanently delete this note from the vault?')) return
  deletingId.value = id
  spawnDeleteParticles(id)
  await new Promise(resolve => setTimeout(resolve, 300))
  try {
    await axios.delete(`/api/delete/${id}`)
    articles.value = articles.value.filter(article => article.ID !== id)
  } catch (err) {
    console.error('Failed to delete article', err)
  } finally {
    deletingId.value = null
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
  if (toast.value.timer) {
    clearTimeout(toast.value.timer)
  }
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
      <div v-if="leadArticle" class="reveal-item" :class="{ 'archiving': archivingId === leadArticle.ID, 'deleting': deletingId === leadArticle.ID }" :data-article-id="leadArticle.ID">
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
                  <span>{{ getReadingTime(leadArticle) }}</span>
                  <span>&bull;</span>
                  <ArticleProgressLabel
                    variant="meta"
                    :status="isMocArticle(leadArticle) ? 'not_started' : leadArticle.reading_status"
                    :progress="leadArticle.reading_progress"
                  />
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

                <div class="flex items-center gap-1">
                  <button
                    @click="archiveArticle(leadArticle.ID)"
                    class="p-1.5 rounded-lg text-gray-400 hover:text-amber-600 dark:hover:text-amber-400 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-colors cursor-pointer"
                    title="Archive note"
                    aria-label="Archive note"
                  >
                    <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="21 8 21 21 3 21 3 8"></polyline>
                      <rect x="1" y="3" width="22" height="5"></rect>
                      <line x1="10" y1="12" x2="14" y2="12"></line>
                    </svg>
                  </button>

                  <button
                    @click="deleteArticle(leadArticle.ID)"
                    class="p-1.5 rounded-lg text-gray-400 hover:text-red-500 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-colors cursor-pointer"
                    title="Permanently delete note"
                    aria-label="Permanently delete note"
                  >
                    <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="3 6 5 6 21 6"></polyline>
                      <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                    </svg>
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
          :class="{ 'archiving': archivingId === article.ID, 'deleting': deletingId === article.ID }"
          :data-article-id="article.ID"
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
                <ArticleProgressLabel
                  variant="overlay"
                  :status="isMocArticle(article) ? 'not_started' : article.reading_status"
                  :progress="article.reading_progress"
                />
              </div>
              <div class="relative z-10 flex items-baseline justify-between select-none">
                <span class="text-3xl font-bold tracking-tighter text-white/10 dark:text-white/[0.08] font-mono">
                  #{{ String(idx + 2).padStart(2, '0') }}
                </span>
                <span class="text-[10px] font-mono text-white/70">
                  {{ getReadingTime(article) }}
                </span>
              </div>
            </div>

            <!-- Floating Overlay Badges (When Image Is Present) -->
            <div v-if="hasValidImage(article)" class="absolute inset-x-3 top-3 flex items-center justify-between pointer-events-none z-10">
              <span class="px-2.5 py-0.5 rounded-full text-[10px] font-mono font-medium bg-black/60 backdrop-blur-md text-white/95 border border-white/15 shadow-xs flex items-center gap-1.5">
                <span class="w-1 h-1 rounded-full bg-emerald-400"></span>
                {{ getDomain(article) }}
              </span>
              <span class="px-2.5 py-0.5 rounded-full bg-black/60 backdrop-blur-md border border-white/15 shadow-xs inline-flex items-center">
                <ArticleProgressLabel
                  variant="overlay"
                  :status="isMocArticle(article) ? 'not_started' : article.reading_status"
                  :progress="article.reading_progress"
                />
              </span>
            </div>

            <!-- Inset Bottom Bar over Image -->
            <div v-if="hasValidImage(article)" class="absolute inset-x-3 bottom-2.5 flex items-center justify-between pointer-events-none z-10">
              <span class="text-[10px] font-mono text-white/90 font-medium drop-shadow-sm">
                {{ formatShortDate(article.ID) || `ID #${article.ID}` }}
              </span>
              <span class="text-[10px] font-mono text-white/85 drop-shadow-sm">
                {{ getReadingTime(article) }}
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
              <div class="flex items-center gap-2">
                <button
                  @click="archiveArticle(article.ID)"
                  class="opacity-0 group-hover:opacity-100 p-1 rounded-md text-gray-400 hover:text-amber-600 dark:hover:text-amber-400 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-all cursor-pointer"
                  title="Archive note"
                  aria-label="Archive note"
                >
                  <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <polyline points="21 8 21 21 3 21 3 8"></polyline>
                    <rect x="1" y="3" width="22" height="5"></rect>
                    <line x1="10" y1="12" x2="14" y2="12"></line>
                  </svg>
                </button>

                <button
                  @click="deleteArticle(article.ID)"
                  class="opacity-0 group-hover:opacity-100 p-1 rounded-md text-gray-400 hover:text-red-500 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-all cursor-pointer"
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

    <!-- VIEW MODE 2: BREATHTAKING VERTICAL TIMELINE STREAM -->
    <div v-else class="max-w-4xl mx-auto relative pl-6 sm:pl-32 py-4">

      <!-- Continuous Luminescent Spine -->
      <div class="absolute left-6 sm:left-[5.5rem] top-3 bottom-6 w-[2px] bg-gradient-to-b from-emerald-500/50 via-gray-200/80 dark:via-white/[0.08] to-transparent pointer-events-none"></div>

      <div class="space-y-6 sm:space-y-8">
        <div
          v-for="(article, idx) in filteredArticles"
          :key="article.ID"
          class="reveal-item relative group"
          :class="{ 'archiving': archivingId === article.ID, 'deleting': deletingId === article.ID }"
          :data-article-id="article.ID"
        >
          <!-- Desktop Timestamp on Left of Spine -->
          <div class="hidden sm:flex absolute -left-[5.5rem] top-5 w-20 flex-col items-end pr-4 text-right select-none">
            <span class="text-[10px] font-mono font-bold tracking-widest text-emerald-600 dark:text-emerald-400 uppercase">
              {{ formatTimelineDate(article.ID).month }}
            </span>
            <span class="text-xl font-bold tracking-tight text-gray-900 dark:text-gray-100 font-['Outfit'] -my-0.5">
              {{ formatTimelineDate(article.ID).day }}
            </span>
            <span class="text-[10px] font-mono text-gray-400 dark:text-gray-500">
              {{ formatTimelineDate(article.ID).year }}
            </span>
          </div>

          <!-- Interactive Node Pin on Spine -->
          <div class="absolute -left-[1.375rem] top-7 -translate-x-1/2 w-4 h-4 rounded-full bg-[#FAFAFA] dark:bg-[#0C0E12] border-2 border-gray-300 dark:border-white/20 group-hover:border-emerald-500 group-hover:scale-125 transition-all duration-300 flex items-center justify-center z-10 shadow-xs">
            <div class="w-1.5 h-1.5 rounded-full bg-gray-400 dark:bg-white/40 group-hover:bg-emerald-500 group-hover:shadow-[0_0_10px_rgba(16,185,129,0.9)] transition-all duration-300"></div>
          </div>

          <!-- Timeline Content Capsule (Double-Bezel Hardware Aesthetic) -->
          <div class="ml-2 sm:ml-4 relative bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 hover:shadow-xl hover:shadow-emerald-500/[0.03] transition-all duration-300 p-4 sm:p-5 flex flex-col md:flex-row items-start md:items-center gap-4 sm:gap-5 group-hover:-translate-y-0.5">
            
            <!-- Thumbnail Visual Preview -->
            <div class="w-full md:w-36 h-36 md:h-24 rounded-xl overflow-hidden bg-gray-100 dark:bg-[#0A0C10] border border-gray-100 dark:border-white/[0.06] flex-shrink-0 relative group/thumb">
              <template v-if="hasValidImage(article)">
                <img
                  :src="article.image"
                  :alt="article.title"
                  @error="onImageError(article.ID)"
                  class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-500 ease-out"
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
                <span class="relative z-10 text-[9px] font-mono text-emerald-400/70 font-semibold tracking-wider uppercase truncate">
                  {{ getDomain(article).split('.')[0] }}
                </span>
                <span class="relative z-10 font-mono text-2xl font-bold text-white/10 select-none">
                  #{{ String(idx + 1).padStart(2, '0') }}
                </span>
              </div>
            </div>

            <!-- Content Details -->
            <div class="flex-grow min-w-0 pr-2">
              <!-- Eyebrow Meta -->
              <div class="flex flex-wrap items-center gap-2 mb-1.5 text-xs font-mono">
                <!-- Mobile Date Pill (hidden on desktop) -->
                <span class="sm:hidden text-emerald-600 dark:text-emerald-400 font-semibold">
                  {{ formatShortDate(article.ID) }}
                </span>
                <span class="sm:hidden text-gray-300 dark:text-gray-600">•</span>

                <!-- Domain Pill -->
                <span class="px-2 py-0.5 rounded-full text-[10px] bg-gray-100 dark:bg-white/[0.05] text-gray-600 dark:text-gray-300 border border-gray-200/60 dark:border-white/[0.06] flex items-center gap-1.5 font-medium">
                  <span class="w-1 h-1 rounded-full bg-emerald-500"></span>
                  {{ getDomain(article) }}
                </span>

                <!-- Reading Time -->
                <span class="text-[11px] text-gray-400 dark:text-gray-500 font-mono">
                  {{ getReadingTime(article) }}
                </span>

                <!-- Reading Status -->
                <ArticleProgressLabel
                  variant="meta"
                  :status="isMocArticle(article) ? 'not_started' : article.reading_status"
                  :progress="article.reading_progress"
                />

                <!-- MOC Badge -->
                <span
                  v-if="isMocArticle(article)"
                  class="px-1.5 py-0.5 rounded text-[10px] font-mono bg-amber-500/10 text-amber-700 dark:text-amber-300 font-medium border border-amber-500/20"
                >
                  ★ MOC
                </span>

                <!-- Tags -->
                <button
                  v-for="tag in article.parsedTags.slice(0, 2)"
                  :key="tag"
                  @click="selectedTag = tag"
                  class="text-[10px] font-mono px-1.5 py-0.5 rounded-md bg-gray-100/70 dark:bg-white/[0.03] text-gray-500 dark:text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 hover:bg-emerald-500/10 transition-colors cursor-pointer"
                >
                  #{{ tag }}
                </button>
              </div>

              <!-- Title -->
              <h3 class="text-base sm:text-lg font-semibold tracking-tight text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors mb-1.5 leading-snug font-['Outfit']">
                <router-link :to="`/articles/${article.ID}`">
                  {{ article.title }}
                </router-link>
              </h3>

              <!-- Excerpt -->
              <p class="text-xs text-gray-500 dark:text-gray-400 line-clamp-2 leading-relaxed font-normal">
                {{ article.article }}
              </p>
            </div>

            <!-- Trailing Interactive Action & Delete -->
            <div class="flex items-center gap-2 self-end md:self-center flex-shrink-0 pt-2 md:pt-0">
              <button
                @click="archiveArticle(article.ID)"
                class="opacity-0 group-hover:opacity-100 hover:text-amber-600 dark:hover:text-amber-400 text-gray-400 p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-all cursor-pointer"
                title="Archive note"
                aria-label="Archive note"
              >
                <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="21 8 21 21 3 21 3 8"></polyline>
                  <rect x="1" y="3" width="22" height="5"></rect>
                  <line x1="10" y1="12" x2="14" y2="12"></line>
                </svg>
              </button>

              <button
                @click="deleteArticle(article.ID)"
                class="opacity-0 group-hover:opacity-100 hover:text-red-500 text-gray-400 p-2 rounded-lg hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-all cursor-pointer"
                title="Permanently delete note"
                aria-label="Permanently delete note"
              >
                <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="3 6 5 6 21 6"></polyline>
                  <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
                </svg>
              </button>

              <router-link
                :to="`/articles/${article.ID}`"
                class="w-10 h-10 rounded-xl bg-gray-100 dark:bg-white/[0.05] group-hover:bg-emerald-500 text-gray-400 group-hover:text-white transition-all duration-300 flex items-center justify-center shadow-2xs group-hover:shadow-emerald-500/25 group-hover:scale-105 active:scale-95 ml-1"
                title="Read article"
                aria-label="Read article"
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

    <!-- Toast Notification (Archive Feedback with Undo) -->
    <Teleport to="body">
      <Transition
        enter-active-class="transition duration-300 ease-out"
        enter-from-class="transform translate-y-4 opacity-0 scale-95"
        enter-to-class="transform translate-y-0 opacity-100 scale-100"
        leave-active-class="transition duration-200 ease-in"
        leave-from-class="transform translate-y-0 opacity-100 scale-100"
        leave-to-class="transform translate-y-4 opacity-0 scale-95"
      >
        <div
          v-if="toast.visible"
          role="status"
          aria-live="polite"
          class="fixed bottom-6 right-6 z-50 flex items-center gap-3 px-4 py-3 rounded-xl bg-gray-900/95 dark:bg-[#121620]/95 text-white shadow-xl shadow-black/20 border border-gray-800 dark:border-white/10 backdrop-blur-md text-sm font-sans"
        >
          <div class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
            <span class="font-normal text-gray-200">{{ toast.message }}</span>
          </div>

          <div class="flex items-center gap-2 pl-2 border-l border-gray-700/80 dark:border-white/10">
            <button
              @click="undoArchive"
              class="px-2.5 py-1 text-xs font-semibold uppercase tracking-wider text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10 rounded-md transition-colors cursor-pointer"
            >
              Undo
            </button>
            <button
              @click="dismissToast"
              class="p-1 text-gray-400 hover:text-white rounded-md transition-colors cursor-pointer"
              title="Dismiss"
            >
              <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <line x1="18" y1="6" x2="6" y2="18"></line>
                <line x1="6" y1="6" x2="18" y2="18"></line>
              </svg>
            </button>
          </div>
        </div>
      </Transition>
    </Teleport>

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

/* ─── Archive fly-out animation ──────────────────────── */

/*
  Stage breakdown (320ms total):
  0–6%   : Snap attention — amber border corona ignites
  6–18%  : Chromatic glitch — brief hue-rotate flicker
  18–40% : Horizontal press — card narrows, perspective tips back
  40–62% : Vault collapse — scale + rise + depth rotation
  62–82% : Sepia burn — desaturates into amber-gold ghost
  82–100%: Final dissolve — shrinks to a point and vanishes
*/
@keyframes archive-out {
  0% {
    opacity: 1;
    transform: perspective(800px) scale(1) translateY(0) rotateX(0deg) rotateZ(0deg);
    filter: brightness(1) saturate(1) hue-rotate(0deg) blur(0px);
    box-shadow:
      0 0 0 0px rgba(245, 158, 11, 0),
      0 0 0px rgba(245, 158, 11, 0);
    outline: 2px solid transparent;
    outline-offset: 0px;
  }

  /* ── Snap: amber corona fires ── */
  6% {
    transform: perspective(800px) scale(1.012) translateY(-3px) rotateX(0deg) rotateZ(0deg);
    filter: brightness(1.18) saturate(1.4) hue-rotate(0deg) blur(0px);
    box-shadow:
      0 0 0 3px rgba(245, 158, 11, 0.95),
      0 0 24px rgba(245, 158, 11, 0.55),
      0 0 64px rgba(245, 158, 11, 0.25),
      inset 0 0 30px rgba(245, 158, 11, 0.08);
    outline: 2px solid rgba(245, 158, 11, 0.7);
    outline-offset: 3px;
    opacity: 1;
  }

  /* ── Glitch: chromatic aberration flicker ── */
  12% {
    filter: brightness(1.1) saturate(1.1) hue-rotate(30deg) blur(0px);
    transform: perspective(800px) scale(1.008) translateY(-2px) rotateX(0deg) rotateZ(0.4deg);
    box-shadow:
      0 0 0 2px rgba(245, 158, 11, 0.8),
      0 0 18px rgba(245, 158, 11, 0.4),
      4px 0 8px rgba(239, 68, 68, 0.3),
      -4px 0 8px rgba(16, 185, 129, 0.3);
  }
  15% {
    filter: brightness(1.05) saturate(1.0) hue-rotate(-10deg) blur(0px);
    transform: perspective(800px) scale(1.0) translateY(-1px) rotateX(0deg) rotateZ(-0.3deg);
  }

  /* ── Horizontal press: folds inward ── */
  28% {
    transform: perspective(800px) scaleX(0.93) scaleY(1.01) translateY(-6px) rotateX(4deg) rotateZ(0deg);
    filter: brightness(0.96) saturate(0.75) hue-rotate(0deg) blur(0px);
    box-shadow:
      0 0 0 1.5px rgba(245, 158, 11, 0.6),
      0 0 12px rgba(245, 158, 11, 0.3),
      inset 0 0 20px rgba(245, 158, 11, 0.06);
    opacity: 0.95;
  }

  /* ── Vault collapse: depth + scale ── */
  50% {
    transform: perspective(800px) scale(0.78) translateY(-28px) rotateX(16deg) rotateZ(0.8deg);
    filter: brightness(0.82) saturate(0.45) hue-rotate(8deg) blur(0.8px);
    box-shadow:
      0 0 0 1px rgba(245, 158, 11, 0.4),
      0 16px 40px rgba(0, 0, 0, 0.3);
    opacity: 0.65;
  }

  /* ── Sepia burn ── */
  72% {
    transform: perspective(800px) scale(0.5) translateY(-50px) rotateX(24deg) rotateZ(1.5deg);
    filter: brightness(0.6) saturate(0.1) sepia(0.9) blur(2px);
    box-shadow: none;
    opacity: 0.3;
  }

  /* ── Final dissolve to nothing ── */
  100% {
    transform: perspective(800px) scale(0.1) translateY(-70px) rotateX(32deg) rotateZ(2deg);
    filter: brightness(0.2) saturate(0) sepia(1) blur(6px);
    box-shadow: none;
    outline: 2px solid transparent;
    opacity: 0;
  }
}

.reveal-item.archiving {
  animation: archive-out 0.32s cubic-bezier(0.4, 0, 0.2, 1) forwards;
  pointer-events: none;
  transform-origin: center 60%;
  will-change: transform, opacity, filter;
}

/* ─── Delete shatter animation ───────────────────────── */

/*
  Stage breakdown (300ms total):
  0–5%   : Red corona flash — border ignites crimson
  5–18%  : Violent shake — rapid left-right jitter (can't escape)
  18–35% : Crack inward — scaleX crush, red tint floods in
  35–55% : Implosion — collapses toward center point
  55–80% : Burn — red-to-black filter drain
  80–100%: Annihilation — shrinks to nothing
*/
@keyframes delete-out {
  0% {
    opacity: 1;
    transform: perspective(800px) scale(1) translateX(0) rotateZ(0deg);
    filter: brightness(1) saturate(1) hue-rotate(0deg) blur(0px);
    box-shadow: 0 0 0 0px rgba(239, 68, 68, 0);
    outline: 2px solid transparent;
  }

  /* ── Red corona ignites ── */
  5% {
    transform: perspective(800px) scale(1.01) translateX(0) rotateZ(0deg);
    filter: brightness(1.2) saturate(1.6) hue-rotate(-15deg) blur(0px);
    box-shadow:
      0 0 0 3px rgba(239, 68, 68, 1),
      0 0 20px rgba(239, 68, 68, 0.7),
      0 0 60px rgba(239, 68, 68, 0.35),
      inset 0 0 30px rgba(239, 68, 68, 0.12);
    outline: 2px solid rgba(239, 68, 68, 0.8);
    outline-offset: 2px;
    opacity: 1;
  }

  /* ── Violent shake — can't escape ── */
  8%  { transform: perspective(800px) scale(1.01) translateX(-6px) rotateZ(-0.8deg); filter: brightness(1.15) saturate(1.5) hue-rotate(-10deg); }
  11% { transform: perspective(800px) scale(1.01) translateX(7px)  rotateZ(0.9deg);  filter: brightness(1.2) saturate(1.6) hue-rotate(-20deg); }
  14% { transform: perspective(800px) scale(1.01) translateX(-5px) rotateZ(-0.6deg); filter: brightness(1.1) saturate(1.4) hue-rotate(-5deg); }
  17% { transform: perspective(800px) scale(1.01) translateX(4px)  rotateZ(0.5deg);  filter: brightness(1.05) saturate(1.2) hue-rotate(0deg); }

  /* ── Crack inward — X-axis crush ── */
  30% {
    transform: perspective(800px) scaleX(0.88) scaleY(1.04) translateX(0) rotateZ(0deg);
    filter: brightness(0.9) saturate(0.8) hue-rotate(-5deg) blur(0.5px);
    box-shadow:
      0 0 0 2px rgba(239, 68, 68, 0.7),
      0 0 14px rgba(239, 68, 68, 0.4),
      inset 0 0 40px rgba(239, 68, 68, 0.15);
    opacity: 0.9;
  }

  /* ── Implosion toward center ── */
  50% {
    transform: perspective(800px) scale(0.72) translateY(10px) rotateZ(1deg);
    filter: brightness(0.7) saturate(0.5) hue-rotate(-20deg) blur(1px);
    box-shadow:
      0 0 0 1px rgba(239, 68, 68, 0.5),
      0 8px 32px rgba(0, 0, 0, 0.4);
    opacity: 0.6;
  }

  /* ── Crimson burn ── */
  72% {
    transform: perspective(800px) scale(0.45) translateY(16px) rotateZ(2deg);
    filter: brightness(0.4) saturate(0.2) hue-rotate(-30deg) blur(3px);
    box-shadow: none;
    opacity: 0.25;
  }

  /* ── Annihilation ── */
  100% {
    transform: perspective(800px) scale(0.05) translateY(20px) rotateZ(3deg);
    filter: brightness(0) saturate(0) hue-rotate(0deg) blur(8px);
    box-shadow: none;
    outline: 2px solid transparent;
    opacity: 0;
  }
}

.reveal-item.deleting {
  animation: delete-out 0.30s cubic-bezier(0.4, 0, 0.2, 1) forwards;
  pointer-events: none;
  transform-origin: center center;
  will-change: transform, opacity, filter;
}
</style>
