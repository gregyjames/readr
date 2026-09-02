<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { authState, checkAuthStatus, login, setupMasterPassword } from '../store/auth'
import { initSettings } from '../store/settings'
import { Lock, Eye, EyeOff, ArrowRight, ShieldCheck, AlertCircle } from 'lucide-vue-next'

const router = useRouter()
const password = ref('')
const confirmPassword = ref('')
const showPassword = ref(false)
const isLoading = ref(false)
const errorMessage = ref('')

onMounted(async () => {
  if (!authState.isLoaded) {
    await checkAuthStatus()
  }
  if (authState.isAuthenticated) {
    await initSettings()
    router.replace('/')
  }
})

const isSetup = computed(() => !authState.isAuthConfigured)

const handleSubmit = async () => {
  errorMessage.value = ''
  
  if (!password.value) {
    errorMessage.value = 'Please enter a password'
    return
  }

  if (isSetup.value) {
    if (password.value.length < 6) {
      errorMessage.value = 'Password must be at least 6 characters'
      return
    }
    if (password.value !== confirmPassword.value) {
      errorMessage.value = 'Passwords do not match'
      return
    }

    isLoading.value = true
    const result = await setupMasterPassword(password.value)
    isLoading.value = false

    if (result.success) {
      await initSettings()
      router.replace('/')
    } else {
      errorMessage.value = result.error || 'Failed to setup master password'
    }
  } else {
    isLoading.value = true
    const result = await login(password.value)
    isLoading.value = false

    if (result.success) {
      await initSettings()
      router.replace('/')
    } else {
      errorMessage.value = result.error || 'Invalid password'
    }
  }
}
</script>

<template>
  <div class="min-h-[80vh] flex items-center justify-center px-4 py-12">
    <div class="w-full max-w-sm">
      
      <!-- Minimal Brand Header -->
      <div class="text-center mb-8">
        <div class="inline-flex items-center justify-center w-10 h-10 rounded-xl bg-gray-900 dark:bg-white text-white dark:text-gray-950 mb-4 shadow-xs">
          <ShieldCheck v-if="isSetup" class="w-5 h-5" />
          <Lock v-else class="w-5 h-5" />
        </div>
        <h1 class="text-xl font-semibold tracking-tight text-gray-900 dark:text-gray-100">
          {{ isSetup ? 'Set Master Password' : 'Readr Vault' }}
        </h1>
        <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400 leading-relaxed">
          {{ isSetup 
            ? 'Set a master password to secure your local articles and graph.' 
            : 'Enter master password to access your encrypted reading vault.' 
          }}
        </p>
      </div>

      <!-- Card -->
      <div class="bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] shadow-sm p-6 sm:p-7">
        
        <!-- Error Alert -->
        <div v-if="errorMessage" class="mb-5 p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/40 flex items-start gap-2.5 text-red-600 dark:text-red-400 text-xs">
          <AlertCircle class="w-4 h-4 flex-shrink-0 mt-0.5" />
          <span class="leading-relaxed">{{ errorMessage }}</span>
        </div>

        <!-- Form -->
        <form @submit.prevent="handleSubmit" class="space-y-4">
          <div>
            <label class="block text-[11px] font-medium tracking-wider uppercase text-gray-500 dark:text-gray-400 mb-1.5 font-mono">
              {{ isSetup ? 'New Master Password' : 'Password' }}
            </label>
            <div class="relative">
              <input
                v-model="password"
                :type="showPassword ? 'text' : 'password'"
                placeholder="••••••••••••"
                class="w-full pl-3.5 pr-10 py-2.5 text-sm rounded-lg border border-gray-200 dark:border-white/10 bg-gray-50/50 dark:bg-white/[0.02] text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-600 focus:outline-none focus:ring-2 focus:ring-emerald-500/20 focus:border-emerald-500 dark:focus:border-emerald-400 transition-all"
                required
                autofocus
              />
              <button
                type="button"
                @click="showPassword = !showPassword"
                class="absolute inset-y-0 right-0 pr-3 flex items-center text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 focus:outline-none cursor-pointer"
                tabindex="-1"
              >
                <EyeOff v-if="showPassword" class="w-4 h-4" />
                <Eye v-else class="w-4 h-4" />
              </button>
            </div>
          </div>

          <div v-if="isSetup">
            <label class="block text-[11px] font-medium tracking-wider uppercase text-gray-500 dark:text-gray-400 mb-1.5 font-mono">
              Confirm Password
            </label>
            <div class="relative">
              <input
                v-model="confirmPassword"
                :type="showPassword ? 'text' : 'password'"
                placeholder="••••••••••••"
                class="w-full pl-3.5 pr-4 py-2.5 text-sm rounded-lg border border-gray-200 dark:border-white/10 bg-gray-50/50 dark:bg-white/[0.02] text-gray-900 dark:text-white placeholder-gray-400 dark:placeholder-gray-600 focus:outline-none focus:ring-2 focus:ring-emerald-500/20 focus:border-emerald-500 dark:focus:border-emerald-400 transition-all"
                required
              />
            </div>
          </div>

          <button
            type="submit"
            :disabled="isLoading"
            class="w-full mt-2 py-2.5 px-4 rounded-lg bg-gray-950 hover:bg-black dark:bg-white dark:hover:bg-gray-100 text-white dark:text-gray-950 font-medium text-sm flex items-center justify-center gap-2 shadow-xs transition-all active:scale-[0.99] disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
          >
            <span v-if="isLoading">Authenticating...</span>
            <template v-else>
              <span>{{ isSetup ? 'Save & Unlock' : 'Unlock Vault' }}</span>
              <ArrowRight class="w-4 h-4" />
            </template>
          </button>
        </form>

      </div>

      <div class="mt-6 text-center text-[11px] text-gray-400 dark:text-gray-500 font-mono">
        Encrypted SQLite & Markdown Knowledge Base
      </div>

    </div>
  </div>
</template>
