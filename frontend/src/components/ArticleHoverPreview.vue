<template>
  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="opacity-0 translate-y-2 scale-95"
    enter-to-class="opacity-100 translate-y-0 scale-100"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="opacity-100 translate-y-0 scale-100"
    leave-to-class="opacity-0 translate-y-1 scale-95"
  >
    <div
      v-if="show && article"
      class="preview-card absolute z-50 w-72 bg-white/95 dark:bg-[#1a1a1a]/95 backdrop-blur-xl border border-gray-200/60 dark:border-white/10 shadow-[0_20px_40px_rgb(0,0,0,0.12)] dark:shadow-[0_20px_40px_rgb(0,0,0,0.6)] rounded-2xl overflow-hidden pointer-events-auto transform -translate-x-1/2 -translate-y-full"
      :style="{ top: pos.top + 'px', left: pos.left + 'px', marginTop: '-12px' }"
      @mouseenter="$emit('mouseenter')"
      @mouseleave="$emit('mouseleave')"
    >
      <div v-if="article.image" class="w-full h-32 overflow-hidden bg-gray-100 dark:bg-gray-800">
        <img :src="article.image" alt="Cover" class="w-full h-full object-cover" />
      </div>
      <div class="p-4">
        <h4 class="font-bold text-gray-900 dark:text-gray-100 leading-tight line-clamp-2 mb-2">
          {{ article.title }}
        </h4>
        <div v-if="article.tags" class="flex flex-wrap gap-1.5 mt-2">
          <span 
            v-for="tag in (article.tags || '').split(',').slice(0,3)" 
            :key="tag" 
            class="px-2 py-0.5 text-[9px] font-bold uppercase tracking-widest bg-emerald-50 dark:bg-emerald-900/30 text-emerald-600 dark:text-emerald-400 rounded-md"
          >
            {{ tag.trim() }}
          </span>
        </div>
        <div v-else class="text-xs text-gray-400 dark:text-gray-500 font-medium">
          Click to read
        </div>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
defineProps<{
  show: boolean
  article: any
  pos: { top: number; left: number }
}>()

defineEmits(['mouseenter', 'mouseleave'])
</script>
