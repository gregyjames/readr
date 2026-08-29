<template>
  <div class="bg-white/70 dark:bg-black/50 backdrop-blur-xl p-1 text-gray-900 dark:text-gray-100 rounded-2xl shadow-[0_8px_30px_rgb(0,0,0,0.08)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.6)] border border-black/5 dark:border-white/10 flex items-center gap-0.5">
    <button
      @click="$emit('zoom-in')"
      class="w-8 h-8 flex items-center justify-center rounded-xl hover:bg-white dark:hover:bg-white/10 text-gray-800 dark:text-gray-200 transition-colors cursor-pointer text-base font-bold select-none leading-none active:scale-90"
      title="Zoom In"
    >
      +
    </button>
    <button
      @click="$emit('zoom-out')"
      class="w-8 h-8 flex items-center justify-center rounded-xl hover:bg-white dark:hover:bg-white/10 text-gray-800 dark:text-gray-200 transition-colors cursor-pointer text-base font-bold select-none leading-none active:scale-90"
      title="Zoom Out"
    >
      −
    </button>
    <button
      @click="$emit('fit')"
      class="w-8 h-8 flex items-center justify-center rounded-xl hover:bg-white dark:hover:bg-white/10 text-gray-800 dark:text-gray-200 transition-colors cursor-pointer select-none active:scale-90"
      title="Fit View"
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="15 3 21 3 21 9"></polyline>
        <polyline points="9 21 3 21 3 15"></polyline>
        <line x1="21" y1="3" x2="14" y2="10"></line>
        <line x1="3" y1="21" x2="10" y2="14"></line>
      </svg>
    </button>
  </div>
</template>
<script setup lang="ts">
defineEmits(['zoom-in', 'zoom-out', 'fit'])
</script>
