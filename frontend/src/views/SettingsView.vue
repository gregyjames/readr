<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { settings, saveSettingsToServer, toggleTheme, setViewMode } from '../store/settings'
import { authState, changePassword, getStoredToken } from '../store/auth'
import { generateBookmarkletCode } from '../utils/bookmarklet'

// Bookmarklet State
const bookmarkletServerUrl = ref(typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080')
const bookmarkletAuthToken = ref(getStoredToken() || '')
const copiedBookmarklet = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | null = null

const generatedBookmarkletHref = computed(() => {
  return generateBookmarkletCode({
    serverUrl: bookmarkletServerUrl.value,
    authToken: bookmarkletAuthToken.value || undefined,
  })
})

const copyBookmarkletCode = async () => {
  try {
    await navigator.clipboard.writeText(generatedBookmarkletHref.value)
    copiedBookmarklet.value = true
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copiedBookmarklet.value = false
      copyTimer = null
    }, 2500)
  } catch (err) {
    console.error('Failed to copy bookmarklet code:', err)
  }
}

// Diagnostics Tab State
const activeTab = ref<'general' | 'diagnostics'>('general')

interface PipelineRun {
  id: number
  article_id: number
  article_title: string
  model: string
  status: string
  duration_ms: number
  retry_count: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens?: number
  error_message: string
  created_at: string
}

interface PipelineDiagnosticsData {
  queue: {
    pending_jobs: number
    active_jobs?: number
    total_in_flight?: number
    max_capacity: number
    total_workers?: number
    busy_workers?: number
  }
  summary: {
    total_runs: number
    successful_runs: number
    failed_runs: number
    total_retries: number
    avg_duration_ms: number
    p95_duration_ms: number
    total_tokens_used: number
    total_prompt_tokens: number
    total_completion_tokens: number
  }
  recent_runs: PipelineRun[]
}

const diagnosticsData = ref<PipelineDiagnosticsData | null>(null)
const isLoadingDiagnostics = ref(false)
const isAutoRefresh = ref(true)
const statusFilter = ref<'all' | 'success' | 'failed'>('all')
const expandedRunId = ref<number | null>(null)
let diagnosticsPollTimer: ReturnType<typeof setInterval> | null = null

const fetchDiagnostics = async (showLoading = false) => {
  if (showLoading) isLoadingDiagnostics.value = true
  try {
    const res = await fetch('/api/diagnostics/pipeline')
    if (res.ok) {
      diagnosticsData.value = await res.json()
    }
  } catch (err) {
    console.error('Failed to fetch pipeline diagnostics:', err)
  } finally {
    if (showLoading) isLoadingDiagnostics.value = false
  }
}

const startDiagnosticsPolling = () => {
  stopDiagnosticsPolling()
  if (isAutoRefresh.value) {
    diagnosticsPollTimer = setInterval(() => {
      if (activeTab.value === 'diagnostics') {
        fetchDiagnostics(false)
      }
    }, 3000)
  }
}

const stopDiagnosticsPolling = () => {
  if (diagnosticsPollTimer) {
    clearInterval(diagnosticsPollTimer)
    diagnosticsPollTimer = null
  }
}

const toggleAutoRefresh = () => {
  isAutoRefresh.value = !isAutoRefresh.value
  if (isAutoRefresh.value) {
    startDiagnosticsPolling()
  } else {
    stopDiagnosticsPolling()
  }
}

const toggleExpandRun = (id: number) => {
  expandedRunId.value = expandedRunId.value === id ? null : id
}

const filteredRuns = computed(() => {
  if (!diagnosticsData.value?.recent_runs) return []
  if (statusFilter.value === 'all') return diagnosticsData.value.recent_runs
  return diagnosticsData.value.recent_runs.filter(r => r.status === statusFilter.value)
})

const successRate = computed(() => {
  if (!diagnosticsData.value || diagnosticsData.value.summary.total_runs === 0) return 100
  const rate = (diagnosticsData.value.summary.successful_runs / diagnosticsData.value.summary.total_runs) * 100
  return Math.round(rate * 10) / 10
})

const formatMs = (ms: number): string => {
  if (!ms || ms <= 0) return '0s'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

const formatTokens = (tokens: number): string => {
  if (!tokens || tokens <= 0) return '0'
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(1)}M`
  if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(1)}k`
  return `${tokens}`
}

const formatDate = (iso: string): string => {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) + ' ' + d.toLocaleDateString([], { month: 'short', day: 'numeric' })
  } catch {
    return iso
  }
}

watch(activeTab, (newTab) => {
  if (newTab === 'diagnostics') {
    fetchDiagnostics(true)
    startDiagnosticsPolling()
  } else {
    stopDiagnosticsPolling()
  }
})

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

const showKey = ref(false)
const savedMessage = ref('')

const isKeyConfigured = computed(() => Boolean(settings.api_key?.trim()))


const isLoadingModels = ref(false)
const modelSearch = ref('')
const isModelDropdownOpen = ref(false)
const models = ref<ModelItem[]>([])

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
  return models.value.find(m => m.id === settings.model) || {
    id: settings.model,
    name: settings.model,
  }
})

