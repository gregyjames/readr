<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import axios from 'axios'
import emitter from '../event-bus.ts'
import BookmarkIcon from '../assets/book.svg'

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

import { useRoute, useRouter} from 'vue-router'
const route = useRoute()
const router = useRouter()

const viewMode = ref<'card' | 'list'>('card')
const selectedTag = ref<string | null>(null)

const fetchArticles = async () => {
  const res = await axios.get('/api/getarticles')
  articles.value = res.data.map((article: any) => ({
    ...article,
    parsedTags: article.tags ? article.tags.split(',').map((tag: string) => tag.trim()) : []
  }))
  await nextTick()
  initReveal()
}

// Scroll-reveal via IntersectionObserver — no deps needed
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
    { threshold: 0.12, rootMargin: '0px 0px -40px 0px' }
  )
  document.querySelectorAll('.reveal-item').forEach((el) => observer!.observe(el))
}

// Format unix timestamp (article ID) as a readable date
function formatDate(unixTs: number): string {
  // IDs under ~1e9 are likely DB sequential IDs, not timestamps — fall back gracefully
  if (unixTs < 1_000_000_000) return ''
  const d = new Date(unixTs * 1000)
  return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
}

onMounted(async () => {
  await fetchArticles()
  const queryView = route.query.view
  const storedView = localStorage.getItem('viewMode')

  if (queryView === 'card' || queryView === 'list') {
    viewMode.value = queryView
  } else if (storedView === 'card' || storedView === 'list') {
    viewMode.value = storedView
    router.replace({ query: { view: storedView } })
  } else {
    router.replace({ query: { view: 'card' } })
  }

  emitter.on('article-added', fetchArticles)
})

onBeforeUnmount(() => {
  emitter.off('article-added', fetchArticles)
  observer?.disconnect()
})

const deleteArticle = async (id: number) => {
  if (!confirm("Are you sure you want to delete this article?")) return
  try {
    await axios.delete(`/api/delete/${id}`)
    articles.value = articles.value.filter(article => article.ID !== id)
  } catch (err) {
    console.error('Failed to delete article', err)
  }
}

watch(() => route.query.view, (newView) => {
  if (newView === 'card' || newView === 'list') {
    viewMode.value = newView
    localStorage.setItem('viewMode', newView)
    nextTick(initReveal)
  }
})

watch(viewMode, () => nextTick(initReveal))

const filteredArticles = computed(() => {
  if (!selectedTag.value) return articles.value
  return articles.value.filter(article =>
    article.parsedTags.includes(selectedTag.value!)
  )
})
</script>

