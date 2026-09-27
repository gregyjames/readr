<template>
  <nav
    aria-label="Pagination Navigation"
    class="flex flex-col sm:flex-row items-center justify-between gap-4 px-4 py-3 border-t border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 text-xs text-zinc-600 dark:text-zinc-400"
  >
    <!-- Range summary -->
    <div>
      <span v-if="totalItems === 0">Showing 0 of 0 items</span>
      <span v-else>Showing {{ startItem }}–{{ endItem }} of {{ totalItems }} items</span>
    </div>

    <div class="flex flex-wrap items-center gap-4">
      <!-- Page size select dropdown -->
      <div class="flex items-center gap-2">
        <label for="page-size-select" class="text-xs text-zinc-500 dark:text-zinc-400">Per page:</label>
        <select
          id="page-size-select"
          :value="pageSize"
          class="text-xs rounded-md border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 text-zinc-800 dark:text-zinc-200 px-2.5 py-1.5 focus:outline-none focus:ring-1 focus:ring-emerald-500 transition-colors cursor-pointer"
          @change="onPageSizeChange"
        >
          <option v-for="option in pageSizeOptions" :key="option" :value="option">
            {{ option }}
          </option>
        </select>
      </div>

      <!-- Navigation buttons & visible pages -->
      <div class="flex items-center gap-1">
        <button
          type="button"
          aria-label="Previous page"
          :disabled="currentPage <= 1 || totalItems === 0"
          class="inline-flex items-center justify-center p-1.5 rounded-md border border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          @click="emit('update:page', currentPage - 1)"
        >
          <ChevronLeft class="w-4 h-4" />
        </button>

        <template v-for="(page, idx) in visiblePages" :key="idx">
          <button
            v-if="typeof page === 'number'"
            type="button"
            :aria-label="`Page ${page}`"
            :aria-current="page === currentPage ? 'page' : undefined"
            class="inline-flex items-center justify-center min-w-[32px] h-8 px-2 text-xs rounded-md transition-colors"
            :class="[
              page === currentPage
                ? 'bg-emerald-600 text-white font-semibold'
                : 'text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 border border-transparent'
            ]"
            @click="emit('update:page', page)"
          >
            {{ page }}
          </button>
          <span
            v-else
            class="inline-flex items-center justify-center min-w-[32px] h-8 text-xs text-zinc-400 dark:text-zinc-600 select-none"
          >
            ...
          </span>
        </template>

        <button
          type="button"
          aria-label="Next page"
          :disabled="currentPage >= totalPages || totalItems === 0"
          class="inline-flex items-center justify-center p-1.5 rounded-md border border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          @click="emit('update:page', currentPage + 1)"
        >
          <ChevronRight class="w-4 h-4" />
        </button>
      </div>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'

interface Props {
  currentPage: number
  totalPages: number
  totalItems: number
  pageSize: number
  pageSizeOptions?: number[]
}

const props = withDefaults(defineProps<Props>(), {
  pageSizeOptions: () => [10, 25, 50, 100],
})

const emit = defineEmits<{
  (e: 'update:page', page: number): void
  (e: 'update:pageSize', size: number): void
}>()

const startItem = computed(() => {
  if (props.totalItems === 0) return 0
  return (props.currentPage - 1) * props.pageSize + 1
})

const endItem = computed(() => {
  return Math.min(props.currentPage * props.pageSize, props.totalItems)
})

const visiblePages = computed<(number | string)[]>(() => {
  if (props.totalPages <= 1) {
    return props.totalPages === 1 ? [1] : []
  }
  if (props.totalPages <= 7) {
    const pages: number[] = []
    for (let i = 1; i <= props.totalPages; i++) {
      pages.push(i)
    }
    return pages
  }

  const pages: (number | string)[] = [1]

  if (props.currentPage <= 4) {
    for (let i = 2; i <= 5; i++) {
      pages.push(i)
    }
    pages.push('...')
    pages.push(props.totalPages)
  } else if (props.currentPage >= props.totalPages - 3) {
    pages.push('...')
    for (let i = props.totalPages - 4; i <= props.totalPages; i++) {
      pages.push(i)
    }
  } else {
    pages.push('...')
    pages.push(props.currentPage - 1, props.currentPage, props.currentPage + 1)
    pages.push('...')
    pages.push(props.totalPages)
  }

  return pages
})

function onPageSizeChange(event: Event) {
  const target = event.target as HTMLSelectElement
  emit('update:pageSize', Number(target.value))
}
</script>
