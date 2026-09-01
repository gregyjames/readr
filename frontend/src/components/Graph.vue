<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { Network } from 'vis-network'
import GraphZoomControls from './GraphZoomControls.vue'
import emitter from '../event-bus'

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
    size: 9,
    font: {
      color: isDark ? '#d1d5db' : '#374151',
      size: 11,
      face: 'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif',
      strokeWidth: 3,
      strokeColor: isDark ? '#0a0a0a' : '#f8f9fa',
      vadjust: 2
    },
    widthConstraint: { maximum: 120 },
    borderWidth: 1.5,
    color: {
      border: isDark ? '#059669' : '#34d399',
      background: '#10b981',
      hover: { border: '#10b981', background: '#059669' },
      highlight: { border: '#34d399', background: '#10b981' }
    }
  },
  groups: {
    article: {
      color: {
        background: '#10b981',
        border: isDark ? '#047857' : '#34d399',
        hover: { background: '#059669', border: '#10b981' }
      },
      font: {
        color: isDark ? '#e5e7eb' : '#374151',
        size: 10,
        strokeWidth: 3,
        strokeColor: isDark ? '#0a0a0a' : '#f8f9fa'
      }
    },
    moc: {
      shape: 'hexagon',
      color: {
        background: '#f59e0b',
        border: isDark ? '#d97706' : '#fbbf24',
        hover: { background: '#d97706', border: '#fbbf24' }
      },
      font: {
        color: isDark ? '#fef3c7' : '#92400e',
        size: 12,
        bold: 'bold',
        strokeWidth: 3,
        strokeColor: isDark ? '#0a0a0a' : '#f8f9fa'
      }
    },
    tag: {
      shape: 'box',
      margin: 5,
      color: {
        background: isDark ? 'rgba(31, 41, 55, 0.85)' : 'rgba(243, 244, 246, 0.9)',
        border: isDark ? '#374151' : '#e5e7eb',
        hover: { background: isDark ? '#374151' : '#e5e7eb', border: isDark ? '#4b5563' : '#d1d5db' }
      },
      font: {
        color: isDark ? '#9ca3af' : '#6b7280',
        size: 10,
        face: 'Inter, sans-serif'
      }
    }
  },
  edges: {
    color: {
      color: isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.08)',
      highlight: '#10b981',
      hover: '#10b981'
    },
    width: 1,
    hoverWidth: 1.5,
    selectionWidth: 1.5,
    smooth: false
  },
  layout: {
    improvedLayout: false
  },
  physics: {
    enabled: true,
    solver: 'repulsion',
    repulsion: {
      nodeDistance: 280,
      centralGravity: 0.003,
      springLength: 280,
      springConstant: 0.03,
      damping: 0.45
    },
    stabilization: {
      enabled: true,
      iterations: 100,
      updateInterval: 25,
      fit: true
    },
    minVelocity: 0.35,
    maxVelocity: 50
  },
  interaction: {
    dragNodes: true,
    dragView: true,
    hover: true,
    hoverConnectedEdges: true,
    selectConnectedEdges: true,
    hideEdgesOnDrag: true,
    tooltipDelay: 100,
    zoomView: false
  }
})

import { useGraphZoom } from '../composables/useGraphZoom'
const { zoomIn, zoomOut, fitGraph: fitView } = useGraphZoom(() => network)

