<template>
  <div class="flex flex-col bg-white dark:bg-[#0C0E14]">
    <div class="px-3.5 py-2 border-b border-black/[0.06] dark:border-white/[0.06] bg-gray-50/50 dark:bg-white/[0.01] flex items-center justify-between text-[10px] font-mono text-gray-400 uppercase tracking-wider font-semibold">
      <span>Linked Mentions</span>
      <span>{{ backlinks.length }}</span>
    </div>
    <div class="p-2 space-y-1">
      <div v-if="backlinks.length === 0" class="text-xs text-gray-400 font-mono py-4 text-center">
        No incoming backlinks
      </div>
      <router-link
        v-for="link in backlinks"
        :key="link.ID"
        :to="`/articles/${link.ID}`"
        :data-id="link.ID"
        class="backlink-row flex items-center justify-between px-2.5 py-1.5 rounded-lg text-xs text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 hover:bg-gray-100 dark:hover:bg-white/[0.04] transition-all group"
      >
        <span class="truncate pr-2 font-medium">{{ link.title }}</span>
        <span class="text-gray-400 text-[10px] group-hover:translate-x-0.5 transition-transform">&rarr;</span>
      </router-link>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  backlinks: Array<{
    ID: number
    title: string
    [key: string]: unknown
  }>
}>()
</script>
