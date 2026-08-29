<script setup lang="ts">
import { ref, onMounted, nextTick, onBeforeUnmount, watch, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import axios from 'axios'
import { Network } from 'vis-network'

interface ArticleData {
  ID: number;
  title: string;
  image: string;
  article: string;
  tags: string;
}

const markdownContent = ref('')
const route = useRoute()
const router = useRouter()
const articleError = ref('')

const localGraphContainer = ref<HTMLElement | null>(null)
let localNetwork: Network | null = null
let observer: MutationObserver | null = null

// Linker state
const showLinker = ref(false)
const linkerPos = ref({ top: 0, left: 0 })
const selectedText = ref('')
const searchInput = ref('')
const allArticles = ref<ArticleData[]>([])

const filteredArticles = computed(() => {
  const query = searchInput.value.toLowerCase()
  const currentId = Number(route.params.id)
  return allArticles.value.filter(a => 
    a.title.toLowerCase().includes(query) && a.ID !== currentId
  )
})

const fetchArticles = async () => {
  try {
    const res = await axios.get('/api/getarticles')
    allArticles.value = res.data
  } catch (err) {
    console.error('Failed to fetch articles', err)
    articleError.value = 'Failed to fetch articles'
  }
}

const handleSelection = () => {
  const selection = window.getSelection()
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
    return
  }
  
  const text = selection.toString().trim()
  if (!text) return

  const range = selection.getRangeAt(0)
  const rect = range.getBoundingClientRect()
  
  selectedText.value = text
  linkerPos.value = {
    top: rect.top + window.scrollY - 40,
    left: rect.left + window.scrollX + (rect.width / 2)
  }
  showLinker.value = true
}

const hideLinker = (e: Event) => {
  const target = e.target as HTMLElement
  if (!target.closest('.linker-popup')) {
    showLinker.value = false
  }
}

const createLink = async (targetId: number) => {
  try {
    await axios.post('/api/link', {
      sourceId: Number(route.params.id),
      targetId: targetId,
      selectedText: selectedText.value
    })
    
    showLinker.value = false
    loadContent(true)
  } catch (err) {
    console.error('Link creation failed', err)
    articleError.value = 'Failed to create link'
    showLinker.value = false
    alert('Failed to create link')
  }
}

const loadLocalGraph = async () => {
  if (!localGraphContainer.value) return
  
  try {
    const res = await fetch('/api/graph')
    const graphData = await res.json()
    
    // Filter for local neighborhood (1st degree)
    const currentArticleNodeId = `article-${route.params.id}`
    
    const connectedEdges = graphData.edges.filter(
      (e: any) => e.from === currentArticleNodeId || e.to === currentArticleNodeId
    )
    
    const connectedNodeIds = new Set<string>([currentArticleNodeId])
    connectedEdges.forEach((e: any) => {
      connectedNodeIds.add(e.from)
      connectedNodeIds.add(e.to)
    })
    
    const localNodes = graphData.nodes.filter((n: any) => connectedNodeIds.has(n.id))
    
    const isDark = document.documentElement.classList.contains('dark')
    
    const options = {
      nodes: { shape: 'dot', size: 10, font: { color: isDark ? '#fff' : '#000', size: 10 } },
      groups: {
        article: { color: '#10b981' },
        tag: { color: '#6366f1', shape: 'square' }
      },
      edges: { color: isDark ? '#333' : '#e2e8f0', width: 1 },
      physics: { barnesHut: { gravitationalConstant: -1000, springLength: 100 } }
    }

    if (localNetwork) {
      localNetwork.setData({ nodes: localNodes, edges: connectedEdges })
      localNetwork.setOptions(options)
    } else {
      localNetwork = new Network(localGraphContainer.value, { nodes: localNodes, edges: connectedEdges }, options)
      
      // Make nodes clickable to navigate!
      localNetwork.on('click', (params) => {
        if (params.nodes.length > 0) {
          const nodeId = params.nodes[0] as string
          if (nodeId.startsWith('article-')) {
            const id = nodeId.replace('article-', '')
            // NOTE: Article.vue already has navigation handling, make sure this integrates smoothly.
            router.push(`/articles/${id}`)
          }
        }
      })
    }
  } catch (err) {
    console.error("Failed to load local graph", err)
  }
}