const getFilteredData = () => {
  const isDark = typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
  const filteredNodes = showTags.value 
    ? graphData.nodes 
    : graphData.nodes.filter((n: any) => n.group === 'article' || n.group === 'moc')

  const filteredEdges = showTags.value
    ? graphData.edges
    : graphData.edges.filter((e: any) => !e.to.startsWith('tag-'))

  const mocs = filteredNodes.filter((n: any) => n.group === 'moc')
  const mocCount = Math.max(mocs.length, 1)
  const clusterRadius = 480

  const mocPositions = new Map<string, { x: number; y: number; angle: number }>()
  mocs.forEach((moc: any, index: number) => {
    const angle = (index / mocCount) * 2 * Math.PI
    mocPositions.set(moc.id, {
      x: Math.cos(angle) * clusterRadius,
      y: Math.sin(angle) * clusterRadius,
      angle
    })
  })

  // Map each article to its primary MOC parent
  const articleMocMap = new Map<string, string>()
  filteredEdges.forEach((e: any) => {
    if (mocPositions.has(e.from) && !mocPositions.has(e.to)) {
      articleMocMap.set(e.to, e.from)
    } else if (mocPositions.has(e.to) && !mocPositions.has(e.from)) {
      articleMocMap.set(e.from, e.to)
    }
  })

  const processedNodes = filteredNodes.map((n: any) => {
    const connections = filteredEdges.filter((e: any) => e.from === n.id || e.to === n.id).length
    
    // Scale node and label size dynamically based on connection degree
    let nodeSize = 7
    let fontSize = 10
    let displayLabel: string | undefined = undefined

    if (n.group === 'moc') {
      nodeSize = Math.max(16, Math.min(16 + connections * 1.8, 32))
      fontSize = Math.max(12, Math.min(12 + Math.floor(connections * 0.4), 15))
      displayLabel = n.label
    } else if (n.group === 'tag') {
      nodeSize = Math.max(6, Math.min(6 + connections * 0.8, 16))
      fontSize = Math.max(9, Math.min(9 + Math.floor(connections * 0.3), 12))
      displayLabel = n.label
    } else {
      // Articles: only show text on large connected nodes (5+ connections)
      nodeSize = Math.max(6, Math.min(6 + connections * 1.5, 24))
      fontSize = Math.max(10, Math.min(10 + Math.floor(connections * 0.35), 13))
      if (connections >= 5) {
        displayLabel = n.label.length > 28 ? n.label.slice(0, 26) + '…' : n.label
      } else {
        displayLabel = undefined
      }
    }
    
    const isMoc = n.group === 'moc'
    const mass = isMoc ? 8.0 : (n.group === 'tag' ? 2.0 : 0.8)
    
    // Seed initial position in MOC quadrant
    let initialX: number | undefined = undefined
    let initialY: number | undefined = undefined

    if (isMoc && mocPositions.has(n.id)) {
      const pos = mocPositions.get(n.id)!
      initialX = pos.x
      initialY = pos.y
    } else if (articleMocMap.has(n.id)) {
      const parentPos = mocPositions.get(articleMocMap.get(n.id)!)!
      const angle = parentPos.angle + (Math.random() - 0.5) * 1.4
      const dist = 55 + Math.random() * 55
      initialX = parentPos.x + Math.cos(angle) * dist
      initialY = parentPos.y + Math.sin(angle) * dist
    }
    
    return {
      ...n,
      label: displayLabel,
      size: nodeSize,
      mass: mass,
      x: initialX,
      y: initialY,
      font: {
        size: fontSize,
        color: n.group === 'moc' ? (isDark ? '#fef3c7' : '#92400e') : (n.group === 'tag' ? (isDark ? '#9ca3af' : '#6b7280') : (isDark ? '#e5e7eb' : '#374151')),
        strokeWidth: 2,
        strokeColor: isDark ? '#0a0a0a' : '#f8f9fa',
        bold: n.group === 'moc' || connections >= 5 ? 'bold' : undefined
      },
      title: `${n.label} (${connections} connection${connections === 1 ? '' : 's'})`
    }
  })

  const mocNodeIds = new Set(filteredNodes.filter((n: any) => n.group === 'moc').map((n: any) => n.id))

  const processedEdges = filteredEdges.map((e: any) => {
    const isMocEdge = mocNodeIds.has(e.from) || mocNodeIds.has(e.to)
    return {
      ...e,
      length: isMocEdge ? 75 : 320,
      springConstant: isMocEdge ? 0.045 : 0.015,
      color: isMocEdge
        ? { color: isDark ? 'rgba(245, 158, 11, 0.25)' : 'rgba(217, 119, 6, 0.2)', highlight: '#f59e0b', hover: '#f59e0b' }
        : { color: isDark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.07)', highlight: '#10b981', hover: '#10b981' }
    }
  })

  return { nodes: processedNodes, edges: processedEdges }
}

let hasDragged = false

const renderGraph = () => {
  if (!container.value) return
  const isDark = document.documentElement.classList.contains('dark')

  if (network) {
    network.destroy()
    network = null
  }

  network = new Network(container.value, getFilteredData(), getOptions(isDark))

  network.on('dragStart', () => {
    hasDragged = false
  })

  network.on('dragging', () => {
    hasDragged = true
  })

  network.on('dragEnd', () => {
    setTimeout(() => {
      hasDragged = false
    }, 100)
  })

  network.on('click', (params) => {
    if (hasDragged) return
    if (params.nodes.length > 0) {
      const nodeId = params.nodes[0] as string
      const node = graphData.nodes.find((n: any) => n.id === nodeId)
      
      if (node && (node.group === 'article' || node.group === 'moc')) {
        const articleId = node.id.replace('article-', '')
        router.push(`/articles/${articleId}`)
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
    error.value = 'Failed to load graph relations'
  } finally {
    isLoading.value = false
  }
}

const toggleTags = () => {
  showTags.value = !showTags.value
  if (network) {
    network.setData(getFilteredData())
  }
}

onMounted(() => {
  loadGraph()

  emitter.on('article-added', () => {
    loadGraph()
  })

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

    <!-- Bottom Action Controls: Zoom Buttons & Tag Toggle -->
    <div v-if="!isLoading && !error" class="absolute bottom-10 right-10 z-10 flex items-center gap-3">
      <!-- Zoom Controls Pill -->
      <GraphZoomControls @zoom-in="zoomIn" @zoom-out="zoomOut" @fit="fitView" />

      <!-- Tag Toggle Button -->
      <button @click="toggleTags" class="bg-white/70 dark:bg-black/50 backdrop-blur-xl px-5 py-2.5 text-gray-900 dark:text-gray-100 rounded-2xl shadow-[0_8px_30px_rgb(0,0,0,0.08)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.6)] border border-black/5 dark:border-white/10 text-sm font-bold hover:bg-white dark:hover:bg-[#1a1a1a] transition-all duration-300 cursor-pointer active:scale-95 flex items-center gap-2">
        <div :class="['w-2 h-2 rounded-full transition-colors', showTags ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-gray-600']"></div>
        {{ showTags ? 'Hide Tags' : 'Show Tags' }}
      </button>
    </div>
  </div>
</template>


