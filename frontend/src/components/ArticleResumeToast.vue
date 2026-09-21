<template>
  <transition name="toast-slide">
    <div
      v-if="show"
      role="status"
      class="fixed bottom-8 left-1/2 -translate-x-1/2 md:left-auto md:translate-x-0 md:right-8 z-50 max-w-[calc(100vw-2rem)] backdrop-blur-xl bg-white/95 dark:bg-[#161616]/95 border border-blue-500/30 shadow-[0_20px_50px_rgba(0,0,0,0.15)] dark:shadow-[0_20px_50px_rgba(0,0,0,0.7)] rounded-2xl pl-3 pr-2 py-2.5 flex items-center gap-2"
    >
      <ArticleStatusRing
        status="not_finished"
        :progress="percent"
        :size="18"
        :show-value="false"
      />
      <span class="text-xs font-mono text-gray-600 dark:text-gray-300 whitespace-nowrap">
        Resumed at {{ percent }}%
      </span>
      <button
        @click="emit('backToTop')"
        class="shrink-0 px-2 py-1 rounded-lg text-xs font-mono font-medium text-blue-600 dark:text-blue-400 hover:bg-blue-500/10 transition-colors cursor-pointer whitespace-nowrap"
      >
        Back to top
      </button>
      <div class="h-4 w-px shrink-0 bg-gray-200 dark:bg-white/10"></div>
      <button
        @click="emit('dismiss')"
        class="shrink-0 text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 p-1.5 rounded-lg hover:bg-gray-100 dark:hover:bg-white/5 transition-colors cursor-pointer"
        aria-label="Dismiss"
      >
        <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>
  </transition>
</template>

<script setup lang="ts">
import ArticleStatusRing from './ArticleStatusRing.vue'

defineProps<{
  show: boolean
  percent: number
}>()

const emit = defineEmits<{
  (e: 'backToTop'): void
  (e: 'dismiss'): void
}>()
</script>
