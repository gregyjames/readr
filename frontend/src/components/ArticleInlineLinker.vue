<template>
  <div 
    v-if="show" 
    @mousedown.prevent
    class="linker-popup absolute z-50 transform -translate-x-1/2 bg-white/90 dark:bg-[#1a1a1a]/90 backdrop-blur-xl border border-gray-200/50 dark:border-white/10 shadow-[0_20px_60px_rgb(0,0,0,0.1)] dark:shadow-[0_20px_60px_rgb(0,0,0,0.8)] rounded-2xl p-2 w-72 transition-all duration-200 ease-out animate-in fade-in zoom-in-95"
    :style="{ top: pos.top + 'px', left: pos.left + 'px' }"
  >
    <input 
      ref="searchInputRef"
      v-model="searchQuery" 
      placeholder="Link to article..." 
      class="w-full bg-gray-100/50 dark:bg-black/50 text-sm px-4 py-2.5 rounded-xl border-transparent focus:ring-2 focus:ring-emerald-500 outline-none text-gray-900 dark:text-gray-100 mb-2 font-medium placeholder-gray-500 dark:placeholder-gray-500"
    />
    <div class="max-h-48 overflow-y-auto space-y-1 px-1 pb-1">
      <button 
        v-for="article in filteredArticles"
        :key="article.ID"
        @click="emit('link', article.ID)"
        class="w-full text-left px-3 py-2 text-sm hover:bg-gray-100 dark:hover:bg-white/5 rounded-xl text-gray-700 dark:text-gray-300 font-medium truncate transition-colors active:scale-[0.98]"
      >
        {{ article.title }}
      </button>
      <div v-if="filteredArticles.length === 0" class="text-xs text-gray-500 font-medium text-center py-4">
        No matching articles
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'

const props = defineProps<{
  show: boolean
  pos: { top: number; left: number }
  articles: Array<{ ID: number; title: string; [key: string]: any }>
  currentId?: string | number
}>()

const emit = defineEmits<{
  (e: 'link', articleId: number): void
}>()

const searchQuery = ref('')
const searchInputRef = ref<HTMLInputElement | null>(null)

watch(() => props.show, (isVisible) => {
  if (isVisible) {
    searchQuery.value = ''
    nextTick(() => {
      searchInputRef.value?.focus()
    })
  }
})

const filteredArticles = computed(() => {
	const query = searchQuery.value.toLowerCase().trim()
	const currentNum = Number(props.currentId)
	const available = props.articles.filter((a) => a.ID !== currentNum)
	if (!query) {
		return available.slice(0, 10)
	}
	return available.filter((a) => a.title.toLowerCase().includes(query))
})
</script>
