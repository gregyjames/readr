<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'

interface ModelItem {
  id: string
  name: string
  description?: string
  context_length?: number
  pricing?: {
    prompt: string
    completion: string
  }
}

const apiKey = ref('')
const showKey = ref(false)
const savedMessage = ref('')
const isKeyConfigured = ref(false)

const selectedModel = ref('openai/gpt-4o-mini')
const models = ref<ModelItem[]>([])

const isDark = ref(document.documentElement.classList.contains('dark'))
const viewMode = ref(localStorage.getItem('viewMode') || 'card')

const toggleTheme = () => {
  const root = document.documentElement
  if (root.classList.contains('dark')) {
    root.classList.remove('dark')
    localStorage.setItem('theme', 'light')
    isDark.value = false
  } else {
    root.classList.add('dark')
    localStorage.setItem('theme', 'dark')
    isDark.value = true
  }
}

const setViewMode = (mode: string) => {
  viewMode.value = mode
  localStorage.setItem('viewMode', mode)
}
const isLoadingModels = ref(false)
const modelSearch = ref('')
const isModelDropdownOpen = ref(false)
const expandGraphContext = ref(true)
const enableAgentEnricher = ref(localStorage.getItem('AGENT_ENRICHER') !== 'false') // Default true
const enableAgentLinker = ref(localStorage.getItem('AGENT_LINKER') !== 'false') // Default true

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

const popularModels = [
  'openai/gpt-4o',
  'openai/gpt-4o-mini',
  'anthropic/claude-3.5-sonnet',
  'anthropic/claude-3.5-haiku',
  'meta-llama/llama-3.3-70b-instruct',
  'google/gemini-2.0-flash-exp:free',
  'deepseek/deepseek-chat',
  'deepseek/deepseek-r1',
]

const filteredModels = computed(() => {
  if (!modelSearch.value.trim()) {
    return models.value
  }
  const q = modelSearch.value.toLowerCase()
  return models.value.filter(m => 
    m.id.toLowerCase().includes(q) || (m.name && m.name.toLowerCase().includes(q))
  )
})

const selectedModelObj = computed(() => {
  return models.value.find(m => m.id === selectedModel.value) || {
    id: selectedModel.value,
    name: selectedModel.value,
  }
})

const fetchModels = async () => {
  isLoadingModels.value = true
  try {
    const headers: Record<string, string> = {}
    if (apiKey.value.trim()) {
      headers['Authorization'] = `Bearer ${apiKey.value.trim()}`
    }
    const res = await fetch('/api/models', { headers })
    if (res.ok) {
      const json = await res.json()
      if (json.data && Array.isArray(json.data)) {
        models.value = json.data.sort((a: ModelItem, b: ModelItem) => {
          const aPop = popularModels.includes(a.id) ? 0 : 1
          const bPop = popularModels.includes(b.id) ? 0 : 1
          if (aPop !== bPop) return aPop - bPop
          return a.name ? a.name.localeCompare(b.name || '') : a.id.localeCompare(b.id)
        })
      }
    }
  } catch (err) {
    console.error('Failed to load OpenRouter models:', err)
  } finally {
    isLoadingModels.value = false
  }
}

onMounted(() => {
  const existingKey = localStorage.getItem('OPENROUTER_API_KEY') || ''
  apiKey.value = existingKey
  isKeyConfigured.value = Boolean(existingKey.trim())

  const savedModel = localStorage.getItem('OPENROUTER_MODEL')
  if (savedModel) {
    selectedModel.value = savedModel
  }

  const savedExpansion = localStorage.getItem('GRAPH_CONTEXT_EXPANSION')
  if (savedExpansion !== null) {
    expandGraphContext.value = savedExpansion === 'true'
  }

  fetchModels()
})

onUnmounted(() => {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
})

const selectModel = (modelId: string) => {
  selectedModel.value = modelId
  localStorage.setItem('OPENROUTER_MODEL', modelId)
  isModelDropdownOpen.value = false
  showSavedMessage(`Model set to ${modelId}`)
}