const loadContent = async (forceRefresh = false) => {
  try {
    articleError.value = ''
    const articleID = route.params.id
    const articleURL = `/articles/${articleID}`
    const fetchUrl = forceRefresh ? `${articleURL}?t=${Date.now()}` : articleURL

    const res = await fetch(fetchUrl)
    if (!res.ok) throw new Error('Failed to fetch content')
    const raw = await res.text()
    
    // Custom Wikilink pre-processor
    const parsedRaw = raw.replace(/\[\[(.*?)\]\]/g, (_, p1) => {
      const parts = p1.split('|')
      const targetTitle = parts[0]
      const display = parts.length > 1 ? parts[1] : parts[0]
      
      const targetArticle = allArticles.value.find(a => a.title === targetTitle)
      const url = targetArticle ? `/articles/${targetArticle.ID}` : '#'
      
      return `<a href="${url}" class="wikilink font-medium text-emerald-600 dark:text-emerald-400 no-underline hover:underline px-1 bg-emerald-50 dark:bg-emerald-900/30 rounded">${display}</a>`
    })

    markdownContent.value = await marked.parse(parsedRaw, {
      gfm: false,
      async: true
    })

    await nextTick()
    document.querySelectorAll('pre code').forEach((block) => {
      hljs.highlightElement(block as HTMLElement)
    })
  } catch (err) {
    console.error('Failed to load content', err)
    articleError.value = 'Failed to load article content'
  }
}

watch(() => route.params.id, () => {
  loadContent()
  loadLocalGraph()
})

onMounted(async () => {
  await fetchArticles()
  await loadContent()
  await loadLocalGraph()
  document.addEventListener('mouseup', handleSelection)
  document.addEventListener('mousedown', hideLinker)
  
  observer = new MutationObserver(() => {
    if (localNetwork) {
      const isDark = document.documentElement.classList.contains('dark')
      localNetwork.setOptions({
        nodes: { font: { color: isDark ? '#fff' : '#000' } },
        edges: { color: isDark ? '#333' : '#e2e8f0' }
      })
    }
  })
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})

onBeforeUnmount(() => {
  document.removeEventListener('mouseup', handleSelection)
  document.removeEventListener('mousedown', hideLinker)
  if (localNetwork) {
    localNetwork.destroy()
    localNetwork = null
  }
  if (observer) {
    observer.disconnect()
    observer = null
  }
})
</script>

<template>
  <div v-if="articleError" class="bg-red-100 dark:bg-red-900 text-red-700 dark:text-red-100 p-4 text-center w-full">
    {{ articleError }}
  </div>
  <div class="flex flex-col lg:flex-row w-full max-w-7xl mx-auto items-start">
    <article class="w-full lg:w-2/3 max-w-2xl mx-auto py-10 transition-colors duration-300">
      <div class="prose prose-lg dark:prose-invert prose-p:text-gray-700 dark:prose-p:text-gray-300 prose-p:leading-relaxed prose-p:font-serif prose-headings:font-sans prose-headings:font-bold prose-headings:tracking-tight prose-headings:text-gray-900 dark:prose-headings:text-gray-100 prose-a:text-emerald-600 dark:prose-a:text-emerald-400 prose-a:no-underline hover:prose-a:underline prose-img:rounded-3xl prose-img:shadow-sm prose-pre:text-left prose-pre:bg-[#111] dark:prose-pre:bg-[#1a1a1a] prose-pre:rounded-2xl transition-colors duration-300" v-html="markdownContent" />
    </article>
    
    <!-- Local Graph Sidebar -->
    <aside class="hidden lg:block w-1/3 sticky top-24 h-[400px] bg-gray-50 dark:bg-[#1a1a1a] rounded-3xl border border-gray-100 dark:border-gray-800 ml-8 overflow-hidden mt-10">
      <div class="px-6 py-4 border-b border-gray-100 dark:border-gray-800 font-semibold text-gray-900 dark:text-gray-100 text-sm">Local Graph</div>
      <div ref="localGraphContainer" class="w-full h-[340px]"></div>
    </aside>

    <div 
      v-if="showLinker" 
      class="linker-popup absolute z-50 transform -translate-x-1/2 bg-white dark:bg-[#111] border border-gray-200 dark:border-gray-800 shadow-xl rounded-xl p-2 w-64"
      :style="{ top: linkerPos.top + 'px', left: linkerPos.left + 'px' }"
    >
      <input 
        v-model="searchInput" 
        placeholder="Link to article..." 
        class="w-full bg-gray-50 dark:bg-[#1a1a1a] text-sm px-3 py-2 rounded-lg border-transparent focus:ring-2 focus:ring-emerald-500 outline-none text-black dark:text-white mb-2"
      />
      <div class="max-h-40 overflow-y-auto space-y-1">
        <button 
          v-for="article in filteredArticles"
          :key="article.ID"
          @click="createLink(article.ID)"
          class="w-full text-left px-2 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg text-gray-700 dark:text-gray-300 truncate"
        >
          {{ article.title }}
        </button>
        <div v-if="filteredArticles.length === 0" class="text-xs text-gray-500 text-center py-2">
          No matching articles
        </div>
      </div>
    </div>
  </div>
</template>

