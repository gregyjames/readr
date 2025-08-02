<script setup lang="ts">
import { ref, onMounted } from 'vue'
import axios from 'axios'

defineProps<{ msg: string }>()
const msg = ref('Loading...')


const articles = [
  {
    id: 1,
    title: 'Finding customers for your new business',
    type: 'Case study',
    image: 'https://plus.unsplash.com/premium_photo-1753820822882-e841b54f942a?w=900&auto=format&fit=crop&q=60&ixlib=rb-4.1.0&ixid=M3wxMjA3fDB8MHxmZWF0dXJlZC1waG90b3MtZmVlZHw3N3x8fGVufDB8fHx8fA%3D%3D',
    summary:
      'Getting a new business off the ground is hard. Here are five ideas to find your first customers.',
  },
  {
    id: 2,
    title: 'Launching an online store in 2025',
    type: 'Tutorial',
    image: 'https://plus.unsplash.com/premium_photo-1753211477530-ac7d65a3119f?q=80&w=1295&auto=format&fit=crop&ixlib=rb-4.1.0&ixid=M3wxMjA3fDB8MHxwaG90by1wYWdlfHx8fGVufDB8fHx8fA%3D%3D',
    summary:
      'Learn how to set up your e-commerce website with modern frameworks and customer-centered design.',
  },
  {
    id: 3,
    title: 'The power of branding for growth',
    type: 'Article',
    image: 'https://images.unsplash.com/photo-1744190070145-505730b69566?w=900&auto=format&fit=crop&q=60&ixlib=rb-4.1.0&ixid=M3wxMjA3fDB8MHxmZWF0dXJlZC1waG90b3MtZmVlZHw3NHx8fGVufDB8fHx8fA%3D%3D',
    summary:
      'Good branding builds trust and drives growth. Let’s explore key visual and messaging strategies.',
  },
]

onMounted(async () => {
const res = await axios.get('http://127.0.0.1:3000/')
console.log(res.data.message)
msg.value = res.data.message
});

</script>

<template>
  <h1>{{ msg }}</h1>
  <h1 class="mb-4 text-4xl font-extrabold leading-none tracking-tight text-gray-900 md:text-5xl lg:text-6xl dark:text-white">We invest in the world’s potential</h1>
  <p class="mb-6 text-lg font-normal text-gray-200 lg:text-xl sm:px-16 xl:px-48">Here at Flowbite we focus on markets where technology, innovation, and capital can unlock long-term value and drive economic growth.</p>
  <div class="grid gap-4 grid-cols-3">
    <!-- Card 1 -->
    <div v-for="article in articles" :key="article.id" class="flex-1 bg-white rounded-xl shadow-md overflow-hidden max-w-full">
      <div class="md:flex h-full">
          <div class="w-full md:w-48 aspect-square flex-shrink-0">
            <img :src="article.image" alt="Cover" class="w-full h-full object-cover rounded-l-xl" />
          </div>
          <div class="p-6 flex flex-col justify-center">
          <div class="uppercase tracking-wide text-sm text-indigo-500 font-semibold">
            {{ article.type }}
          </div>
          <a href="#" class="block mt-1 text-lg leading-tight font-medium text-black hover:underline">
            {{ article.title }}
          </a>
          <p class="mt-2 text-gray-500">
            {{ article.summary }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