const saveSettings = () => {
  const trimmed = apiKey.value.trim()
  if (trimmed) {
    localStorage.setItem('OPENROUTER_API_KEY', trimmed)
    isKeyConfigured.value = true
  } else {
    localStorage.removeItem('OPENROUTER_API_KEY')
    isKeyConfigured.value = false
  }

  localStorage.setItem('OPENROUTER_MODEL', selectedModel.value)
  localStorage.setItem('GRAPH_CONTEXT_EXPANSION', String(expandGraphContext.value))
  localStorage.setItem('AGENT_ENRICHER', enableAgentEnricher.value ? 'true' : 'false')
  localStorage.setItem('AGENT_LINKER', enableAgentLinker.value ? 'true' : 'false')
  showSavedMessage('Settings saved successfully!')
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
        Configure your OpenRouter API key and preferred AI model for Readr chat.
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

      <!-- Model Selection Dropdown -->
      <div class="space-y-2 pt-2 border-t border-gray-100 dark:border-gray-800">
        <div class="flex items-center justify-between">
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Default AI Model
          </label>
          <button 
            type="button" 
            @click="fetchModels" 
            class="text-xs text-emerald-600 dark:text-emerald-400 hover:underline flex items-center gap-1 cursor-pointer"
            :disabled="isLoadingModels"
          >
            <svg v-if="isLoadingModels" class="animate-spin h-3 w-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            {{ isLoadingModels ? 'Refreshing...' : 'Refresh Models' }}
          </button>
        </div>

        <!-- Custom Dropdown with search -->
        <div class="relative">
          <button
            type="button"
            @click="isModelDropdownOpen = !isModelDropdownOpen"
            class="w-full px-4 py-3 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl flex items-center justify-between text-left text-sm text-gray-900 dark:text-gray-100 hover:border-gray-300 dark:hover:border-gray-700 transition-colors cursor-pointer"
          >
            <div class="truncate pr-4">
              <div class="font-medium text-gray-900 dark:text-gray-100">
                {{ selectedModelObj.name || selectedModelObj.id }}
              </div>
              <div class="text-xs text-gray-400 dark:text-gray-500 font-mono truncate">
                {{ selectedModelObj.id }}
              </div>
            </div>
            <svg class="w-4 h-4 text-gray-400 shrink-0 transition-transform" :class="{ 'rotate-180': isModelDropdownOpen }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
            </svg>
          </button>

          <!-- Dropdown Menu -->
          <div 
            v-if="isModelDropdownOpen"
            class="absolute z-50 mt-2 w-full bg-white dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-2xl shadow-xl overflow-hidden"
          >
            <!-- Search input inside dropdown -->
            <div class="p-3 border-b border-gray-100 dark:border-gray-800 bg-gray-50/50 dark:bg-[#161616]">
              <input
                v-model="modelSearch"
                type="text"
                placeholder="Search models (e.g. claude, gpt, llama, deepseek)..."
                class="w-full px-3 py-2 text-xs bg-white dark:bg-[#111] border border-gray-200 dark:border-gray-800 rounded-lg focus:outline-none focus:border-emerald-500 text-gray-900 dark:text-gray-100 placeholder:text-gray-400"
                @click.stop
              />
            </div>

            <!-- Options list -->
            <div class="max-h-64 overflow-y-auto py-1">
              <div 
                v-if="filteredModels.length === 0" 
                class="p-4 text-center text-xs text-gray-400 dark:text-gray-500"
              >
                {{ isLoadingModels ? 'Loading OpenRouter models...' : 'No matching models found.' }}
              </div>
              <button
                v-for="m in filteredModels"
                :key="m.id"
                type="button"
                @click="selectModel(m.id)"
                class="w-full px-4 py-2.5 text-left text-xs hover:bg-gray-100 dark:hover:bg-white/5 transition-colors flex items-center justify-between cursor-pointer"
                :class="{ 'bg-emerald-50/60 dark:bg-emerald-950/20 text-emerald-700 dark:text-emerald-400 font-medium': m.id === selectedModel }"
              >
                <div class="truncate pr-2">
                  <div class="font-medium text-gray-900 dark:text-gray-100 truncate">
                    {{ m.name || m.id }}
                  </div>
                  <div class="text-[11px] text-gray-400 dark:text-gray-500 font-mono truncate">
                    {{ m.id }}
                  </div>
                </div>
                <span v-if="m.id === selectedModel" class="text-emerald-600 dark:text-emerald-400 shrink-0 text-xs">
                  ✓
                </span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- App Appearance Settings -->
      <div class="pt-4 border-t border-gray-100 dark:border-gray-800 space-y-4">
        <!-- Theme Toggle -->
        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-gray-100">Dark Mode</div>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">Switch between light and dark themes.</p>
          </div>
          <button
            type="button"
            @click="toggleTheme"
            class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="isDark ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="isDark ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
        </div>

        <!-- Default View Mode Toggle -->
        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-gray-100">Default Feed Layout</div>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">Choose between card or list view for articles.</p>
          </div>
          <div class="flex bg-gray-100 dark:bg-[#1a1a1a] p-1 rounded-xl">
            <button
              type="button"
              @click="setViewMode('card')"
              class="px-3 py-1.5 text-xs font-medium rounded-lg transition-all cursor-pointer"
              :class="viewMode === 'card' ? 'bg-white dark:bg-[#2a2a2a] text-gray-900 dark:text-white shadow-sm' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
            >
              Card
            </button>
            <button
              type="button"
              @click="setViewMode('list')"
              class="px-3 py-1.5 text-xs font-medium rounded-lg transition-all cursor-pointer"
              :class="viewMode === 'list' ? 'bg-white dark:bg-[#2a2a2a] text-gray-900 dark:text-white shadow-sm' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
            >
              List
            </button>
          </div>
        </div>
      </div>

      <!-- Graph Context Expansion Toggle -->
      <div class="pt-4 border-t border-gray-100 dark:border-gray-800 flex items-center justify-between">
        <div>
          <div class="text-sm font-medium text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <span>1-Hop Graph Context Expansion</span>
            <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-400">Knowledge Graph</span>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 max-w-md">
            When you reference an article with @, automatically include directly connected notes and backlinks in the AI's context.
          </p>
        </div>
        <button
          type="button"
          @click="expandGraphContext = !expandGraphContext"
          class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
          :class="expandGraphContext ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="expandGraphContext ? 'translate-x-5' : 'translate-x-0'"
          />
        </button>
      </div>

      <!-- Background Agents Toggles -->
      <div class="pt-4 border-t border-gray-100 dark:border-gray-800 space-y-4">
        <div>
          <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Background Agents
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
            Automate vault enrichment and graph weaving when new articles are added or reparsed.
          </p>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-gray-100 flex items-center gap-2">
              <span>OKF Frontmatter Enricher</span>
              <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-blue-100 dark:bg-blue-950/60 text-blue-700 dark:text-blue-400">LLM</span>
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 max-w-md">
              Extract and format OKF YAML frontmatter when new articles are added.
            </p>
          </div>
          <button
            type="button"
            @click="enableAgentEnricher = !enableAgentEnricher"
            class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="enableAgentEnricher ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="enableAgentEnricher ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-gray-100 flex items-center gap-2">
              <span>Autonomous Graph Linker</span>
              <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-blue-100 dark:bg-blue-950/60 text-blue-700 dark:text-blue-400">LLM</span>
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 max-w-md">
              Automatically weave new articles into the Knowledge Graph by injecting markdown wikilinks.
            </p>
          </div>
          <button
            type="button"
            @click="enableAgentLinker = !enableAgentLinker"
            class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="enableAgentLinker ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="enableAgentLinker ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
        </div>
      </div>

      <!-- Action buttons & Feedback -->
      <div class="flex items-center justify-between pt-4">
        <div class="flex items-center gap-3">
          <button
            @click="saveSettings"
            class="bg-[#111] dark:bg-white text-white dark:text-[#111] px-5 py-2.5 rounded-xl hover:bg-[#222] dark:hover:bg-gray-100 active:scale-[0.98] text-sm font-medium transition-all duration-200 shadow-sm cursor-pointer"
          >
            Save Settings
          </button>
          <button
            v-if="isKeyConfigured"
            @click="clearKey"
            class="bg-gray-100 dark:bg-gray-800 text-gray-600 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700 px-4 py-2.5 rounded-xl text-sm font-medium transition-all duration-200 cursor-pointer"
          >
            Clear Key
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
          OpenRouter provides access to 200+ models like Claude 3.5 Sonnet, GPT-4o, DeepSeek R1, and Llama 3.3.
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
