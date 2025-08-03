<script setup lang="ts">
import BookmarkIcon from './assets/book.svg'
import HomeIcon from './assets/home.svg'
import AddIcon from './assets/add.svg'
import CardIcon from './assets/card.svg'
import ListIcon from './assets/list.svg'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
const router = useRouter()

const showModal = ref(false)
const url = ref('')
const viewMode = ref<'card' | 'list'>('card')

const submitForm = async () => {
  await fetch('http://localhost:3000/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url: url.value }),
  })
  showModal.value = false
}

function closeModal() {
  showModal.value = false
}

const toggleViewMode = () => {
  viewMode.value = viewMode.value === 'card' ? 'list' : 'card'
  localStorage.setItem('viewMode', viewMode.value)
  router.push({ name: 'home', query: { view: viewMode.value } })
}
</script>

<template>
  <nav class="bg-green-600 w-full shadow-md fixed top-0 left-0 z-50">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex justify-between items-center h-16">
        <div class="flex items-center space-x-2">
          <!-- SVG or Image Logo -->
          <BookmarkIcon class="w-6 h-6 text-white" />
      
          <!-- Text -->
          <span class="text-white text-lg font-semibold">Readr</span>
        </div>

        <!-- Menu -->
        <div class="flex space-x-6">
          <router-link to="/" class="text-white hover:bg-green-700 p-2 rounded"><HomeIcon class="w-6 h-6 text-white" /></router-link>
          <a @click="showModal = true" class="text-white hover:bg-green-700 p-2 rounded"><AddIcon class="w-6 h-6 text-white" /></a>
          <button @click="toggleViewMode" class="p-2 rounded flex items-center gap-2 bg-green-600 text-white hover:bg-green-700 transition">
            <span v-if="viewMode === 'card'">
              <ListIcon class="w-6 h-6 text-white"/>
            </span>
            <span v-else>
              <CardIcon class="w-6 h-6 text-white"/>
            </span>
          </button>
        </div>
      </div>
    </div>
  </nav>
  <transition name="fade-blur">
    <div v-if="showModal" @click.self="closeModal" class="fixed inset-0 bg-black/65 bg-blur backdrop-blur-sm flex justify-center items-center z-50 transition-opacity duration-300 ease-out">
      <!-- Modal content -->
      <div class="bg-white rounded-lg shadow-lg w-full max-w-md p-6 relative">
        <button
          @click.self="closeModal"
          class="absolute top-2 right-2 text-gray-400 hover:text-gray-800 text-xl font-bold"
          aria-label="Close">×</button>
        <h2 class="text-xl font-semibold mb-4">Add an article</h2>

        <form @submit.prevent="submitForm" class="space-y-4">
          <div>
            <label for="url" class="block text-sm font-medium text-gray-700">URL</label>
            <input
              v-model="url"
              type="url"
              id="url"
              required
              placeholder="https://example.com"
              class="w-full px-4 py-2 border border-gray-300 rounded focus:ring-2 focus:ring-blue-500 focus:outline-none"
            />
          </div>

          <button
            type="submit"
            class="bg-green-600 text-white px-4 py-2 rounded hover:bg-green-700 w-full"
          >
            Submit
          </button>
        </form>
      </div>
    </div>
  </transition>
  <div class="container w-full">
    <router-view />
  </div>
</template>


<style scoped>
.fade-blur-enter-active,
.fade-blur-leave-active {
  transition: opacity 0.3s ease, backdrop-filter 0.3s ease;
}
.fade-blur-enter-from,
.fade-blur-leave-to {
  opacity: 0;
  backdrop-filter: blur(0px);
}
.fade-blur-enter-to,
.fade-blur-leave-from {
  opacity: 1;
  backdrop-filter: blur(8px);
}
</style>