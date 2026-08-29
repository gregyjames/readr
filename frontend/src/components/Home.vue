<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed } from 'vue'
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

defineProps<{ msg: string }>()

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
}

onMounted(async () => {
  await fetchArticles()
  const queryView = route.query.view
  const storedView = localStorage.getItem('viewMode')

  if (queryView === 'card' || queryView === 'list') {
    viewMode.value = queryView
  } else if (storedView === 'card' || storedView === 'list') {
    viewMode.value = storedView
    router.replace({ query: { view: storedView } }) // sync URL
  } else {
    router.replace({ query: { view: 'card' } }) // default
  }

  emitter.on('article-added', fetchArticles)
})

onBeforeUnmount(() => {
  emitter.off('article-added', fetchArticles)
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
  }
})

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
        <span class="text-sm text-gray-500">Filtered by</span>
        <span class="bg-gray-900 text-white text-sm font-medium px-4 py-1.5 rounded-full shadow-sm">{{ selectedTag }}</span>
      </div>
      <button @click="selectedTag = null" class="text-sm font-medium text-gray-500 hover:text-gray-900 transition-colors">
        Clear filter
      </button>
    </div>

    <!-- Empty State -->
    <div v-if="filteredArticles.length === 0" class="flex flex-col items-center justify-center py-32 px-4 text-center">
      <div class="w-16 h-16 bg-gray-100 rounded-full flex items-center justify-center mb-6">
        <BookmarkIcon class="w-8 h-8 text-gray-400" />
      </div>
      <h3 class="text-xl font-medium text-gray-900 mb-2">No articles found</h3>
      <p class="text-gray-500 max-w-sm mx-auto">You haven't added any articles yet, or none match your current filter.</p>
    </div>

    <!-- Card View -->
    <div v-else-if="viewMode === 'card'" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
      <div v-for="article in filteredArticles" :key="article.ID" class="group relative bg-white rounded-3xl shadow-[0_4px_24px_rgba(0,0,0,0.02)] border border-gray-100 hover:shadow-[0_12px_48px_rgba(0,0,0,0.06)] hover:-translate-y-1 transition-all duration-400 ease-out overflow-hidden flex flex-col">
        <button
          @click.prevent="deleteArticle(article.ID)"
          class="absolute top-4 right-4 bg-white/90 backdrop-blur text-gray-900 opacity-0 group-hover:opacity-100 hover:bg-red-500 hover:text-white rounded-full w-8 h-8 flex items-center justify-center text-lg z-10 transition-all shadow-sm"
          title="Delete"
        >
        &times;
        </button>
        <div class="w-full aspect-[4/3] overflow-hidden bg-gray-50 relative">
          <img v-if="article.image" :src="article.image" alt="Cover" class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-700 ease-out" />
        </div>
        <div class="p-6 flex flex-col flex-grow">
          <div class="flex flex-wrap gap-1.5 mb-4">
            <button v-for="tag in article.parsedTags.slice(0, 3)" :key="tag" @click.prevent="selectedTag = tag" class="bg-gray-100/80 hover:bg-gray-200 text-gray-700 text-[11px] font-medium px-2.5 py-1 rounded-full uppercase tracking-wider transition-colors z-20 relative cursor-pointer">
              {{ tag }}
            </button>
          </div>
          <router-link
            :to="`${article.article}`"
            class="block text-xl leading-tight font-semibold text-gray-900 hover:text-emerald-600 transition-colors mb-3 tracking-tight before:absolute before:inset-0"
          >
            {{ article.title }}
          </router-link>
          <p class="text-gray-500 text-sm line-clamp-2 mt-auto">
            {{ article.article }}
          </p>
        </div>
      </div>
    </div>

    <!-- List View -->
    <div v-else class="space-y-6 max-w-4xl mx-auto">
      <div v-for="article in filteredArticles" :key="article.ID" class="group relative flex flex-col sm:flex-row bg-white rounded-3xl p-4 shadow-[0_4px_24px_rgba(0,0,0,0.02)] border border-gray-100 hover:shadow-[0_12px_48px_rgba(0,0,0,0.06)] hover:-translate-y-1 transition-all duration-400 ease-out">
        <button
          @click.prevent="deleteArticle(article.ID)"
          class="absolute top-4 right-4 bg-white/90 backdrop-blur text-gray-900 opacity-0 group-hover:opacity-100 hover:bg-red-500 hover:text-white rounded-full w-8 h-8 flex items-center justify-center text-lg z-20 transition-all shadow-sm"
          title="Delete"
        >
        &times;
        </button>
        <div class="w-full sm:w-48 aspect-[16/9] sm:aspect-square flex-shrink-0 overflow-hidden rounded-2xl bg-gray-50 mb-4 sm:mb-0 relative z-10">
          <img v-if="article.image" :src="article.image" alt="Cover" class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-700 ease-out" />
        </div>
        <div class="flex flex-col justify-center sm:pl-8 sm:pr-12">
          <div class="flex flex-wrap gap-1.5 mb-3">
            <button v-for="tag in article.parsedTags.slice(0, 4)" :key="tag" @click.prevent="selectedTag = tag" class="bg-gray-100/80 hover:bg-gray-200 text-gray-700 text-[11px] font-medium px-2.5 py-1 rounded-full uppercase tracking-wider transition-colors z-20 relative cursor-pointer">
              {{ tag }}
            </button>
          </div>
          <router-link
            :to="`${article.article}`"
            class="text-2xl font-semibold text-gray-900 hover:text-emerald-600 transition-colors tracking-tight mb-2 before:absolute before:inset-0"
          >
            {{ article.title }}
          </router-link>
          <p class="text-gray-500 text-sm line-clamp-2">
            {{ article.article }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
