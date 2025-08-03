<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

interface Article {
  ID: number
  title: string
  article: string
  image: string
}

defineProps<{ msg: string }>()

const articles = ref<Article[]>([])
const viewMode = ref<'card' | 'list'>('card')

onMounted(async () => {
  const res = await axios.get('http://127.0.0.1:3000/getarticles')
  articles.value = res.data
})
</script>

<template>
  <div>
    <div class="flex pt-20 justify-between items-center mb-6">
      <h2 class="text-2xl font-bold text-gray-800 dark:text-white">{{ msg }}</h2>
      <button
        @click="viewMode = viewMode === 'card' ? 'list' : 'card'"
        class="px-4 py-2 bg-green-600 text-white rounded-md hover:bg-green-700 transition"
      >
        Switch to {{ viewMode === 'card' ? 'List' : 'Card' }} View
      </button>
    </div>

    <!-- Card View -->
    <div v-if="viewMode === 'card'" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
      <div v-for="article in articles" :key="article.ID" class="flex-1 bg-white rounded-xl shadow-md overflow-hidden">
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
              {{ article.article.slice(0, 120) }}...
            </p>
          </div>
        </div>
      </div>
    </div>

    <!-- List View -->
    <div v-else class="space-y-4">
      <div v-for="article in articles" :key="article.ID" class="flex items-center bg-white rounded-xl shadow-sm p-4">
        <img :src="article.image" alt="Cover" class="w-24 h-24 object-cover rounded-md mr-4" />
        <div>
          <router-link
            :to="`${article.article}`"
            class="text-lg font-medium text-black hover:underline"
          >
            {{ article.title }}
          </router-link>
          <p class="text-gray-500 text-sm">
            {{ article.article.slice(0, 120) }}...
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
