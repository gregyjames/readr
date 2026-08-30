<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'

const apiKey = ref('')
const showKey = ref(false)
const savedMessage = ref('')
const isKeyConfigured = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

const showSavedMessage = (msg: string) => {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
  savedMessage.value = msg
  timer = setTimeout(() => {
    savedMessage.value = ''
    timer = null
  }, 3000)
}

onMounted(() => {
  const existingKey = localStorage.getItem('OPENROUTER_API_KEY') || ''
  apiKey.value = existingKey
  isKeyConfigured.value = Boolean(existingKey.trim())
})

onUnmounted(() => {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
})

const saveKey = () => {
  const trimmed = apiKey.value.trim()
  if (trimmed) {
    localStorage.setItem('OPENROUTER_API_KEY', trimmed)
    isKeyConfigured.value = true
    showSavedMessage('API key saved successfully!')
  } else {
    localStorage.removeItem('OPENROUTER_API_KEY')
    isKeyConfigured.value = false
    showSavedMessage('API key removed.')
  }
}

const clearKey = () => {
  apiKey.value = ''
  localStorage.removeItem('OPENROUTER_API_KEY')
  isKeyConfigured.value = false
  showSavedMessage('API key cleared.')
}
</script>

<template>
  <div class="max-w-2xl mx-auto py-8">
    <!-- Header -->
    <div class="mb-8">
      <h1 class="text-3xl font-bold tracking-tight text-gray-900 dark:text-gray-100">Settings</h1>
      <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
        Configure your integrations and API credentials for Readr AI assistant.
      </p>
    </div>

    <!-- Main Settings Card -->
    <div class="bg-white dark:bg-[#111] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.2)] p-6 sm:p-8 space-y-6">
      
      <!-- Section Title & Status -->
      <div class="flex items-center justify-between pb-5 border-b border-gray-100 dark:border-gray-800">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">OpenRouter API</h2>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
            Required for chat conversations and querying your saved articles with AI.
          </p>
        </div>
        <span 
          class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium"
          :class="isKeyConfigured ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800' : 'bg-amber-50 text-amber-700 dark:bg-amber-950/50 dark:text-amber-400 border border-amber-200 dark:border-amber-800'"
        >
          <span class="w-1.5 h-1.5 rounded-full" :class="isKeyConfigured ? 'bg-emerald-500' : 'bg-amber-500'"></span>
          {{ isKeyConfigured ? 'Connected' : 'Not Configured' }}
        </span>
      </div>

      <!-- Key Input Field -->
      <div class="space-y-2">
        <label for="api-key" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
          OpenRouter API Key
        </label>
        <div class="relative">
          <input
            id="api-key"
            v-model="apiKey"
            :type="showKey ? 'text' : 'password'"
            placeholder="sk-or-v1-..."
            class="w-full pl-4 pr-12 py-3 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-gray-300 dark:focus:border-gray-600 focus:ring-4 focus:ring-gray-100 dark:focus:ring-gray-800 focus:outline-none transition-all placeholder:text-gray-400 dark:placeholder:text-gray-600 text-gray-900 dark:text-gray-100 text-sm font-mono"
          />
          <button
            type="button"
            @click="showKey = !showKey"
            class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 p-1.5 rounded-lg transition-colors"
            :title="showKey ? 'Hide key' : 'Show key'"
          >
            <svg v-if="!showKey" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
              <circle cx="12" cy="12" r="3" />
            </svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M9.88 9.88a3 3 0 1 0 4.24 4.24" />
              <path d="M10.73 5.08A10.43 10.43 0 0 1 12 5c7 0 10 7 10 7a13.16 13.16 0 0 1-1.67 2.68" />
              <path d="M6.61 6.61A13.526 13.526 0 0 0 2 12s3 7 10 7a9.74 9.74 0 0 0 5.39-1.61" />
              <line x1="2" y1="2" x2="22" y2="22" />
            </svg>
          </button>
        </div>
        <p class="text-xs text-gray-400 dark:text-gray-500">
          Your key is saved locally in your browser's local storage and never exposed on our public servers.
        </p>
      </div>

      <!-- Action buttons & Feedback -->
      <div class="flex items-center justify-between pt-2">
        <div class="flex items-center gap-3">
          <button
            @click="saveKey"
            class="bg-[#111] dark:bg-white text-white dark:text-[#111] px-5 py-2.5 rounded-xl hover:bg-[#222] dark:hover:bg-gray-100 active:scale-[0.98] text-sm font-medium transition-all duration-200 shadow-sm cursor-pointer"
          >
            Save Key
          </button>
          <button
            v-if="isKeyConfigured"
            @click="clearKey"
            class="bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 px-4 py-2.5 rounded-xl text-sm font-medium transition-all duration-200 cursor-pointer"
          >
            Remove
          </button>
        </div>

        <transition name="fade">
          <span v-if="savedMessage" class="text-xs font-medium text-emerald-600 dark:text-emerald-400">
            {{ savedMessage }}
          </span>
        </transition>
      </div>

      <!-- Info Box -->
      <div class="mt-6 p-4 rounded-2xl bg-gray-50 dark:bg-[#161616] border border-gray-200/50 dark:border-gray-800/50 text-xs text-gray-600 dark:text-gray-400 leading-relaxed">
        <p class="font-medium text-gray-800 dark:text-gray-200 mb-1">Need an API key?</p>
        <p>
          Get one directly from 
          <a
            href="https://openrouter.ai/keys"
            target="_blank"
            rel="noopener noreferrer"
            class="text-emerald-600 dark:text-emerald-400 hover:underline font-medium inline-flex items-center gap-0.5"
          >
            OpenRouter.ai
            <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
              <polyline points="15 3 21 3 21 9"></polyline>
              <line x1="10" y1="14" x2="21" y2="3"></line>
            </svg>
          </a>.
          OpenRouter provides access to models like Claude 3.5 Sonnet, GPT-4o, and Llama 3 with unified pay-as-you-go pricing.
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
