<script setup lang="ts">
import { ref, onMounted, watch, computed } from 'vue'
import axios from 'axios'

interface Article {
  ID: number
  title: string
  article: string
  image: string
  tags: string
}

defineProps<{ msg: string }>()

const articles = ref<Article[]>([])

import { useRoute, useRouter} from 'vue-router'
const route = useRoute()
const router = useRouter()

const viewMode = ref<'card' | 'list'>('card')
const selectedTag = ref<string | null>(null)

onMounted(async () => {
  const res = await axios.get('http://127.0.0.1:3000/getarticles')
  articles.value = res.data

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
})

watch(() => route.query.view, (newView) => {
  if (newView === 'card' || newView === 'list') {
    viewMode.value = newView
    localStorage.setItem('viewMode', newView)
  }
})

const filteredArticles = computed(() => {
  if (!selectedTag.value) return articles.value
  return articles.value.filter(article =>
    article.tags?.split(',').map(tag => tag.trim()).includes(selectedTag.value!)
  )
})

</script>

<template>
  <div>

    <div v-if="selectedTag" class="flex justify-end items-center mb-4">
      <p class="text-sm mr-2 text-gray-600">
        Filtering by tag: <strong>{{ selectedTag }}</strong>
      </p>
      <button @click="selectedTag = null" class="text-green-600 hover:underline text-sm">
        Clear filter
      </button>
    </div>
    <!-- Card View -->
    <div v-if="viewMode === 'card'" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
      <div v-for="article in filteredArticles" :key="article.ID" class="flex-1 bg-white rounded-xl shadow-md overflow-hidden border border-gray-500 ">
        <div class="md:flex h-full">
          <div class="w-full md:w-48 aspect-square flex-shrink-0">
            <img :src="article.image" alt="Cover" class="w-full h-full object-cover" />
          </div>
          <div class="p-6 flex flex-col justify-center">
            <router-link
              :to="`${article.article}`"
              class="block mt-1 text-lg leading-tight font-medium text-black hover:underline"
            >
              {{ article.title }}
            </router-link>
            <p class="mt-2 text-gray-500 text-sm">
              {{ article.article }}
            </p>
            <div class="mt-3 flex flex-wrap gap-2 justify-center">
              <span v-for="tag in article.tags.split(',').slice(0, 5)" :key="tag" @click="selectedTag = tag.trim()" class="bg-green-100 text-green-800 text-xs font-semibold px-2.5 py-0.5 rounded">
                {{ tag.trim() }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- List View -->
    <div v-else class="space-y-10 mt-10">
      <div v-for="article in filteredArticles" :key="article.ID" class="flex bg-white rounded-xl shadow-sm p-4 border border-gray-500 ">
        <img :src="article.image" alt="Cover" class="w-24 h-24 object-cover rounded-md mr-4" />
        <div>
          <router-link
            :to="`${article.article}`"
            class="text-lg font-medium text-black hover:underline"
          >
            {{ article.title }}
          </router-link>
          <p class="text-gray-500 text-sm text-left">
            {{ article.article }}
          </p>
          <div class="mt-3 flex flex-wrap gap-2">
            <span v-for="tag in article.tags.split(',').slice(0, 5)" :key="tag" @click="selectedTag = tag.trim()" class="bg-green-100 text-green-800 text-xs font-semibold px-2.5 py-0.5 rounded">
              {{ tag.trim() }}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
