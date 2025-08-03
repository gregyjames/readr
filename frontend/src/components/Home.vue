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

onMounted(async () => {
  const res = await axios.get('http://127.0.0.1:3000/getarticles')
  articles.value = res.data
})
</script>

<template>
  <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-6">
    <div v-for="article in articles" :key="article.ID" class="flex-1 bg-white rounded-xl shadow-md overflow-hidden">
      <div class="md:flex h-full">
        <div class="w-full md:w-48 aspect-square flex-shrink-0">
          <img :src="article.image" alt="Cover" class="w-full h-full object-cover" />
        </div>
        <div class="p-6 flex flex-col justify-center">
          <router-link :to="`${article.article}`" class="block mt-1 text-lg leading-tight font-medium text-black hover:underline">
            {{ article.title }}
          </router-link>
          <p class="mt-2 text-gray-500 text-sm">
            {{ article.article.slice(0, 120) }}...
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
