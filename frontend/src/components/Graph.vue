<script setup lang="ts">
import { ref, onMounted } from 'vue'

const container = ref<HTMLElement | null>(null)

onMounted(() => {
  if (container.value) {
    // placeholder
  }
})
</script>
<template>
  <div class="w-full h-screen bg-gray-50 dark:bg-[#0a0a0a] pt-20 overflow-hidden" ref="container">
    <div class="p-8 text-black dark:text-white">Graph View Loading...</div>
  </div>
</template>
