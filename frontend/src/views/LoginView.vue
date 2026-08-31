<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { authState, checkAuthStatus, login, setupMasterPassword } from '../store/auth'
import { initSettings } from '../store/settings'
import { Lock, KeyRound, Eye, EyeOff, ArrowRight, ShieldCheck, AlertCircle } from 'lucide-vue-next'

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
  <div class="min-h-[85vh] flex items-center justify-center px-4">
    <div class="w-full max-w-md bg-white dark:bg-[#111] rounded-3xl border border-gray-200/80 dark:border-gray-800/80 shadow-[0_8px_30px_rgb(0,0,0,0.06)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.3)] p-8 sm:p-10 transition-all duration-300">
      
      <!-- Brand Icon & Title -->
      <div class="text-center mb-8">
        <div class="inline-flex items-center justify-center w-14 h-14 rounded-2xl bg-emerald-50 dark:bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 mb-4 ring-1 ring-emerald-500/20">
          <ShieldCheck v-if="isSetup" class="w-7 h-7" />
          <Lock v-else class="w-7 h-7" />
        </div>
        <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100">
          {{ isSetup ? 'Welcome to Readr — Set Master Password' : 'Unlock Readr Vault' }}
        </h1>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
          {{ isSetup 
            ? 'Create a master password to protect your articles, notes, and graph.' 
            : 'Enter your master password to access your knowledge vault.' 
          }}
        </p>
      </div>

      <!-- Error Alert -->
      <div v-if="errorMessage" class="mb-6 p-4 rounded-xl bg-red-50 dark:bg-red-500/10 border border-red-200 dark:border-red-500/20 flex items-start gap-3 text-red-600 dark:text-red-400 text-sm">
        <AlertCircle class="w-5 h-5 flex-shrink-0 mt-0.5" />
        <span>{{ errorMessage }}</span>
      </div>

      <!-- Form -->
      <form @submit.prevent="handleSubmit" class="space-y-5">
        <div>
          <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-2">
            {{ isSetup ? 'New Master Password' : 'Password' }}
          </label>
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-gray-400">
              <KeyRound class="w-4 h-4" />
            </div>
            <input
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              placeholder="••••••••••••"
              class="w-full pl-10 pr-10 py-3 text-sm rounded-xl border border-gray-200 dark:border-gray-800 bg-gray-50/50 dark:bg-black/30 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-emerald-500/40 focus:border-emerald-500 transition-all"
              required
              autofocus
            />
            <button
              type="button"
              @click="showPassword = !showPassword"
              class="absolute inset-y-0 right-0 pr-3.5 flex items-center text-gray-400 hover:text-gray-600 dark:hover:text-gray-200 focus:outline-none"
            >
              <EyeOff v-if="showPassword" class="w-4 h-4" />
              <Eye v-else class="w-4 h-4" />
            </button>
          </div>
        </div>

        <div v-if="isSetup">
          <label class="block text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400 mb-2">
            Confirm Password
          </label>
          <div class="relative">
            <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-gray-400">
              <KeyRound class="w-4 h-4" />
            </div>
            <input
              v-model="confirmPassword"
              :type="showPassword ? 'text' : 'password'"
              placeholder="••••••••••••"
              class="w-full pl-10 pr-4 py-3 text-sm rounded-xl border border-gray-200 dark:border-gray-800 bg-gray-50/50 dark:bg-black/30 text-gray-900 dark:text-white placeholder-gray-400 focus:outline-none focus:ring-2 focus:ring-emerald-500/40 focus:border-emerald-500 transition-all"
              required
            />
          </div>
        </div>

        <button
          type="submit"
          :disabled="isLoading"
          class="w-full mt-2 py-3 px-4 rounded-xl bg-emerald-600 hover:bg-emerald-500 active:bg-emerald-700 text-white font-medium text-sm flex items-center justify-center gap-2 shadow-sm transition-all focus:outline-none focus:ring-2 focus:ring-emerald-500/40 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <span v-if="isLoading">Verifying...</span>
          <template v-else>
            <span>{{ isSetup ? 'Set Password & Unlock' : 'Unlock Vault' }}</span>
            <ArrowRight class="w-4 h-4" />
          </template>
        </button>
      </form>

    </div>
  </div>
</template>
