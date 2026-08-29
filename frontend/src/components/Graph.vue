<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { Network } from 'vis-network'

const container = ref<HTMLElement | null>(null)
const router = useRouter()
const showTags = ref(true)
const isLoading = ref(true)
const error = ref<string | null>(null)

let network: Network | null = null
let themeObserver: MutationObserver | null = null
let graphData: { nodes: any[]; edges: any[] } = { nodes: [], edges: [] }

const getOptions = (isDark: boolean) => ({
  nodes: {
    shape: 'dot',
    size: 16,
    font: { color: isDark ? '#fff' : '#000', size: 14 }
  },
  groups: {
    article: { color: '#10b981' },
    tag: { color: '#6366f1', shape: 'square' }
  },
  edges: {
    color: isDark ? '#333' : '#e2e8f0',
    width: 1
  },
  physics: {
    barnesHut: { gravitationalConstant: -2000, centralGravity: 0.3, springLength: 150 }
  }
})

const getFilteredData = () => {
  const filteredNodes = showTags.value 
    ? graphData.nodes 
    : graphData.nodes.filter((n: any) => n.group === 'article')

  const filteredEdges = showTags.value
    ? graphData.edges
    : graphData.edges.filter((e: any) => !e.to.startsWith('tag-'))

  return { nodes: filteredNodes, edges: filteredEdges }
}

const renderGraph = () => {
  if (!container.value) return

  const data = getFilteredData()
  const isDark = document.documentElement.classList.contains('dark')
  const options = getOptions(isDark)

  if (network) {
    network.setData(data)
    network.setOptions(options)
    return
  }

  network = new Network(container.value, data, options)

  network.on('click', (params) => {
    if (params.nodes.length > 0) {
      const nodeId = params.nodes[0] as string
      if (nodeId.startsWith('article-')) {
        const id = nodeId.replace('article-', '')
        router.push(`/${id}`)
      }
    }
  })
}

const loadGraph = async () => {
  isLoading.value = true
  error.value = null
  try {
    const res = await fetch('/api/graph')
    if (!res.ok) {
      throw new Error(`Failed to load graph data (${res.status})`)
    }
    graphData = await res.json()
    renderGraph()
  } catch (err: any) {
    console.error('Failed to load graph:', err)
    error.value = err?.message || 'Failed to load graph data'
  } finally {
    isLoading.value = false
  }
}

const toggleTags = () => {
  showTags.value = !showTags.value
  renderGraph()
}

onMounted(() => {
  loadGraph()

  themeObserver = new MutationObserver(() => {
    if (network) {
      const isDark = document.documentElement.classList.contains('dark')
      network.setOptions(getOptions(isDark))
    }
  })

  themeObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class']
  })
})

onUnmounted(() => {
  if (network) {
    network.destroy()
    network = null
  }
  if (themeObserver) {
    themeObserver.disconnect()
    themeObserver = null
  }
})
</script>

<template>
  <div class="relative w-full h-screen bg-gray-50 dark:bg-[#0a0a0a] overflow-hidden">
    <div ref="container" class="absolute inset-0 pt-16"></div> 

    <!-- Loading State -->
    <div v-if="isLoading" class="absolute inset-0 flex items-center justify-center pointer-events-none z-20">
      <div class="flex items-center space-x-2 text-gray-500 dark:text-gray-400 bg-white/80 dark:bg-black/80 px-4 py-2 rounded-full shadow border border-gray-200 dark:border-gray-800 backdrop-blur-sm">
        <svg class="animate-spin h-5 w-5 text-emerald-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
        </svg>
        <span class="text-sm font-medium">Loading graph...</span>
      </div>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="absolute inset-0 flex items-center justify-center p-4 z-20">
      <div class="bg-white dark:bg-[#111] p-6 rounded-2xl border border-red-200 dark:border-red-900/50 shadow-lg text-center max-w-sm">
        <p class="text-red-600 dark:text-red-400 font-medium mb-3">{{ error }}</p>
        <button @click="loadGraph" class="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-700 text-white text-sm font-medium rounded-lg transition-colors cursor-pointer">
          Retry
        </button>
      </div>
    </div>

    <!-- Tag Toggle Button -->
    <div v-if="!isLoading && !error" class="absolute top-24 right-8 z-10">
      <button @click="toggleTags" class="bg-white dark:bg-[#111] px-4 py-2 text-black dark:text-white rounded-full shadow border border-gray-200 dark:border-gray-800 text-sm font-medium hover:bg-gray-50 dark:hover:bg-[#1a1a1a] transition-colors cursor-pointer">
        {{ showTags ? 'Hide Tags' : 'Show Tags' }}
      </button>
    </div>
  </div>
</template>