const fetchModels = async () => {
  isLoadingModels.value = true
  try {
    const headers: Record<string, string> = {}
    if (settings.api_key?.trim()) {
      headers['Authorization'] = `Bearer ${settings.api_key.trim()}`
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
  fetchModels()
  fetchDiagnostics()
})

onUnmounted(() => {
  if (timer) {
    clearTimeout(timer)
    timer = null
  }
  if (passwordTimer) {
    clearTimeout(passwordTimer)
    passwordTimer = null
  }
  if (copyTimer) {
    clearTimeout(copyTimer)
    copyTimer = null
  }
  stopDiagnosticsPolling()
})

const selectModel = (modelId: string) => {
  settings.model = modelId
  isModelDropdownOpen.value = false
  saveSettingsToServer()
  showSavedMessage(`Model set to ${modelId}`)
}

const saveSettings = async () => {
  try {
    await saveSettingsToServer()
    showSavedMessage('Settings saved successfully!')
  } catch (e) {
    console.error(e)
    alert('Failed to save settings')
  }
}

const clearKey = async () => {
  settings.api_key = ''
  try {
    await saveSettingsToServer()
    showSavedMessage('API key cleared.')
  } catch (e) {
    console.error(e)
    alert('Failed to clear API key')
  }
}

// Master Password State & Logic
const currentPassword = ref('')
const newPassword = ref('')
const confirmNewPassword = ref('')
const showCurrentPassword = ref(false)
const showNewPassword = ref(false)
const showConfirmNewPassword = ref(false)
const isUpdatingPassword = ref(false)
const passwordError = ref('')
const passwordSuccess = ref('')
let passwordTimer: ReturnType<typeof setTimeout> | null = null

const handleChangePassword = async () => {
  passwordError.value = ''
  passwordSuccess.value = ''

  if (authState.isAuthConfigured && !currentPassword.value) {
    passwordError.value = 'Please enter your current password'
    return
  }
  if (!newPassword.value) {
    passwordError.value = 'Please enter a new password'
    return
  }
  if (newPassword.value.length < 6) {
    passwordError.value = 'New password must be at least 6 characters'
    return
  }
  if (newPassword.value !== confirmNewPassword.value) {
    passwordError.value = 'New passwords do not match'
    return
  }

  isUpdatingPassword.value = true
  const res = await changePassword(currentPassword.value, newPassword.value)
  isUpdatingPassword.value = false

  if (res.success) {
    authState.isAuthConfigured = true
    authState.isAuthenticated = true
    passwordSuccess.value = authState.isAuthConfigured ? 'Master password updated successfully!' : 'Master password set successfully!'
    currentPassword.value = ''
    newPassword.value = ''
    confirmNewPassword.value = ''
    if (passwordTimer) clearTimeout(passwordTimer)
    passwordTimer = setTimeout(() => {
      passwordSuccess.value = ''
      passwordTimer = null
    }, 4000)
  } else {
    passwordError.value = res.error || 'Failed to update master password'
  }
}

// Vault Maintenance: Clean Broken Links
const isCleaningLinks = ref(false)
const cleanLinksResult = ref<{
  scanned_articles: number
  updated_articles: number
  cleaned_links: number
  purged_db_links: number
} | null>(null)
const cleanLinksError = ref<string | null>(null)

const cleanBrokenLinks = async () => {
  isCleaningLinks.value = true
  cleanLinksResult.value = null
  cleanLinksError.value = null
  try {
    const res = await fetch('/api/vault/clean-links', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      }
    })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      throw new Error(data.error || 'Failed to clean broken links')
    }
    cleanLinksResult.value = await res.json()
  } catch (err: any) {
    cleanLinksError.value = err.message || 'An unexpected error occurred during vault cleanup'
  } finally {
    isCleaningLinks.value = false
  }
}

// Librarian Agent: Manual MOC Synthesis
const isExecutingLibrarian = ref(false)
const librarianResult = ref<{
  status: string
  scanned_articles: number
  clusters_detected: number
  created_mocs: number
  updated_mocs: number
  execution_time_ms: number
  errors?: string[]
} | null>(null)
const librarianError = ref<string | null>(null)

const executeLibrarian = async () => {
  isExecutingLibrarian.value = true
  librarianResult.value = null
  librarianError.value = null
  try {
    const res = await fetch('/api/librarian/run', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      }
    })
    if (!res.ok) {
      const data = await res.json().catch(() => ({}))
      throw new Error(data.error || 'Failed to execute Librarian agent')
    }
    librarianResult.value = await res.json()
  } catch (err: any) {
    librarianError.value = err.message || 'An unexpected error occurred during Librarian execution'
  } finally {
    isExecutingLibrarian.value = false
  }
}
</script>

