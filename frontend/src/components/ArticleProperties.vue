<template>
  <div class="p-3.5 space-y-3 bg-white dark:bg-[#0C0E14]">
    <div class="flex items-center justify-between text-[10px] font-mono text-gray-400 uppercase tracking-wider font-semibold">
      <span>Properties</span>
      <span v-if="properties.date" class="text-gray-400 font-normal">
        {{ formatDisplayDate(properties.date) }}
      </span>
    </div>

    <div class="grid grid-cols-2 gap-2 text-xs font-mono">
      <div class="p-2.5 rounded-xl bg-gray-50 dark:bg-white/[0.02] border border-black/[0.04] dark:border-white/[0.04]">
        <span class="text-[10px] text-gray-400 block mb-0.5">READ TIME</span>
        <span class="font-semibold text-gray-800 dark:text-gray-200">{{ readingTime }}</span>
      </div>
      <div class="p-2.5 rounded-xl bg-gray-50 dark:bg-white/[0.02] border border-black/[0.04] dark:border-white/[0.04]">
        <span class="text-[10px] text-gray-400 block mb-0.5">WORD COUNT</span>
        <span class="font-semibold text-gray-800 dark:text-gray-200">{{ wordCount }} words</span>
      </div>
    </div>

    <!-- Tags in Properties -->
    <div v-if="properties.tags && properties.tags.length > 0" class="pt-0.5 space-y-1.5">
      <span class="text-[10px] font-mono text-gray-400 block uppercase font-semibold">TAGS</span>
      <div class="flex flex-wrap gap-1.5">
        <span
          v-for="tag in properties.tags"
          :key="tag"
          class="text-[11px] font-mono px-2 py-0.5 rounded-md bg-gray-100/90 dark:bg-white/[0.05] text-gray-600 dark:text-gray-300 border border-black/[0.04] dark:border-white/[0.04]"
        >
          #{{ tag }}
        </span>
      </div>
    </div>

    <div v-if="properties.source" class="pt-0.5">
      <a
        :href="properties.source"
        target="_blank"
        rel="noopener noreferrer"
        class="w-full flex items-center justify-between px-3 py-2 rounded-xl text-xs font-mono bg-gray-50 hover:bg-emerald-500/10 dark:bg-white/[0.03] dark:hover:bg-emerald-500/10 border border-black/[0.04] dark:border-white/[0.04] text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 transition-all group"
      >
        <span class="truncate">Original Source</span>
        <span class="group-hover:translate-x-0.5 group-hover:-translate-y-0.5 transition-transform text-[11px]">&nearr;</span>
      </a>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  properties: {
    date?: string
    tags?: string[]
    source?: string
    [key: string]: unknown
  }
  readingTime: string
  wordCount: number
}>()

const formatDisplayDate = (d: string) => {
  try {
    const dt = new Date(d)
    if (isNaN(dt.getTime())) return d
    return dt.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
  } catch {
    return d
  }
}
</script>
