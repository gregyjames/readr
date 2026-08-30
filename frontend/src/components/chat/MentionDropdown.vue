<script setup lang="ts">
import type { Article } from '../../types/chat'

defineProps<{
  articles: Article[]
  selectedIndex: number
}>()

const emit = defineEmits<{
  (e: 'select', article: Article): void
}>()
</script>

<template>
  <div
    class="absolute bottom-full left-4 right-4 mb-2 bg-white dark:bg-[#1a1a1a] border border-gray-200 dark:border-gray-800 rounded-2xl shadow-xl overflow-hidden z-50 max-h-56 overflow-y-auto"
  >
    <div class="px-3 py-2 text-[11px] font-semibold text-gray-400 dark:text-gray-500 border-b border-gray-100 dark:border-gray-800 uppercase tracking-wider">
      Mention an article
    </div>
    <div
      v-for="(art, idx) in articles"
      :key="art.ID"
      @click="emit('select', art)"
      class="px-3 py-2.5 flex items-center gap-2.5 text-xs text-gray-800 dark:text-gray-200 hover:bg-emerald-50 dark:hover:bg-emerald-950/40 hover:text-emerald-700 dark:hover:text-emerald-300 transition-colors cursor-pointer"
      :class="selectedIndex === idx ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-700 dark:text-emerald-300 font-medium' : ''"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400 flex-shrink-0">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path>
        <polyline points="14 2 14 8 20 8"></polyline>
      </svg>
      <span class="truncate">{{ art.title }}</span>
    </div>
  </div>
</template>