<template>
  <div>
    <div v-if="selectedTag" class="flex justify-between items-center mb-10 px-2">
      <div class="flex items-center gap-3">
        <span class="text-sm text-gray-500 dark:text-gray-400">Filtered by</span>
        <span class="bg-gray-900 dark:bg-gray-100 text-white dark:text-gray-900 text-sm font-medium px-4 py-1.5 rounded-full shadow-sm">{{ selectedTag }}</span>
      </div>
      <button @click="selectedTag = null" class="text-sm font-medium text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 transition-colors">
        Clear filter
      </button>
    </div>

    <!-- Empty State -->
    <div v-if="filteredArticles.length === 0" class="flex flex-col items-center justify-center py-40 px-4 text-center">
      <div class="w-20 h-20 bg-gray-100/50 dark:bg-gray-800/50 rounded-full flex items-center justify-center mb-8 backdrop-blur-sm border border-gray-200/50 dark:border-gray-700/50 shadow-inner">
        <BookmarkIcon class="w-10 h-10 text-gray-400 dark:text-gray-500" />
      </div>
      <h3 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 mb-3">No articles found</h3>
      <p class="text-lg text-gray-500 dark:text-gray-400 max-w-sm mx-auto leading-relaxed">You haven't added any articles yet, or none match your current filter.</p>
    </div>

    <!-- Card View (unchanged) -->
    <div v-else-if="viewMode === 'card'" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-10">
      <div v-for="article in filteredArticles" :key="article.ID" class="group relative bg-white dark:bg-[#121212] rounded-[2rem] shadow-[0_8px_30px_rgb(0,0,0,0.04)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.6)] border border-gray-100/80 dark:border-white/5 hover:shadow-[0_20px_60px_rgb(0,0,0,0.08)] dark:hover:shadow-[0_20px_60px_rgb(0,0,0,0.8)] hover:-translate-y-1.5 transition-all duration-500 ease-out overflow-hidden flex flex-col active:scale-[0.98]">
        <button
          @click.prevent="deleteArticle(article.ID)"
          class="absolute top-5 right-5 bg-white/80 dark:bg-black/60 backdrop-blur-md border border-gray-200/50 dark:border-white/10 text-gray-900 dark:text-gray-100 opacity-0 group-hover:opacity-100 hover:bg-red-500 dark:hover:bg-red-500 hover:text-white hover:border-transparent rounded-full w-9 h-9 flex items-center justify-center text-lg z-20 transition-all duration-300 shadow-sm active:scale-90"
          title="Delete"
        >
        &times;
        </button>
        <div class="w-full aspect-[4/3] overflow-hidden bg-gray-50 dark:bg-[#1a1a1a] relative group-hover:after:absolute group-hover:after:inset-0 group-hover:after:bg-black/10 dark:group-hover:after:bg-white/5 group-hover:after:transition-colors">
          <img v-if="article.image" :src="article.image" alt="Cover" class="w-full h-full object-cover transform group-hover:scale-105 transition-transform duration-700 ease-[cubic-bezier(0.2,0.8,0.2,1)]" />
        </div>
        <div class="p-8 flex flex-col flex-grow">
          <div class="flex flex-wrap gap-2 mb-5">
            <button v-for="tag in article.parsedTags.slice(0, 3)" :key="tag" @click.prevent="selectedTag = tag" class="bg-gray-100/80 dark:bg-white/5 hover:bg-emerald-100 dark:hover:bg-emerald-900/30 text-gray-700 dark:text-gray-300 hover:text-emerald-700 dark:hover:text-emerald-300 text-[10px] font-bold px-3 py-1.5 rounded-full uppercase tracking-widest transition-colors z-20 relative cursor-pointer active:scale-95">
              {{ tag }}
            </button>
          </div>
          <router-link
            :to="`${article.article}`"
            class="block text-2xl leading-tight font-bold text-gray-900 dark:text-white hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors mb-4 tracking-tight before:absolute before:inset-0"
          >
            {{ article.title }}
          </router-link>
          <p class="text-gray-500 dark:text-gray-400 text-sm leading-relaxed line-clamp-2 mt-auto font-medium">
            {{ article.article }}
          </p>
        </div>
      </div>
    </div>

    <!-- Timeline List View -->
    <div v-else class="timeline-root max-w-3xl mx-auto">
      <!-- spine line -->
      <div class="timeline-spine" aria-hidden="true"></div>

      <div
        v-for="(article, i) in filteredArticles"
        :key="article.ID"
        class="reveal-item timeline-entry group"
        :style="{ transitionDelay: `${Math.min(i, 4) * 60}ms` }"
      >
        <!-- dot + date -->
        <div class="timeline-marker" aria-hidden="true">
          <span class="timeline-dot"></span>
        </div>
        <div class="timeline-date" aria-label="saved on">
          {{ formatDate(article.ID) }}
        </div>

        <!-- card -->
        <div class="timeline-card relative bg-white dark:bg-[#121212] rounded-[1.75rem] shadow-[0_4px_20px_rgb(0,0,0,0.04)] dark:shadow-[0_4px_20px_rgb(0,0,0,0.5)] border border-gray-100/80 dark:border-white/5 hover:shadow-[0_12px_40px_rgb(0,0,0,0.08)] dark:hover:shadow-[0_12px_40px_rgb(0,0,0,0.7)] hover:-translate-y-1 transition-all duration-500 ease-out active:scale-[0.99] overflow-hidden flex flex-col sm:flex-row">
          <button
            @click.prevent="deleteArticle(article.ID)"
            class="absolute top-5 right-5 bg-white/80 dark:bg-black/60 backdrop-blur-md border border-gray-200/50 dark:border-white/10 text-gray-900 dark:text-gray-100 opacity-0 group-hover:opacity-100 hover:bg-red-500 dark:hover:bg-red-500 hover:text-white hover:border-transparent rounded-full w-8 h-8 flex items-center justify-center text-base z-30 transition-all duration-300 shadow-sm active:scale-90"
            title="Delete"
          >
          &times;
          </button>

          <!-- thumbnail -->
          <div class="w-full sm:w-44 aspect-[16/9] sm:aspect-auto flex-shrink-0 overflow-hidden bg-gray-50 dark:bg-[#1a1a1a] sm:rounded-l-[1.75rem]">
            <img v-if="article.image" :src="article.image" :alt="article.title" class="w-full h-full object-cover transform group-hover:scale-105 transition-transform duration-700 ease-[cubic-bezier(0.2,0.8,0.2,1)]" />
            <div v-else class="w-full h-full flex items-center justify-center opacity-30">
              <BookmarkIcon class="w-8 h-8 text-gray-400 dark:text-gray-500" />
            </div>
          </div>

          <!-- body -->
          <div class="flex flex-col justify-center px-7 py-6 sm:pr-12">
            <div class="flex flex-wrap gap-1.5 mb-3">
              <button
                v-for="tag in article.parsedTags.slice(0, 4)"
                :key="tag"
                @click.prevent="selectedTag = tag"
                class="bg-gray-100/80 dark:bg-white/5 hover:bg-emerald-100 dark:hover:bg-emerald-900/30 text-gray-700 dark:text-gray-300 hover:text-emerald-700 dark:hover:text-emerald-300 text-[9px] font-bold px-2.5 py-1 rounded-full uppercase tracking-widest transition-colors z-20 relative cursor-pointer active:scale-95"
              >
                {{ tag }}
              </button>
            </div>
            <router-link
              :to="`${article.article}`"
              class="text-xl font-bold text-gray-900 dark:text-white hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors tracking-tight mb-2 leading-snug before:absolute before:inset-0"
            >
              {{ article.title }}
            </router-link>
            <p class="text-gray-400 dark:text-gray-500 text-sm leading-relaxed line-clamp-2">
              {{ article.article }}
            </p>
          </div>
        </div>
      </div>
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
