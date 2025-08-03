<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import axios from 'axios'

interface Article {
  ID: number
  title: string
  article: string
  image: string
}

defineProps<{ msg: string }>()

const articles = ref<Article[]>([])

import { useRoute, useRouter} from 'vue-router'
const route = useRoute()
const router = useRouter()

const viewMode = ref<'card' | 'list'>('card')

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
</script>

<template>
  <div>
    <div class="flex pt-20 justify-between items-center mb-6">
      
    </div>

    <!-- Card View -->
    <div v-if="viewMode === 'card'" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
      <div v-for="article in articles" :key="article.ID" class="flex-1 bg-white rounded-xl shadow-md overflow-hidden border border-gray-500 ">
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
          </div>
        </div>
      </div>
    </div>

    <!-- List View -->
    <div v-else class="space-y-4">
      <div v-for="article in articles" :key="article.ID" class="flex bg-white rounded-xl shadow-sm p-4 border border-gray-500 ">
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
        </div>
      </div>
    </div>
  </div>
</template>
