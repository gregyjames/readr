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
    size: 14,
    font: { color: isDark ? '#f8f8f8' : '#111', size: 13, face: 'Outfit' },
    widthConstraint: { maximum: 160 },
    borderWidth: 2,
    color: {
      border: isDark ? '#1a1a1a' : '#ffffff',
      background: '#10b981',
      hover: { border: isDark ? '#333' : '#e2e8f0', background: '#059669' }
    }
  },
  groups: {
    article: { 
      color: { background: '#10b981', border: isDark ? '#059669' : '#34d399' }
    },
    tag: { 
      shape: 'box', 
      color: { background: isDark ? '#1f2937' : '#f3f4f6', border: isDark ? '#374151' : '#e5e7eb' },
      font: { color: isDark ? '#d1d5db' : '#4b5563', size: 11 }
    }
  },
  edges: {
    color: isDark ? '#333333' : '#e2e8f0',
    width: 1.5,
    smooth: { type: 'continuous' }
  },
  physics: {
    barnesHut: { gravitationalConstant: -3000, centralGravity: 0.4, springLength: 120, damping: 0.15 }
  },
  interaction: {
    hover: true,
    tooltipDelay: 200
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

  network.once('stabilizationIterationsDone', () => {
    network?.fit({
      animation: { duration: 800, easingFunction: 'easeOutQuart' }
    });
  });

  network.on('click', (params) => {
    if (params.nodes.length > 0) {
      const nodeId = params.nodes[0] as string
      if (nodeId.startsWith('article-')) {
        const id = nodeId.replace('article-', '')
        router.push(`/articles/${id}`)
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
  <div class="relative w-full h-[100dvh] bg-[#f8f9fa] dark:bg-[#0a0a0a] overflow-hidden">
    <!-- Ambient subtle background glow -->
    <div class="absolute inset-0 bg-[radial-gradient(ellipse_at_center,_var(--tw-gradient-stops))] from-gray-200/20 via-transparent to-transparent dark:from-emerald-900/10 dark:via-transparent pointer-events-none"></div>

    <div ref="container" class="absolute inset-0 pt-16"></div> 

    <!-- Loading State -->
    <div v-if="isLoading" class="absolute inset-0 flex items-center justify-center pointer-events-none z-20">
      <div class="flex items-center space-x-3 text-gray-800 dark:text-gray-200 bg-white/60 dark:bg-[#1a1a1a]/60 px-6 py-3 rounded-2xl shadow-[0_8px_30px_rgb(0,0,0,0.04)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.6)] border border-white/50 dark:border-white/10 backdrop-blur-xl">
        <svg class="animate-spin h-5 w-5 text-emerald-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
        </svg>
        <span class="text-sm font-semibold tracking-wide">Loading graph...</span>
      </div>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="absolute inset-0 flex items-center justify-center p-4 z-20">
      <div class="bg-white/80 dark:bg-[#1a1a1a]/80 backdrop-blur-xl p-8 rounded-3xl border border-red-100 dark:border-red-900/30 shadow-[0_20px_60px_rgb(0,0,0,0.08)] dark:shadow-[0_20px_60px_rgb(0,0,0,0.8)] text-center max-w-sm">
        <p class="text-red-600 dark:text-red-400 font-bold mb-4 tracking-tight">{{ error }}</p>
        <button @click="loadGraph" class="px-5 py-2 bg-red-500 hover:bg-red-600 text-white text-sm font-bold rounded-xl transition-all cursor-pointer active:scale-95">
          Retry Connection
        </button>
      </div>
    </div>

    <!-- Tag Toggle Button -->
    <div v-if="!isLoading && !error" class="absolute bottom-10 right-10 z-10">
      <button @click="toggleTags" class="bg-white/70 dark:bg-black/50 backdrop-blur-xl px-5 py-2.5 text-gray-900 dark:text-gray-100 rounded-2xl shadow-[0_8px_30px_rgb(0,0,0,0.08)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.6)] border border-black/5 dark:border-white/10 text-sm font-bold hover:bg-white dark:hover:bg-[#1a1a1a] transition-all duration-300 cursor-pointer active:scale-90 flex items-center gap-2">
        <div :class="['w-2 h-2 rounded-full transition-colors', showTags ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-gray-600']"></div>
        {{ showTags ? 'Hide Tags' : 'Show Tags' }}
      </button>
    </div>
  </div>
</template>


