<script setup lang="ts">
import BookmarkIcon from './assets/book.svg'
import HomeIcon from './assets/home.svg'
import AddIcon from './assets/add.svg'
import CardIcon from './assets/card.svg'
import ListIcon from './assets/list.svg'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import emitter from './event-bus.ts'

const router = useRouter()

const showModal = ref(false)
const url = ref('')
const viewMode = ref<'card' | 'list'>('card')
const tags = ref<string[]>([])
const tagInput = ref('')
const isSubmitting = ref(false)

const submitForm = async () => {
  isSubmitting.value = true
  try{
    await fetch('/api/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ 
        url: url.value,
        Tags: tags.value
      }),
    })
    emitter.emit('article-added')
    showModal.value = false
    url.value = ''
    tags.value = []
    tagInput.value = ''
  }
  catch (err) {
    console.error('Submit failed', err)
  }
  finally{
    isSubmitting.value = false
  }
}

function closeModal() {
  showModal.value = false
}

const toggleViewMode = () => {
  viewMode.value = viewMode.value === 'card' ? 'list' : 'card'
  localStorage.setItem('viewMode', viewMode.value)
  router.push({ name: 'home', query: { view: viewMode.value } })
}

function addTag() {
  const trimmed = tagInput.value.trim()
  if (trimmed && !tags.value.includes(trimmed)) {
    tags.value.push(trimmed)
  }
  tagInput.value = ''
}

function removeTag(tag: string) {
  tags.value = tags.value.filter(t => t !== tag)
}
</script>

<template>
  <nav class="bg-white/70 backdrop-blur-xl border-b border-gray-200/50 w-full fixed top-0 left-0 z-50">
    <div class="max-w-6xl mx-auto px-6">
      <div class="flex justify-between items-center h-20">
        <div class="flex items-center space-x-2">
          <router-link to="/" class="flex items-center text-gray-900 group">
            <div class="p-2 bg-gray-100 rounded-lg group-hover:bg-gray-200 transition-colors duration-300">
              <BookmarkIcon class="w-5 h-5 text-gray-900" />
            </div>
            <span class="ml-3 text-gray-900 text-xl font-bold tracking-tight">Readr</span>
          </router-link>
        </div>

        <!-- Menu -->
        <div class="flex items-center space-x-4">
          <router-link to="/" class="text-gray-500 hover:text-gray-900 p-2 rounded-full hover:bg-gray-100 transition-colors">
            <HomeIcon class="w-5 h-5" />
          </router-link>
          
          <button @click="toggleViewMode" class="text-gray-500 hover:text-gray-900 p-2 rounded-full hover:bg-gray-100 transition-colors">
            <span v-if="viewMode === 'card'">
              <ListIcon class="w-5 h-5"/>
            </span>
            <span v-else>
              <CardIcon class="w-5 h-5"/>
            </span>
          </button>
          
          <button @click="showModal = true" class="flex items-center gap-2 bg-[#111] hover:bg-[#222] active:scale-95 text-white px-4 py-2 rounded-full transition-all duration-300 shadow-sm ml-2">
            <AddIcon class="w-4 h-4 text-white" />
            <span class="font-medium text-sm">Add Article</span>
          </button>
        </div>
      </div>
    </div>
  </nav>
  <transition name="fade-blur">
    <div v-if="showModal" @click.self="closeModal" class="fixed inset-0 bg-[#0a0a0a]/60 backdrop-blur-md flex justify-center items-center z-50 transition-all duration-300 ease-out p-4">
      <!-- Modal content -->
      <div class="bg-white rounded-3xl shadow-[0_8px_32px_rgba(0,0,0,0.08)] border border-gray-100 w-full max-w-md p-8 relative transform transition-all duration-300 scale-100 opacity-100">
        <button
          @click.self="closeModal"
          :disabled="isSubmitting"
          class="absolute top-6 right-6 text-gray-400 hover:text-gray-900 text-2xl leading-none font-light disabled:opacity-10 disabled:text-gray-700 transition-colors"
          aria-label="Close">&times;</button>
        <h2 class="text-2xl font-semibold mb-8 tracking-tight text-gray-900">Add an article</h2>

        <form @submit.prevent="submitForm" class="space-y-6">
          <div>
            <label for="url" class="block text-sm font-medium text-gray-700 mb-2">URL</label>
            <input
              v-model="url"
              type="url"
              id="url"
              required
              placeholder="https://example.com/article"
              class="w-full px-4 py-3 bg-gray-50 border-transparent rounded-xl focus:bg-white focus:border-gray-200 focus:ring-4 focus:ring-gray-100 focus:outline-none transition-all placeholder:text-gray-400 text-gray-900"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-2">Tags</label>
            <div v-if="tags.length > 0" class="flex flex-wrap gap-2 mb-3">
              <span v-for="tag in tags" :key="tag" class="bg-gray-100 text-gray-700 text-xs font-medium px-3 py-1.5 rounded-full flex items-center gap-1.5 border border-gray-200/60">
                {{ tag }}
                <button type="button" @click="removeTag(tag)" class="text-gray-400 hover:text-gray-700 transition-colors text-xs font-bold">&times;</button>
              </span>
            </div>
            <input
              v-model="tagInput"
              @keydown.enter.prevent="addTag"
              type="text"
              placeholder="Type tag and press Enter"
              class="w-full px-4 py-3 bg-gray-50 border-transparent rounded-xl focus:bg-white focus:border-gray-200 focus:ring-4 focus:ring-gray-100 focus:outline-none transition-all placeholder:text-gray-400 text-gray-900"
            />
          </div>
          <button
            type="submit"
            :disabled="isSubmitting"
            class="bg-[#111] text-white px-4 py-3.5 rounded-xl hover:bg-[#222] active:scale-[0.98] w-full font-medium transition-all duration-300 disabled:bg-gray-300 disabled:text-gray-500 disabled:cursor-not-allowed mt-4"
          >
            {{ isSubmitting ? 'Saving...' : 'Save article' }}
          </button>
        </form>
      </div>
    </div>
  </transition>
  <main class="w-full max-w-6xl mx-auto px-6 pt-32 pb-16 flex-grow">
    <router-view  />
  </main>
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