<template>
  <div class="mx-auto py-6 space-y-6 transition-all duration-200" :class="activeTab === 'diagnostics' ? 'max-w-5xl' : 'max-w-2xl'">
    <!-- Header & Tab Navigation -->
    <div class="space-y-4">
      <div>
        <h1 class="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-100 font-['Outfit']">Settings</h1>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Configure OpenRouter AI models, pipeline enrichment agents, and telemetry diagnostics.
        </p>
      </div>

      <!-- Segmented Tab Buttons -->
      <div class="inline-flex items-center bg-gray-100/80 dark:bg-white/[0.04] p-0.5 rounded-lg border border-gray-200/60 dark:border-white/[0.06]">
        <button
          type="button"
          @click="activeTab = 'general'"
          class="px-3 py-1.5 text-xs font-medium rounded-md transition-all cursor-pointer flex items-center gap-1.5"
          :class="activeTab === 'general' ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs' : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="3"></circle>
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
          </svg>
          General
        </button>

        <button
          type="button"
          @click="activeTab = 'diagnostics'"
          class="px-3 py-1.5 text-xs font-medium rounded-md transition-all cursor-pointer flex items-center gap-1.5"
          :class="activeTab === 'diagnostics' ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs' : 'text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-white'"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"></polyline>
          </svg>
          Pipeline Diagnostics
          <span
            v-if="diagnosticsData && ((diagnosticsData.queue.total_in_flight ?? diagnosticsData.queue.pending_jobs) > 0)"
            class="px-1.5 py-0.2 rounded-full text-[10px] font-mono font-bold bg-emerald-500 text-white"
          >
            {{ diagnosticsData.queue.total_in_flight ?? diagnosticsData.queue.pending_jobs }}
          </span>
        </button>
      </div>
    </div>

    <!-- General Settings Tab Content -->
    <div v-if="activeTab === 'general'" class="space-y-6">
    <!-- Main Settings Card -->
    <div class="bg-white dark:bg-[#12151C] rounded-xl border border-gray-200/80 dark:border-white/[0.08] shadow-2xs p-5 sm:p-6 space-y-5">
      
      <!-- Section Title & Status -->
      <div class="flex items-center justify-between pb-4 border-b border-gray-100 dark:border-white/[0.06]">
        <div>
          <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">OpenRouter Integration</h2>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
            Required for background document parsing, auto-tagging, and chat.
          </p>
        </div>
        <span 
          class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded text-xs font-mono"
          :class="isKeyConfigured ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/20' : 'bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-500/20'"
        >
          <span class="w-1.5 h-1.5 rounded-full" :class="isKeyConfigured ? 'bg-emerald-500' : 'bg-amber-500'"></span>
          {{ isKeyConfigured ? 'Connected' : 'Missing Key' }}
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
            v-model="settings.api_key"
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
                :class="{ 'bg-emerald-50/60 dark:bg-emerald-950/20 text-emerald-700 dark:text-emerald-400 font-medium': m.id === settings.model }"
              >
                <div class="truncate pr-2">
                  <div class="font-medium text-gray-900 dark:text-gray-100 truncate">
                    {{ m.name || m.id }}
                  </div>
                  <div class="text-[11px] text-gray-400 dark:text-gray-500 font-mono truncate">
                    {{ m.id }}
                  </div>
                </div>
                <span v-if="m.id === settings.model" class="text-emerald-600 dark:text-emerald-400 shrink-0 text-xs">
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
            :class="settings.theme === 'dark' ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="settings.theme === 'dark' ? 'translate-x-5' : 'translate-x-0'"
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
              :class="settings.view_mode === 'card' ? 'bg-white dark:bg-[#2a2a2a] text-gray-900 dark:text-white shadow-sm' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
            >
              Card
            </button>
            <button
              type="button"
              @click="setViewMode('list')"
              class="px-3 py-1.5 text-xs font-medium rounded-lg transition-all cursor-pointer"
              :class="settings.view_mode === 'list' ? 'bg-white dark:bg-[#2a2a2a] text-gray-900 dark:text-white shadow-sm' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
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
          @click="settings.graph_context_expansion = !settings.graph_context_expansion"
          class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
          :class="settings.graph_context_expansion ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="settings.graph_context_expansion ? 'translate-x-5' : 'translate-x-0'"
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
            @click="settings.agent_enricher = !settings.agent_enricher"
            aria-label="Toggle OKF Frontmatter Enricher"
            :aria-pressed="settings.agent_enricher"
            class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="settings.agent_enricher ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="settings.agent_enricher ? 'translate-x-5' : 'translate-x-0'"
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
            @click="settings.agent_linker = !settings.agent_linker"
            aria-label="Toggle Autonomous Graph Linker"
            :aria-pressed="settings.agent_linker"
            class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="settings.agent_linker ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="settings.agent_linker ? 'translate-x-5' : 'translate-x-0'"
            />
          </button>
        </div>

        <div class="flex items-center justify-between">
          <div>
            <div class="text-sm font-medium text-gray-900 dark:text-gray-100 flex items-center gap-2">
              <span>Executive Summarizer</span>
              <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-blue-100 dark:bg-blue-950/60 text-blue-700 dark:text-blue-400">LLM</span>
            </div>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-1 max-w-md">
              Automatically generate concise 2–3 sentence executive summaries when templates include a summary block.
            </p>
          </div>
          <button
            type="button"
            @click="settings.agent_summarizer = !settings.agent_summarizer"
            aria-label="Toggle Executive Summarizer"
            :aria-pressed="settings.agent_summarizer"
            class="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
            :class="settings.agent_summarizer ? 'bg-emerald-600' : 'bg-gray-200 dark:bg-gray-700'"
          >
            <span
              class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
              :class="settings.agent_summarizer ? 'translate-x-5' : 'translate-x-0'"
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

    <!-- Security & Master Password Card -->
    <div class="bg-white dark:bg-[#111] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.2)] p-6 sm:p-8 space-y-6">
      
      <!-- Section Title & Status -->
      <div class="flex items-center justify-between pb-5 border-b border-gray-100 dark:border-gray-800">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">Security & Master Password</h2>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
            Update your master password used to authenticate and protect your knowledge vault.
          </p>
        </div>
        <span 
          class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          {{ authState.isAuthConfigured ? 'Protected' : 'Not Set' }}
        </span>
      </div>

      <!-- Notifications -->
      <transition name="fade">
        <div v-if="passwordError" class="p-4 rounded-2xl bg-red-50 dark:bg-red-500/10 border border-red-200/80 dark:border-red-500/20 text-red-600 dark:text-red-400 text-xs flex items-center justify-between">
          <div class="flex items-center gap-2">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="shrink-0">
              <circle cx="12" cy="12" r="10"></circle>
              <line x1="12" y1="8" x2="12" y2="12"></line>
              <line x1="12" y1="16" x2="12.01" y2="16"></line>
            </svg>
            <span>{{ passwordError }}</span>
          </div>
          <button type="button" @click="passwordError = ''" class="text-red-400 hover:text-red-600 dark:hover:text-red-300 font-bold ml-2 cursor-pointer">&times;</button>
        </div>
      </transition>

      <transition name="fade">
        <div v-if="passwordSuccess" class="p-4 rounded-2xl bg-emerald-50 dark:bg-emerald-500/10 border border-emerald-200/80 dark:border-emerald-500/20 text-emerald-700 dark:text-emerald-400 text-xs flex items-center justify-between">
          <div class="flex items-center gap-2">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="shrink-0">
              <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path>
              <polyline points="22 4 12 14.01 9 11.01"></polyline>
            </svg>
            <span>{{ passwordSuccess }}</span>
          </div>
          <button type="button" @click="passwordSuccess = ''" class="text-emerald-400 hover:text-emerald-600 dark:hover:text-emerald-300 font-bold ml-2 cursor-pointer">&times;</button>
        </div>
      </transition>

      <!-- Password Form -->
      <form @submit.prevent="handleChangePassword" class="space-y-4">
        <!-- Current Password (only if already configured) -->
        <div v-if="authState.isAuthConfigured" class="space-y-2">
          <label for="current-password" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Current Password
          </label>
          <div class="relative">
            <input
              id="current-password"
              v-model="currentPassword"
              :type="showCurrentPassword ? 'text' : 'password'"
              placeholder="••••••••••••"
              class="w-full pl-4 pr-12 py-3 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-gray-300 dark:focus:border-gray-600 focus:ring-4 focus:ring-gray-100 dark:focus:ring-gray-800 focus:outline-none transition-all placeholder:text-gray-400 dark:placeholder:text-gray-600 text-gray-900 dark:text-gray-100 text-sm font-mono"
            />
            <button
              type="button"
              @click="showCurrentPassword = !showCurrentPassword"
              class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 p-1.5 rounded-lg transition-colors cursor-pointer"
              :title="showCurrentPassword ? 'Hide password' : 'Show password'"
            >
              <svg v-if="!showCurrentPassword" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
        </div>

        <!-- New Password and Confirm New Password -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="space-y-2">
            <label for="new-password" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
              New Password
            </label>
            <div class="relative">
              <input
                id="new-password"
                v-model="newPassword"
                :type="showNewPassword ? 'text' : 'password'"
                placeholder="••••••••••••"
                class="w-full pl-4 pr-12 py-3 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-gray-300 dark:focus:border-gray-600 focus:ring-4 focus:ring-gray-100 dark:focus:ring-gray-800 focus:outline-none transition-all placeholder:text-gray-400 dark:placeholder:text-gray-600 text-gray-900 dark:text-gray-100 text-sm font-mono"
                required
              />
              <button
                type="button"
                @click="showNewPassword = !showNewPassword"
                class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 p-1.5 rounded-lg transition-colors cursor-pointer"
                :title="showNewPassword ? 'Hide password' : 'Show password'"
              >
                <svg v-if="!showNewPassword" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
          </div>

          <div class="space-y-2">
            <label for="confirm-new-password" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
              Confirm New Password
            </label>
            <div class="relative">
              <input
                id="confirm-new-password"
                v-model="confirmNewPassword"
                :type="showConfirmNewPassword ? 'text' : 'password'"
                placeholder="••••••••••••"
                class="w-full pl-4 pr-12 py-3 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-gray-300 dark:focus:border-gray-600 focus:ring-4 focus:ring-gray-100 dark:focus:ring-gray-800 focus:outline-none transition-all placeholder:text-gray-400 dark:placeholder:text-gray-600 text-gray-900 dark:text-gray-100 text-sm font-mono"
                required
              />
              <button
                type="button"
                @click="showConfirmNewPassword = !showConfirmNewPassword"
                class="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 p-1.5 rounded-lg transition-colors cursor-pointer"
                :title="showConfirmNewPassword ? 'Hide password' : 'Show password'"
              >
                <svg v-if="!showConfirmNewPassword" xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
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
          </div>
        </div>

        <!-- Submit Button -->
        <div class="pt-2">
          <button
            type="submit"
            :disabled="isUpdatingPassword"
            class="bg-[#111] dark:bg-white text-white dark:text-[#111] px-5 py-2.5 rounded-xl hover:bg-[#222] dark:hover:bg-gray-100 active:scale-[0.98] text-sm font-medium transition-all duration-200 shadow-sm disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            {{ isUpdatingPassword ? 'Updating Password...' : 'Change Master Password' }}
          </button>
        </div>
      </form>
    </div>

    <!-- Librarian & Map of Content (MOC) Agent Card -->
    <div class="bg-white dark:bg-[#111] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 p-6 sm:p-8 space-y-6 shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.2)]">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-amber-500"><path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1-2.5-2.5Z"/><path d="M6 6h10"/><path d="M6 10h10"/></svg>
            Librarian & Map of Content (MOC) Agent
          </h2>
          <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">
            Autonomous background curator that synthesizes, groups, and incrementally updates hub notes for dense topic clusters.
          </p>
        </div>

        <label class="relative inline-flex items-center cursor-pointer">
          <input
            type="checkbox"
            v-model="settings.librarian_enabled"
            @change="saveSettings"
            class="sr-only peer"
          />
          <div class="w-11 h-6 bg-gray-200 peer-focus:outline-none rounded-full peer dark:bg-gray-800 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all dark:border-gray-600 peer-checked:bg-amber-500"></div>
        </label>
      </div>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <!-- Cron Schedule -->
        <div class="space-y-1.5">
          <label class="text-xs font-medium text-gray-700 dark:text-gray-300 flex items-center justify-between">
            <span>Cron Schedule</span>
            <span class="text-[11px] text-gray-400 font-mono">0 0 * * * = Daily 12am</span>
          </label>
          <input
            v-model="settings.librarian_cron"
            @blur="saveSettings"
            type="text"
            placeholder="0 0 * * *"
            class="w-full px-3.5 py-2 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl text-xs font-mono focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-amber-500 focus:outline-none transition-all text-gray-900 dark:text-gray-100"
          />
        </div>

        <!-- Min Cluster Size -->
        <div class="space-y-1.5">
          <label class="text-xs font-medium text-gray-700 dark:text-gray-300 flex items-center justify-between">
            <span>Minimum Cluster Size</span>
            <span class="text-[11px] text-gray-400">Related articles required</span>
          </label>
          <input
            v-model.number="settings.librarian_min_cluster_size"
            @blur="saveSettings"
            type="number"
            min="2"
            max="50"
            placeholder="5"
            class="w-full px-3.5 py-2 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl text-xs font-mono focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-amber-500 focus:outline-none transition-all text-gray-900 dark:text-gray-100"
          />
        </div>
      </div>

      <!-- Result Banner -->
      <div v-if="librarianResult" class="p-4 rounded-2xl bg-amber-50/80 dark:bg-amber-950/30 border border-amber-200/60 dark:border-amber-800/40 text-xs text-amber-800 dark:text-amber-300 flex items-start gap-3">
        <svg v-if="librarianResult.status === 'success'" class="w-5 h-5 shrink-0 text-amber-600 dark:text-amber-400 mt-0.5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <svg v-else class="w-5 h-5 shrink-0 text-amber-600 dark:text-amber-400 mt-0.5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <div class="space-y-1">
          <template v-if="librarianResult.status === 'success'">
            <p class="font-semibold">Librarian Execution Complete</p>
            <p>
              Scanned <strong>{{ librarianResult.scanned_articles }}</strong> articles across <strong>{{ librarianResult.clusters_detected }}</strong> clusters • Created <strong>{{ librarianResult.created_mocs }}</strong> new MOCs • Updated <strong>{{ librarianResult.updated_mocs }}</strong> existing MOCs in <strong>{{ librarianResult.execution_time_ms }}ms</strong>.
            </p>
          </template>
          <template v-else-if="librarianResult.status === 'skipped (no api key)'">
            <p class="font-semibold">Librarian Skipped</p>
            <p>No OpenRouter API key configured. Please add an API key in General Settings to enable MOC synthesis.</p>
          </template>
          <template v-else-if="librarianResult.status === 'skipped (disabled)'">
            <p class="font-semibold">Librarian Skipped</p>
            <p>Librarian agent is currently disabled in settings.</p>
          </template>
          <template v-else-if="librarianResult.status.startsWith('skipped')">
            <p class="font-semibold">Librarian Skipped ({{ librarianResult.status }})</p>
            <p>
              Scanned <strong>{{ librarianResult.scanned_articles }}</strong> articles across <strong>{{ librarianResult.clusters_detected }}</strong> clusters • All MOCs are up to date.
            </p>
          </template>
          <template v-else-if="librarianResult.status.includes('partial')">
            <p class="font-semibold">Librarian Execution Partial</p>
            <p>
              Scanned <strong>{{ librarianResult.scanned_articles }}</strong> articles • Created <strong>{{ librarianResult.created_mocs }}</strong> MOCs • Updated <strong>{{ librarianResult.updated_mocs }}</strong> MOCs with some cluster errors in <strong>{{ librarianResult.execution_time_ms }}ms</strong>.
            </p>
          </template>
          <template v-else>
            <p class="font-semibold">Librarian Status: {{ librarianResult.status }}</p>
            <p>
              Scanned <strong>{{ librarianResult.scanned_articles }}</strong> articles across <strong>{{ librarianResult.clusters_detected }}</strong> clusters in <strong>{{ librarianResult.execution_time_ms }}ms</strong>.
            </p>
          </template>
        </div>
      </div>

      <!-- Error Banner -->
      <div v-if="librarianError" class="p-4 rounded-2xl bg-red-50 dark:bg-red-950/30 border border-red-200/60 dark:border-red-800/40 text-xs text-red-800 dark:text-red-300 flex items-start gap-3">
        <svg class="w-5 h-5 shrink-0 text-red-600 dark:text-red-400 mt-0.5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <div>
          <p class="font-semibold">Librarian Execution Failed</p>
          <p>{{ librarianError }}</p>
        </div>
      </div>

      <div class="flex items-center justify-between pt-2">
        <div class="text-xs text-gray-500 dark:text-gray-400">
          Preserves user notes under <code>## Notes & Synthesis</code> while generating structured hub wikilinks.
        </div>
        <button
          type="button"
          @click="executeLibrarian"
          :disabled="isExecutingLibrarian"
          class="bg-amber-50 hover:bg-amber-100 dark:bg-amber-950/40 dark:hover:bg-amber-900/60 text-amber-800 dark:text-amber-300 border border-amber-200 dark:border-amber-800/60 px-5 py-2.5 rounded-xl active:scale-[0.98] text-sm font-medium transition-all duration-200 shadow-sm disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer flex items-center gap-2"
        >
          <svg v-if="isExecutingLibrarian" class="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
          </svg>
          <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/>
          </svg>
          {{ isExecutingLibrarian ? 'Synthesizing MOCs...' : 'Run Librarian Now' }}
        </button>
      </div>
    </div>

    <!-- Vault Maintenance Card -->
    <div class="bg-white dark:bg-[#111] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 p-6 sm:p-8 space-y-6 shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.2)]">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-emerald-500"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
            Vault Maintenance
          </h2>
          <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">
            Scan all markdown notes in your vault to convert broken wikilinks to clean text and purge orphaned database relations.
          </p>
        </div>
      </div>

      <!-- Result Banner -->
      <div v-if="cleanLinksResult" class="p-4 rounded-2xl bg-emerald-50/80 dark:bg-emerald-950/30 border border-emerald-200/60 dark:border-emerald-800/40 text-xs text-emerald-800 dark:text-emerald-300 flex items-start gap-3">
        <svg class="w-5 h-5 shrink-0 text-emerald-600 dark:text-emerald-400 mt-0.5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <div class="space-y-1">
          <p class="font-semibold">Vault Clean Up Complete</p>
          <p>
            Scanned <strong>{{ cleanLinksResult.scanned_articles }}</strong> notes • Cleaned <strong>{{ cleanLinksResult.cleaned_links }}</strong> broken links across <strong>{{ cleanLinksResult.updated_articles }}</strong> notes • Purged <strong>{{ cleanLinksResult.purged_db_links }}</strong> orphaned graph links.
          </p>
        </div>
      </div>

      <!-- Error Banner -->
      <div v-if="cleanLinksError" class="p-4 rounded-2xl bg-red-50 dark:bg-red-950/30 border border-red-200/60 dark:border-red-800/40 text-xs text-red-800 dark:text-red-300 flex items-start gap-3">
        <svg class="w-5 h-5 shrink-0 text-red-600 dark:text-red-400 mt-0.5" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <div>
          <p class="font-semibold">Cleanup Failed</p>
          <p>{{ cleanLinksError }}</p>
        </div>
      </div>

      <div class="flex items-center justify-between pt-2">
        <div class="text-xs text-gray-500 dark:text-gray-400">
          Preserves original note wording and sentence flow while removing unresolved brackets.
        </div>
        <button
          type="button"
          @click="cleanBrokenLinks"
          :disabled="isCleaningLinks"
          class="bg-gray-100 hover:bg-gray-200 dark:bg-white/10 dark:hover:bg-white/15 text-gray-900 dark:text-gray-100 px-5 py-2.5 rounded-xl active:scale-[0.98] text-sm font-medium transition-all duration-200 shadow-sm disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer flex items-center gap-2"
        >
          <svg v-if="isCleaningLinks" class="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
          </svg>
          <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>
          </svg>
          {{ isCleaningLinks ? 'Scanning Vault...' : 'Clean Broken Links' }}
        </button>
      </div>
    </div>

    <!-- Browser Bookmarklet Card -->
    <div class="bg-white dark:bg-[#111] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 p-6 sm:p-8 space-y-6 shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.2)]">
      <div class="flex items-start justify-between gap-4 pb-5 border-b border-gray-100 dark:border-gray-800">
        <div>
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
            <span class="text-lg" aria-hidden="true">🔖</span>
            Browser Bookmarklet
            <span class="px-2 py-0.5 rounded-full text-[10px] font-bold bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-400">1-Click Save</span>
          </h2>
          <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">
            Save articles, blog posts, and documentation directly into your Readr vault from any webpage with custom tags.
          </p>
        </div>
      </div>

      <!-- Config Inputs -->
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div class="space-y-2">
          <label for="bookmarklet-server-url" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Server URL
          </label>
          <input
            id="bookmarklet-server-url"
            v-model="bookmarkletServerUrl"
            type="text"
            placeholder="http://localhost:8080"
            class="w-full px-4 py-2.5 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-emerald-500 text-gray-900 dark:text-gray-100 text-xs font-mono"
          />
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            Address of your Readr instance reachable from your browser.
          </p>
        </div>

        <div class="space-y-2">
          <label for="bookmarklet-token" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            Authentication Token (Optional)
          </label>
          <input
            id="bookmarklet-token"
            v-model="bookmarkletAuthToken"
            type="password"
            placeholder="Bearer token (if protected)"
            class="w-full px-4 py-2.5 bg-gray-50 dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-xl focus:bg-white dark:focus:bg-[#1a1a1a] focus:border-emerald-500 text-gray-900 dark:text-gray-100 text-xs font-mono"
          />
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            Pre-filled with your current active session token if logged in.
          </p>
        </div>
      </div>

      <!-- Interactive Bookmarklet Actions -->
      <div class="p-5 rounded-2xl bg-gradient-to-br from-emerald-50/50 to-teal-50/30 dark:from-emerald-950/20 dark:to-teal-950/10 border border-emerald-200/60 dark:border-emerald-800/40 flex flex-col sm:flex-row items-center justify-between gap-4">
        <div class="space-y-1 text-center sm:text-left">
          <div class="text-xs font-semibold uppercase tracking-wider text-emerald-800 dark:text-emerald-400">
            Install Bookmarklet
          </div>
          <p class="text-xs text-gray-600 dark:text-gray-300">
            Drag the button to your browser bookmarks bar, or click to copy the JavaScript code.
          </p>
        </div>

        <div class="flex items-center gap-3 shrink-0">
          <!-- Draggable Link Button -->
          <a
            :href="generatedBookmarkletHref"
            @click.prevent="copyBookmarkletCode"
            title="Drag to your bookmarks bar"
            class="inline-flex items-center gap-2 px-4 py-2.5 bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold rounded-xl shadow-md hover:shadow-lg transition-all transform active:scale-95 cursor-grab select-none"
          >
            <span>📚</span>
            <span>Save to Readr</span>
          </a>

          <!-- Copy Button -->
          <button
            type="button"
            @click="copyBookmarkletCode"
            class="px-3.5 py-2.5 bg-white dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 hover:border-gray-300 dark:hover:border-gray-700 text-gray-700 dark:text-gray-200 text-xs font-medium rounded-xl transition-colors cursor-pointer flex items-center gap-1.5"
          >
            <svg v-if="copiedBookmarklet" class="w-4 h-4 text-emerald-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
            </svg>
            <svg v-else class="w-4 h-4 text-gray-400" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
            </svg>
            {{ copiedBookmarklet ? 'Copied Code!' : 'Copy Code' }}
          </button>
        </div>
      </div>

      <!-- Quick Setup Instructions -->
      <div class="space-y-3 pt-2">
        <h3 class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">
          Browser Installation Guide
        </h3>
        <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-3 text-xs text-gray-600 dark:text-gray-400">
          <div class="p-3 bg-gray-50 dark:bg-[#161616] rounded-xl border border-gray-200/50 dark:border-gray-800/50">
            <span class="font-semibold text-gray-800 dark:text-gray-200">Chrome / Brave</span>
            <p class="mt-1 text-[11px] leading-relaxed">
              Press <kbd class="px-1 py-0.5 bg-gray-200 dark:bg-gray-800 rounded font-mono">Cmd+Shift+B</kbd> to show bookmarks bar, then drag the button above into it.
            </p>
          </div>
          <div class="p-3 bg-gray-50 dark:bg-[#161616] rounded-xl border border-gray-200/50 dark:border-gray-800/50">
            <span class="font-semibold text-gray-800 dark:text-gray-200">Safari</span>
            <p class="mt-1 text-[11px] leading-relaxed">
              Press <kbd class="px-1 py-0.5 bg-gray-200 dark:bg-gray-800 rounded font-mono">Cmd+Shift+B</kbd> for Favorites Bar, then drag the button into Favorites.
            </p>
          </div>
          <div class="p-3 bg-gray-50 dark:bg-[#161616] rounded-xl border border-gray-200/50 dark:border-gray-800/50">
            <span class="font-semibold text-gray-800 dark:text-gray-200">Firefox</span>
            <p class="mt-1 text-[11px] leading-relaxed">
              Right-click the bookmarks toolbar &rarr; Bookmarks Toolbar &rarr; Always Show. Drag the button directly onto it.
            </p>
          </div>
          <div class="p-3 bg-gray-50 dark:bg-[#161616] rounded-xl border border-gray-200/50 dark:border-gray-800/50">
            <span class="font-semibold text-gray-800 dark:text-gray-200">Edge</span>
            <p class="mt-1 text-[11px] leading-relaxed">
              Press <kbd class="px-1 py-0.5 bg-gray-200 dark:bg-gray-800 rounded font-mono">Ctrl+Shift+B</kbd> to show favorites bar, then drag the button onto the bar.
            </p>
          </div>
        </div>
      </div>
    </div>
    </div> <!-- closes activeTab === 'general' -->

    <!-- Diagnostics Tab Content -->
    <div v-else-if="activeTab === 'diagnostics'" class="space-y-6">
      
      <!-- Top Action Bar -->
      <div class="bg-white dark:bg-[#111] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 p-5 shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.2)] flex flex-wrap items-center justify-between gap-4">
        <div class="flex items-center gap-3">
          <div class="relative flex h-3 w-3">
            <span v-if="(diagnosticsData?.queue.total_in_flight ?? diagnosticsData?.queue.pending_jobs ?? 0) > 0" class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
            <span class="relative inline-flex rounded-full h-3 w-3" :class="(diagnosticsData?.queue.total_in_flight ?? diagnosticsData?.queue.pending_jobs ?? 0) > 0 ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-gray-600'"></span>
          </div>
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100 flex items-center gap-2">
              Agent Pipeline Status
              <span class="text-xs font-normal text-gray-500 dark:text-gray-400">
                • {{ (diagnosticsData?.queue.total_in_flight ?? diagnosticsData?.queue.pending_jobs ?? 0) > 0 ? ((diagnosticsData?.queue.active_jobs ?? 0) > 0 ? 'Processing (' + diagnosticsData?.queue.active_jobs + ' active, ' + diagnosticsData?.queue.pending_jobs + ' queued)' : diagnosticsData?.queue.pending_jobs + ' queued') : 'Worker Ready (Idle)' }}
              </span>
            </h2>
            <p class="text-xs text-gray-400 dark:text-gray-500">
              Auto-refreshing queue telemetry and OpenRouter execution analytics.
            </p>
          </div>
        </div>

        <div class="flex items-center gap-3">
          <!-- Auto-Refresh Toggle -->
          <button
            type="button"
            @click="toggleAutoRefresh"
            class="px-3 py-1.5 rounded-xl text-xs font-medium border transition-colors flex items-center gap-1.5 cursor-pointer"
            :class="isAutoRefresh ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-400 border-emerald-200 dark:border-emerald-800' : 'bg-gray-50 text-gray-600 dark:bg-white/5 dark:text-gray-400 border-gray-200 dark:border-gray-800'"
          >
            <span class="w-1.5 h-1.5 rounded-full" :class="isAutoRefresh ? 'bg-emerald-500 animate-pulse' : 'bg-gray-400'"></span>
            Auto-refresh {{ isAutoRefresh ? 'ON (3s)' : 'OFF' }}
          </button>

          <!-- Manual Refresh Button -->
          <button
            type="button"
            @click="fetchDiagnostics(true)"
            :disabled="isLoadingDiagnostics"
            class="px-3 py-1.5 rounded-xl text-xs font-medium bg-gray-100 hover:bg-gray-200 dark:bg-white/10 dark:hover:bg-white/15 text-gray-700 dark:text-gray-300 transition-colors flex items-center gap-1.5 cursor-pointer disabled:opacity-50"
          >
            <svg class="w-3.5 h-3.5" :class="{ 'animate-spin': isLoadingDiagnostics }" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            Refresh
          </button>
        </div>
      </div>

      <!-- 4 Stat Metric Cards -->
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <!-- Card 1: Queue Depth -->
        <div class="bg-white dark:bg-[#111] rounded-2xl border border-gray-200/70 dark:border-gray-800/70 p-5 shadow-sm space-y-2">
          <div class="flex items-center justify-between text-xs font-medium text-gray-500 dark:text-gray-400">
            <span>Pipeline In-Flight</span>
            <span class="px-2 py-0.5 rounded-full text-[10px] font-semibold" :class="(diagnosticsData?.queue.total_in_flight ?? diagnosticsData?.queue.pending_jobs ?? 0) > 0 ? 'bg-amber-100 text-amber-800 dark:bg-amber-950/60 dark:text-amber-300' : 'bg-gray-100 text-gray-600 dark:bg-white/5 dark:text-gray-400'">
              {{ (diagnosticsData?.queue.total_in_flight ?? diagnosticsData?.queue.pending_jobs ?? 0) > 0 ? 'Active' : 'Idle' }}
            </span>
          </div>
          <div class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 font-mono">
            {{ diagnosticsData?.queue.total_in_flight ?? diagnosticsData?.queue.pending_jobs ?? 0 }} <span class="text-xs text-gray-400 font-normal">/ {{ diagnosticsData?.queue.max_capacity ?? 100 }} max</span>
          </div>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ diagnosticsData?.queue.active_jobs ?? 0 }} active worker • {{ diagnosticsData?.queue.pending_jobs ?? 0 }} in buffer
          </p>
        </div>

        <!-- Card 2: Success Rate -->
        <div class="bg-white dark:bg-[#111] rounded-2xl border border-gray-200/70 dark:border-gray-800/70 p-5 shadow-sm space-y-2">
          <div class="flex items-center justify-between text-xs font-medium text-gray-500 dark:text-gray-400">
            <span>Success Rate</span>
            <span class="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-100 text-emerald-800 dark:bg-emerald-950/60 dark:text-emerald-300">
              {{ diagnosticsData?.summary.successful_runs ?? 0 }} passed
            </span>
          </div>
          <div class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 font-mono">
            {{ successRate }}%
          </div>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ diagnosticsData?.summary.total_runs ?? 0 }} total runs ({{ diagnosticsData?.summary.failed_runs ?? 0 }} failed)
          </p>
        </div>

        <!-- Card 3: Performance & Retries -->
        <div class="bg-white dark:bg-[#111] rounded-2xl border border-gray-200/70 dark:border-gray-800/70 p-5 shadow-sm space-y-2">
          <div class="flex items-center justify-between text-xs font-medium text-gray-500 dark:text-gray-400">
            <span>Avg / P95 Latency</span>
            <span v-if="(diagnosticsData?.summary.total_retries ?? 0) > 0" class="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-amber-100 text-amber-800 dark:bg-amber-950/60 dark:text-amber-300">
              {{ diagnosticsData?.summary.total_retries }} retries
            </span>
            <span v-else class="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-gray-100 text-gray-600 dark:bg-white/5 dark:text-gray-400">
              0 retries
            </span>
          </div>
          <div class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 font-mono">
            {{ formatMs(diagnosticsData?.summary.avg_duration_ms ?? 0) }}
            <span class="text-xs text-gray-400 font-normal">avg</span>
          </div>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            P95: {{ formatMs(diagnosticsData?.summary.p95_duration_ms ?? 0) }} across runs
          </p>
        </div>

        <!-- Card 4: Total Token Consumption -->
        <div class="bg-white dark:bg-[#111] rounded-2xl border border-gray-200/70 dark:border-gray-800/70 p-5 shadow-sm space-y-2">
          <div class="flex items-center justify-between text-xs font-medium text-gray-500 dark:text-gray-400">
            <span>Tokens Consumed</span>
            <span class="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-gray-100 text-gray-700 dark:bg-white/5 dark:text-gray-300">
              API Exact
            </span>
          </div>
          <div class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 font-mono">
            {{ formatTokens(diagnosticsData?.summary.total_tokens_used ?? 0) }}
          </div>
          <p class="text-[11px] text-gray-400 dark:text-gray-500">
            {{ formatTokens(diagnosticsData?.summary.total_prompt_tokens ?? 0) }} prompt • {{ formatTokens(diagnosticsData?.summary.total_completion_tokens ?? 0) }} completion
          </p>
        </div>
      </div>

      <!-- Execution History Card & Table -->
      <div class="bg-white dark:bg-[#111] rounded-3xl border border-gray-200/70 dark:border-gray-800/70 shadow-[0_4px_24px_rgba(0,0,0,0.04)] dark:shadow-[0_4px_24px_rgba(0,0,0,0.2)] overflow-hidden">
        
        <!-- Table Header & Filter Bar -->
        <div class="p-5 sm:p-6 border-b border-gray-100 dark:border-gray-800 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">Pipeline Execution History</h3>
            <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
              Historical logs of agent enrichment runs, latency durations, and retry counts.
            </p>
          </div>

          <!-- Status Filters -->
          <div class="flex items-center bg-gray-100 dark:bg-[#1a1a1a] p-1 rounded-xl">
            <button
              type="button"
              @click="statusFilter = 'all'"
              class="px-3 py-1.5 text-xs font-medium rounded-lg transition-all cursor-pointer"
              :class="statusFilter === 'all' ? 'bg-white dark:bg-[#2a2a2a] text-gray-900 dark:text-white shadow-sm' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
            >
              All ({{ diagnosticsData?.recent_runs.length ?? 0 }})
            </button>
            <button
              type="button"
              @click="statusFilter = 'success'"
              class="px-3 py-1.5 text-xs font-medium rounded-lg transition-all cursor-pointer"
              :class="statusFilter === 'success' ? 'bg-white dark:bg-[#2a2a2a] text-emerald-600 dark:text-emerald-400 shadow-sm' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
            >
              Success ({{ diagnosticsData?.summary.successful_runs ?? 0 }})
            </button>
            <button
              type="button"
              @click="statusFilter = 'failed'"
              class="px-3 py-1.5 text-xs font-medium rounded-lg transition-all cursor-pointer"
              :class="statusFilter === 'failed' ? 'bg-white dark:bg-[#2a2a2a] text-rose-600 dark:text-rose-400 shadow-sm' : 'text-gray-500 hover:text-gray-700 dark:hover:text-gray-300'"
            >
              Failed ({{ diagnosticsData?.summary.failed_runs ?? 0 }})
            </button>
          </div>
        </div>

        <!-- History Table -->
        <div class="overflow-x-auto">
          <table class="w-full text-left text-xs">
            <thead class="bg-gray-50/70 dark:bg-white/[0.02] text-gray-500 dark:text-gray-400 uppercase tracking-wider font-semibold border-b border-gray-100 dark:border-gray-800">
              <tr>
                <th class="px-5 py-3">Timestamp</th>
                <th class="px-5 py-3">Article</th>
                <th class="px-5 py-3">Model</th>
                <th class="px-5 py-3">Duration</th>
                <th class="px-5 py-3">Tokens (Prompt / Comp)</th>
                <th class="px-5 py-3 text-right">Status</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-gray-800/60">
              <tr v-if="filteredRuns.length === 0">
                <td colspan="6" class="px-5 py-12 text-center text-gray-400 dark:text-gray-500">
                  <svg class="mx-auto h-8 w-8 text-gray-300 dark:text-gray-600 mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                  </svg>
                  No pipeline execution records found for this filter.
                </td>
              </tr>
              <template v-for="run in filteredRuns" :key="run.id">
                <tr 
                  @click="run.error_message ? toggleExpandRun(run.id) : null"
                  class="hover:bg-gray-50/80 dark:hover:bg-white/[0.02] transition-colors"
                  :class="{ 'cursor-pointer': run.error_message }"
                >
                  <!-- Time -->
                  <td class="px-5 py-3.5 whitespace-nowrap text-gray-500 dark:text-gray-400 font-mono text-[11px]">
                    {{ formatDate(run.created_at) }}
                  </td>

                  <!-- Article Title -->
                  <td class="px-5 py-3.5 font-medium text-gray-900 dark:text-gray-100 max-w-sm truncate">
                    <div class="flex items-center gap-1.5 truncate">
                      <span v-if="run.article_title?.startsWith('[Librarian]')" class="px-1.5 py-0.5 rounded text-[10px] font-bold bg-amber-100 text-amber-800 dark:bg-amber-950/60 dark:text-amber-300 shrink-0">
                        Librarian
                      </span>
                      <span class="truncate">{{ run.article_title ? run.article_title.replace('[Librarian] ', '') : ('Article #' + run.article_id) }}</span>
                      <span v-if="run.error_message" class="ml-2 text-[10px] font-normal text-rose-500 dark:text-rose-400 underline shrink-0">
                        (click to view error)
                      </span>
                    </div>
                  </td>

                  <!-- Model -->
                  <td class="px-5 py-3.5 whitespace-nowrap font-mono text-[11px] text-gray-600 dark:text-gray-400">
                    {{ run.model ? run.model.split('/').pop() : 'default' }}
                  </td>

                  <!-- Duration & Retries -->
                  <td class="px-5 py-3.5 whitespace-nowrap font-mono text-gray-900 dark:text-gray-100">
                    <span>{{ formatMs(run.duration_ms) }}</span>
                    <span 
                      v-if="run.retry_count > 0"
                      class="ml-2 px-1.5 py-0.5 rounded text-[10px] font-bold bg-amber-100 text-amber-800 dark:bg-amber-950/60 dark:text-amber-300"
                      :title="`${run.retry_count} ${run.retry_count === 1 ? 'retry' : 'retries'}`"
                    >
                      {{ run.retry_count }}r
                    </span>
                  </td>

                  <!-- Tokens (Prompt / Comp) -->
                  <td class="px-5 py-3.5 whitespace-nowrap font-mono">
                    <span class="text-gray-900 dark:text-gray-100 font-semibold">{{ formatTokens(run.prompt_tokens + run.completion_tokens) }}</span>
                    <span class="ml-1.5 text-gray-400 dark:text-gray-500 text-[11px]">
                      ({{ formatTokens(run.prompt_tokens) }} / {{ formatTokens(run.completion_tokens) }})
                    </span>
                  </td>

                  <!-- Status Badge -->
                  <td class="px-5 py-3.5 whitespace-nowrap text-right">
                    <span 
                      v-if="run.status === 'success'"
                      class="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-50 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-400 border border-emerald-200 dark:border-emerald-800"
                    >
                      <span class="w-1 h-1 rounded-full bg-emerald-500"></span>
                      Success
                    </span>
                    <span 
                      v-else-if="run.status === 'failed'"
                      class="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-[10px] font-semibold bg-rose-50 text-rose-700 dark:bg-rose-950/50 dark:text-rose-400 border border-rose-200 dark:border-rose-800"
                    >
                      <span class="w-1 h-1 rounded-full bg-rose-500"></span>
                      Failed
                    </span>
                    <span 
                      v-else
                      class="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-[10px] font-semibold bg-blue-50 text-blue-700 dark:bg-blue-950/50 dark:text-blue-400 border border-blue-200 dark:border-blue-800"
                    >
                      <span class="w-1 h-1 rounded-full bg-blue-500 animate-ping"></span>
                      Running
                    </span>
                  </td>
                </tr>

                <!-- Expanded Error Row -->
                <tr v-if="expandedRunId === run.id && run.error_message" class="bg-rose-50/50 dark:bg-rose-950/20">
                  <td colspan="6" class="px-5 py-3">
                    <div class="space-y-1">
                      <div class="text-[11px] font-semibold text-rose-700 dark:text-rose-400 flex items-center gap-1">
                        <svg class="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                          <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7 4a1 1 0 11-2 0 1 1 0 012 0zm-1-9a1 1 0 00-1 1v4a1 1 0 102 0V6a1 1 0 00-1-1z" clip-rule="evenodd" />
                        </svg>
                        Execution Error Details:
                      </div>
                      <pre class="text-[11px] font-mono text-rose-900 dark:text-rose-200 bg-rose-100/60 dark:bg-rose-950/50 p-2.5 rounded-lg overflow-x-auto whitespace-pre-wrap">{{ run.error_message }}</pre>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
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
