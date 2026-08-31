<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import axios from 'axios'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import { Network } from 'vis-network'
import ArticleHoverPreview from './ArticleHoverPreview.vue'
import GraphZoomControls from './GraphZoomControls.vue'
import { useGraphZoom } from '../composables/useGraphZoom'
import emitter from '../event-bus'

defineProps<{ id?: string }>()

const route = useRoute()

const previewArticle = ref<any>(null)
const previewPos = ref({ top: 0, left: 0 })
const showPreview = ref(false)
let previewTimeout: any = null

const getArticleId = () => String(route.params.id || '').replace('.md', '')
const router = useRouter()
const markdownContent = ref('')
const rawMarkdown = ref('')
const isEditing = ref(false)
const editContent = ref('')
const isSaving = ref(false)

interface PropertyItem {
  key: string
  label: string
  value: any
  type: 'title' | 'source' | 'tags' | 'date' | 'image' | 'text'
}

const properties = ref<PropertyItem[]>([])

function parseFrontmatter(raw: string): PropertyItem[] {
  const match = raw.match(/^---\r?\n([\s\S]*?)\r?\n---/)
  if (!match) return []
  const block = match[1]
  const lines = block.split(/\r?\n/)
  const result: PropertyItem[] = []

  for (const line of lines) {
    const colonIdx = line.indexOf(':')
    if (colonIdx === -1) continue
    const key = line.slice(0, colonIdx).trim()
    if (!key) continue
    let val = line.slice(colonIdx + 1).trim()
    
    // Strip surrounding quotes
    val = val.replace(/^["']|["']$/g, '')

    let type: PropertyItem['type'] = 'text'
    let parsedValue: any = val

    if (key.toLowerCase() === 'tags') {
      type = 'tags'
      parsedValue = val
        .replace(/^\[|\]$/g, '')
        .split(',')
        .map(t => t.trim().replace(/^["']|["']$/g, ''))
        .filter(Boolean)
    } else if (key.toLowerCase() === 'source' || /^https?:\/\//i.test(val)) {
      type = 'source'
    } else if (key.toLowerCase() === 'saved' || key.toLowerCase() === 'date' || /^\d{4}-\d{2}-\d{2}/.test(val)) {
      type = 'date'
    } else if (key.toLowerCase() === 'cover' || key.toLowerCase() === 'image') {
      type = 'image'
    } else if (key.toLowerCase() === 'title') {
      type = 'title'
    }

    result.push({
      key,
      label: key.charAt(0).toUpperCase() + key.slice(1),
      value: parsedValue,
      type
    })
  }

  return result
}

const knownProperties = computed(() => {
  const meta = {
    title: '',
    source: '',
    tags: [] as string[],
    date: '',
    cover: '',
    custom: [] as { key: string; label: string; value: any }[]
  }

  for (const p of properties.value) {
    if (p.type === 'title') meta.title = p.value
    else if (p.type === 'source') meta.source = p.value
    else if (p.type === 'tags') meta.tags = Array.isArray(p.value) ? p.value : [p.value]
    else if (p.type === 'date') meta.date = p.value
    else if (p.type === 'image') meta.cover = p.value
    else meta.custom.push({ key: p.key, label: p.label, value: p.value })
  }

  return meta
})

const currentArticle = computed(() => {
  const id = Number(getArticleId())
  return allArticles.value.find(a => a.ID === id)
})

const articleTitle = computed(() => {
  return knownProperties.value.title || currentArticle.value?.title || ''
})

function formatDisplayDate(dateStr: string): string {
  if (!dateStr) return ''
  try {
    const d = new Date(dateStr)
    if (isNaN(d.getTime())) return dateStr
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })
  } catch {
    return dateStr
  }
}

function getHostname(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}

type NotificationState = 'idle' | 'running' | 'completed' | 'error'
const notificationState = ref<NotificationState>('idle')
const notificationMessage = ref('')
let notificationTimer: any = null

const isReparsing = ref(false)

const dismissNotification = () => {
  if (notificationTimer) clearTimeout(notificationTimer)
  notificationState.value = 'idle'
  isReparsing.value = false
}

const handleReparseComplete = async () => {
  await fetchArticles()
  await loadContent()
  if (notificationState.value === 'running') {
    notificationState.value = 'completed'
    notificationMessage.value = 'Article successfully reparsed and graph updated!'
    isReparsing.value = false
    if (notificationTimer) clearTimeout(notificationTimer)
    notificationTimer = setTimeout(() => {
      notificationState.value = 'idle'
    }, 4000)
  }
}

import { settings } from '../store/settings'

const reparseArticle = async () => {
  const articleID = getArticleId()
  if (!articleID || isReparsing.value) return
  isReparsing.value = true
  notificationState.value = 'running'
  notificationMessage.value = 'Agents running in background: analyzing, enriching frontmatter, and linking...'
  if (notificationTimer) clearTimeout(notificationTimer)

  // Safety fallback: if SSE disconnects or takes too long, auto-resolve after 12s
  notificationTimer = setTimeout(async () => {
    if (notificationState.value === 'running') {
      await handleReparseComplete()
    }
  }, 12000)

  try {
    await axios.post(`/api/articles/${articleID}/reparse`)
  } catch (err: any) {
    console.error('Failed to trigger agents', err)
    isReparsing.value = false
    notificationState.value = 'error'
    notificationMessage.value = err?.response?.data?.error || 'Failed to trigger background agents'
    if (notificationTimer) clearTimeout(notificationTimer)
    notificationTimer = setTimeout(() => {
      notificationState.value = 'idle'
    }, 4000)
  }
}

const startEditing = () => {
  editContent.value = rawMarkdown.value
  isEditing.value = true
}

const cancelEditing = () => {
  isEditing.value = false
  editContent.value = ''
}

const saveEdit = async () => {
  isSaving.value = true
  try {
    await axios.post(`/api/edit/${getArticleId()}`, { content: editContent.value })
    isEditing.value = false
    await loadContent()
  } catch (err) {
    console.error('Failed to save', err)
    alert('Failed to save edit')
  } finally {
    isSaving.value = false
  }
}

const articleError = ref('')
const showLinker = ref(false)
const linkerPos = ref({ top: 0, left: 0 })
const selectedText = ref('')
const searchInput = ref('')
const allArticles = ref<ArticleData[]>([])

const filteredArticles = computed(() => {
  const query = searchInput.value.toLowerCase()
  const currentId = Number(getArticleId())
  return allArticles.value.filter(a => 
    a.title.toLowerCase().includes(query) && a.ID !== currentId
  )
})

interface ArticleData {
  ID: number
  title: string
  image: string
  article: string
  tags: string
}


const graphDataCache = ref<any>(null)

const backlinks = computed(() => {
  const currentId = Number(getArticleId())
  const currentArticleNodeId = `article-${currentId}`
  
  if (!graphDataCache.value) return []

  const incomingEdges = graphDataCache.value.edges.filter((e: any) => e.to === currentArticleNodeId)
  const incomingNodeIds = incomingEdges.map((e: any) => e.from)

  return allArticles.value.filter((a: any) => incomingNodeIds.includes(`article-${a.ID}`))
})

let observer: MutationObserver | null = null
const localGraphContainer = ref<HTMLElement | null>(null)
let localNetwork: Network | null = null
const { zoomIn: localZoomIn, zoomOut: localZoomOut, fitGraph: localFitView } = useGraphZoom(() => localNetwork)

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
    top: rect.top + window.scrollY - 50,
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
      sourceId: Number(getArticleId()),
      targetId: targetId,
      selectedText: selectedText.value
    })
    
    showLinker.value = false
    loadContent()
  } catch (err: any) {
    console.error('Link creation failed', err)
    const msg = err.response?.data?.error || 'Failed to create link'
    articleError.value = msg
    showLinker.value = false
    alert(msg)
  }
}

const loadLocalGraph = async () => {
  if (!localGraphContainer.value) return
  
  try {
    const res = await axios.get('/api/graph')
    const graphData = res.data
    graphDataCache.value = graphData
    
    if (!localGraphContainer.value) return
    
    const currentArticleNodeId = `article-${getArticleId()}`
    
    const connectedEdges = graphData.edges.filter(
      (e: any) => e.from === currentArticleNodeId || e.to === currentArticleNodeId
    )
    
    const connectedNodeIds = new Set<string>([currentArticleNodeId])
    connectedEdges.forEach((e: any) => {
      connectedNodeIds.add(e.from)
      connectedNodeIds.add(e.to)
    })
    
    const localNodes = graphData.nodes.filter((n: any) => connectedNodeIds.has(n.id)).map((n: any) => {
      const connections = connectedEdges.filter((e: any) => e.from === n.id || e.to === n.id).length
      const nodeSize = Math.min(10 + (connections * 3), 35)
      
      if (n.group === 'article') {
        return { ...n, title: n.label, label: undefined, size: nodeSize }
      }
      return { ...n, size: nodeSize }
    })
    
    const isDark = document.documentElement.classList.contains('dark')
    
    const options = {
      nodes: { 
        shape: 'dot', 
        size: 10, 
        font: { color: isDark ? '#fff' : '#000', size: 10, face: 'Outfit' },
        borderWidth: 2,
        color: { border: isDark ? '#1a1a1a' : '#fff', background: '#10b981' },
        widthConstraint: { maximum: 120 }
      },
      groups: {
        article: { color: { background: '#10b981', border: isDark ? '#059669' : '#34d399' } },
        tag: { 
          shape: 'box', 
          color: { background: isDark ? '#1f2937' : '#f3f4f6', border: isDark ? '#374151' : '#e5e7eb' },
          font: { color: isDark ? '#d1d5db' : '#4b5563', size: 9 }
        }
      },
      edges: { color: isDark ? '#333' : '#e2e8f0', width: 1 },
      physics: { barnesHut: { gravitationalConstant: -800, springLength: 80, damping: 0.2 } },
      interaction: { hover: true, tooltipDelay: 200, zoomView: false }
    }

    if (localNetwork) {
      localNetwork.setData({ nodes: localNodes, edges: connectedEdges })
      localNetwork.setOptions(options)
      localNetwork.once('afterDrawing', () => { localNetwork?.fit({ animation: { duration: 500, easingFunction: 'easeInOutQuad' } }); });
    } else {
      localNetwork = new Network(localGraphContainer.value, { nodes: localNodes, edges: connectedEdges }, options)
      
      localNetwork.once('stabilizationIterationsDone', () => {
        localNetwork?.fit({
          animation: { duration: 800, easingFunction: 'easeOutQuart' }
        });
      });
      
      localNetwork.on('click', (params) => {
        if (params.nodes.length > 0) {
          const nodeId = params.nodes[0] as string
          if (nodeId.startsWith('article-')) {
            const id = nodeId.replace('article-', '')
            router.push(`/articles/${id}`)
          }
        }
      })
    }
  } catch (err) {
    console.error("Failed to load local graph", err)
  }
}

const loadContent = async () => {
  try {
    articleError.value = ''
    const articleID = getArticleId()
    if (!articleID) return

    const articleURL = `/api/articles/${articleID}.md`
    // Always append cache buster to prevent stale browser cache after navigation
    const fetchUrl = `${articleURL}?t=${Date.now()}`

    const res = await axios.get(fetchUrl)
    const raw = String(res.data)
    rawMarkdown.value = raw
    properties.value = parseFrontmatter(raw)
    
    const parsedRaw = raw.replace(/\[\[([^[\]\n]+?)\]\]/g, (_, p1) => {
      const parts = p1.split('|')
      const targetTitle = parts[0].trim()
      const display = parts.length > 1 ? parts[1].trim() : targetTitle
      
      const targetArticle = allArticles.value.find(a => 
        a.title.trim().toLowerCase() === targetTitle.toLowerCase() ||
        String(a.ID) === targetTitle
      )

      if (targetArticle) {
        return `<a href="/articles/${targetArticle.ID}" data-article-id="${targetArticle.ID}" class="wikilink font-semibold text-emerald-600 dark:text-emerald-400 no-underline hover:underline hover:text-emerald-700 dark:hover:text-emerald-300 transition-colors bg-emerald-50/50 dark:bg-emerald-900/20 px-1.5 py-0.5 rounded-md border border-emerald-100 dark:border-emerald-800/50 cursor-pointer">${display}</a>`
      } else {
        return `<span class="font-semibold text-emerald-600/70 dark:text-emerald-400/70 bg-emerald-50/30 dark:bg-emerald-900/10 px-1.5 py-0.5 rounded-md border border-dashed border-emerald-200/50 dark:border-emerald-800/30" title="Target article not found in vault: ${targetTitle}">${display}</span>`
      }
    })

    // Strip YAML frontmatter (--- ... ---) so it never renders in the article view                                                                                 
    const strippedRaw = parsedRaw.replace(/^---[\s\S]*?---\n?/, '')  

    markdownContent.value = await marked.parse(strippedRaw, {
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

watch(() => route.params.id, (newId) => {
  if (newId) {
    showPreview.value = false
    loadContent()
    loadLocalGraph()
  }
})

const handleMouseOver = (e: MouseEvent) => {
  const target = (e.target as HTMLElement).closest('a.wikilink, .backlink-row')
  if (target) {
    let articleId = ''
    if (target.classList.contains('wikilink')) {
      articleId = target.getAttribute('data-article-id') || ''
    } else {
      articleId = target.getAttribute('data-id') || ''
    }

    if (articleId) {
      const article = allArticles.value.find(a => String(a.ID) === articleId)
      if (article) {
        clearTimeout(previewTimeout)
        previewArticle.value = article
        
        const rect = target.getBoundingClientRect()
        previewPos.value = {
          top: rect.top + window.scrollY,
          left: rect.left + window.scrollX + (rect.width / 2)
        }
        showPreview.value = true
      }
    }
  }
}

const handleMouseOut = (e: MouseEvent) => {
  if ((e.target as HTMLElement).closest('a.wikilink, .backlink-row')) {
    previewTimeout = setTimeout(() => {
      showPreview.value = false
    }, 200)
  }
}

const cancelPreviewHide = () => {
  clearTimeout(previewTimeout)
}

const hidePreview = () => {
  previewTimeout = setTimeout(() => {
    showPreview.value = false
  }, 200)
}

const handleWikilinkClick = (e: MouseEvent) => {
  const target = (e.target as HTMLElement).closest('a.wikilink, a') as HTMLAnchorElement | null
  if (target) {
    const articleId = target.getAttribute('data-article-id')
    const href = target.getAttribute('href')

    if (articleId) {
      e.preventDefault()
      e.stopPropagation()
      showPreview.value = false
      router.push(`/articles/${articleId}`)
    } else if (href && href.startsWith('/articles/')) {
      e.preventDefault()
      e.stopPropagation()
      showPreview.value = false
      router.push(href)
    }
  }
}

onMounted(async () => {
  await fetchArticles()
  await loadContent()
  await loadLocalGraph()
  
  emitter.on('article-added', handleReparseComplete)
  emitter.on('graph-updated', handleReparseComplete)

  document.addEventListener('mouseup', handleSelection)
  document.addEventListener('mousedown', hideLinker)
  document.addEventListener('click', handleWikilinkClick)
  document.addEventListener('mouseover', handleMouseOver)
  document.addEventListener('mouseout', handleMouseOut)
  
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
  if (notificationTimer) {
    clearTimeout(notificationTimer)
  }
  emitter.off('article-added', handleReparseComplete)
  emitter.off('graph-updated', handleReparseComplete)
  document.removeEventListener('mouseup', handleSelection)
  document.removeEventListener('mousedown', hideLinker)
  document.removeEventListener('click', handleWikilinkClick)
  document.removeEventListener('mouseover', handleMouseOver)
  document.removeEventListener('mouseout', handleMouseOut)
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
  <div v-if="articleError" class="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 p-4 text-center w-full font-medium border-b border-red-100 dark:border-red-900/30">
    {{ articleError }}
  </div>
  <div class="flex flex-col lg:flex-row w-full max-w-7xl mx-auto items-start relative px-4 lg:px-8">
    <article class="w-full lg:w-2/3 max-w-2xl mx-auto py-16 transition-colors duration-300">
      
      
      <!-- If Editing Markdown -->
      <div v-if="isEditing" class="mb-8 w-full animate-in fade-in slide-in-from-top-4 duration-300">
        <textarea v-model="editContent" rows="18" class="w-full p-6 bg-gray-50 dark:bg-[#111] border border-gray-200 dark:border-gray-800 rounded-2xl text-gray-900 dark:text-gray-100 font-mono text-sm leading-relaxed focus:ring-4 focus:ring-emerald-500/20 focus:border-emerald-500 outline-none resize-y transition-all shadow-inner"></textarea>
        <div class="flex justify-end gap-3 mt-4">
          <button @click="cancelEditing" :disabled="isSaving" class="px-5 py-2.5 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 font-bold text-sm transition-colors disabled:opacity-50">Cancel</button>
          <button @click="saveEdit" :disabled="isSaving" class="px-6 py-2.5 bg-emerald-500 hover:bg-emerald-600 text-white font-bold text-sm rounded-xl transition-colors active:scale-95 shadow-sm disabled:opacity-50 flex items-center gap-2">
            <svg v-if="isSaving" class="animate-spin h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path></svg>
            {{ isSaving ? 'Saving...' : 'Save Changes' }}
          </button>
        </div>
      </div>

      <div v-else>
        <!-- Editorial Provenance Masthead -->
        <header v-if="properties.length > 0" class="mb-10 pb-8 border-b border-gray-200/60 dark:border-white/10">
          <!-- Top Row: Source Publisher, Date, and Edit Button -->
          <div class="flex items-center justify-between gap-4">
            <div class="flex flex-wrap items-center gap-3">
              <!-- Source Badge -->
              <a
                v-if="knownProperties.source"
                :href="knownProperties.source"
                target="_blank"
                rel="noopener noreferrer"
                class="group inline-flex items-center gap-2 px-3 py-1 rounded-full bg-gray-100/80 dark:bg-white/5 hover:bg-emerald-50 dark:hover:bg-emerald-950/40 text-gray-700 dark:text-gray-300 hover:text-emerald-700 dark:hover:text-emerald-300 border border-gray-200/50 dark:border-white/5 text-xs font-medium tracking-tight transition-all duration-200 active:scale-95"
              >
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 group-hover:scale-125 transition-transform"></span>
                <span class="font-medium">{{ getHostname(knownProperties.source) }}</span>
                <svg xmlns="http://www.w3.org/2000/svg" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 transition-all"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path><polyline points="15 3 21 3 21 9"></polyline><line x1="10" y1="14" x2="21" y2="3"></line></svg>
              </a>

              <!-- Captured Date -->
              <div v-if="knownProperties.date" class="text-xs text-gray-400 dark:text-gray-500 font-medium flex items-center gap-1.5">
                <span class="hidden sm:inline">Saved</span>
                <span>{{ formatDisplayDate(knownProperties.date) }}</span>
              </div>
            </div>

            <div class="flex items-center gap-2">
              <button
                @click="reparseArticle"
                :disabled="isReparsing"
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100 dark:hover:bg-white/5 transition-all active:scale-95 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
                title="Re-run enabled agents (Enricher, Linker) for this article"
              >
                <svg v-if="isReparsing" class="animate-spin h-3 w-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path></svg>
                <svg v-else xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"></path><path d="M21 3v5h-5"></path></svg>
                <span>{{ isReparsing ? 'Running...' : 'Re-run Agents' }}</span>
              </button>
              <!-- Edit Control -->
              <button
                @click="startEditing"
                class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100 dark:hover:bg-white/5 transition-all active:scale-95 cursor-pointer"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg>
                <span>Edit</span>
              </button>
            </div>
          </div>

          <!-- Bottom Row: Topic tags & custom metadata -->
          <div v-if="knownProperties.tags.length > 0 || knownProperties.custom.length > 0" class="flex flex-wrap items-center gap-2 mt-4">
            <!-- Topic Chips -->
            <span
              v-for="tag in knownProperties.tags"
              :key="tag"
              class="inline-flex items-center text-[10px] font-bold uppercase tracking-wider px-2.5 py-1 rounded-md bg-gray-100/90 dark:bg-white/5 text-gray-700 dark:text-gray-300 border border-gray-200/50 dark:border-white/5"
            >
              {{ tag }}
            </span>

            <!-- Custom Attributes (if any) -->
            <div
              v-for="custom in knownProperties.custom"
              :key="custom.key"
              class="inline-flex items-center gap-1.5 text-xs px-2.5 py-0.5 rounded-md bg-gray-50 dark:bg-white/[0.02] border border-gray-200/40 dark:border-white/5 text-gray-600 dark:text-gray-400 font-mono"
            >
              <span class="text-[10px] text-gray-400 uppercase tracking-wider">{{ custom.label }}:</span>
              <span class="font-medium text-gray-800 dark:text-gray-200">{{ custom.value }}</span>
            </div>
          </div>
        </header>

        <!-- Standalone edit button for legacy articles without frontmatter -->
        <div v-else class="flex justify-end mb-6">
          <button @click="startEditing" class="px-4 py-2 bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded-lg text-sm font-bold hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors active:scale-95">Edit Markdown</button>
        </div>

        <!-- Article Main Title -->
        <h1 v-if="articleTitle" class="text-3xl sm:text-4xl md:text-5xl font-bold tracking-tight text-gray-900 dark:text-gray-50 mb-8 font-sans leading-[1.18]">
          {{ articleTitle }}
        </h1>

        <!-- Markdown Prose Body -->
        <div
          class="prose prose-lg md:prose-xl dark:prose-invert prose-p:text-gray-700 dark:prose-p:text-gray-300 prose-p:leading-relaxed prose-p:font-serif prose-headings:font-sans prose-headings:font-bold prose-headings:tracking-tight prose-headings:text-gray-900 dark:prose-headings:text-gray-50 prose-a:text-emerald-600 dark:prose-a:text-emerald-400 prose-a:no-underline hover:prose-a:underline prose-img:rounded-3xl prose-img:shadow-sm prose-pre:text-left prose-pre:bg-[#111] dark:prose-pre:bg-[#1a1a1a] prose-pre:rounded-[2rem] transition-colors duration-300"
          v-html="markdownContent"
        />
      </div>

      
      <div v-if="backlinks.length > 0" class="mt-24 pt-12 border-t border-gray-200/50 dark:border-white/10">
        <h3 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 mb-6 flex items-center gap-3">
          <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400 dark:text-gray-500"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>
          Linked Mentions
        </h3>
        
        <div class="overflow-hidden rounded-[2rem] border border-gray-200/50 dark:border-white/5 bg-white/50 dark:bg-[#121212]/50 backdrop-blur-3xl shadow-[0_8px_30px_rgb(0,0,0,0.04)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.4)]">
          <table class="w-full text-left border-collapse">
            <thead>
              <tr class="border-b border-gray-200/50 dark:border-white/5 bg-gray-50/50 dark:bg-black/20 text-sm font-semibold text-gray-500 dark:text-gray-400 tracking-wide uppercase">
                <th class="px-8 py-5 font-bold">Article</th>
                <th class="px-8 py-5 font-bold text-right hidden sm:table-cell">Tags</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="link in backlinks" :key="link.ID" :data-id="link.ID" class="backlink-row group border-b last:border-0 border-gray-100 dark:border-white/5 hover:bg-gray-50/80 dark:hover:bg-white/5 transition-all duration-300 cursor-pointer active:scale-[0.99]" @click="router.push(`/articles/${link.ID}`)">
                <td class="px-8 py-5">
                  <span class="text-lg font-bold text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors">{{ link.title }}</span>
                </td>
                <td class="px-8 py-5 text-right hidden sm:table-cell">
                  <div class="flex justify-end gap-2">
                     <span v-for="tag in (link.tags ? link.tags.split(',') : []).slice(0,3)" :key="tag" class="px-2.5 py-1 text-[10px] font-bold uppercase tracking-widest bg-gray-100/80 dark:bg-black/60 text-gray-600 dark:text-gray-400 rounded-lg group-hover:bg-emerald-50 dark:group-hover:bg-emerald-900/30 group-hover:text-emerald-700 dark:group-hover:text-emerald-300 transition-colors">
                       {{ tag.trim() }}
                     </span>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

    </article>
    
    <!-- Local Graph Sidebar -->
    <aside class="hidden lg:block w-1/3 sticky top-32 h-[420px] bg-white/40 dark:bg-black/20 backdrop-blur-3xl rounded-[2rem] border border-gray-200/50 dark:border-white/5 ml-12 overflow-hidden mt-16 shadow-[0_8px_30px_rgb(0,0,0,0.04)] dark:shadow-[0_8px_30px_rgb(0,0,0,0.4)]">
      <div class="px-7 py-5 border-b border-gray-100 dark:border-white/5 font-bold tracking-tight text-gray-900 dark:text-gray-100 text-sm flex items-center gap-2">
        <div class="w-2 h-2 rounded-full bg-emerald-500"></div>
        Local Network
      </div>
      <div class="absolute bottom-4 right-4 z-10 opacity-70 hover:opacity-100 transition-opacity">
        <GraphZoomControls @zoom-in="localZoomIn" @zoom-out="localZoomOut" @fit="localFitView" />
      </div>
      <div ref="localGraphContainer" class="w-full h-[360px]"></div>
    </aside>

    <div 
      v-if="showLinker" 
      @mousedown.prevent
      class="linker-popup absolute z-50 transform -translate-x-1/2 bg-white/90 dark:bg-[#1a1a1a]/90 backdrop-blur-xl border border-gray-200/50 dark:border-white/10 shadow-[0_20px_60px_rgb(0,0,0,0.1)] dark:shadow-[0_20px_60px_rgb(0,0,0,0.8)] rounded-2xl p-2 w-72 transition-all duration-200 ease-out animate-in fade-in zoom-in-95"
      :style="{ top: linkerPos.top + 'px', left: linkerPos.left + 'px' }"
    >
      <input 
        v-model="searchInput" 
        placeholder="Link to article..." 
        class="w-full bg-gray-100/50 dark:bg-black/50 text-sm px-4 py-2.5 rounded-xl border-transparent focus:ring-2 focus:ring-emerald-500 outline-none text-gray-900 dark:text-gray-100 mb-2 font-medium placeholder-gray-500 dark:placeholder-gray-500"
      />
      <div class="max-h-48 overflow-y-auto space-y-1 px-1 pb-1">
        <button 
          v-for="article in filteredArticles"
          :key="article.ID"
          @click="createLink(article.ID)"
          class="w-full text-left px-3 py-2 text-sm hover:bg-gray-100 dark:hover:bg-white/5 rounded-xl text-gray-700 dark:text-gray-300 font-medium truncate transition-colors active:scale-[0.98]"
        >
          {{ article.title }}
        </button>
        <div v-if="filteredArticles.length === 0" class="text-xs text-gray-500 font-medium text-center py-4">
          No matching articles
        </div>
      </div>
    </div>
  </div>

  <ArticleHoverPreview 
    :show="showPreview" 
    :article="previewArticle" 
    :pos="previewPos" 
    @mouseenter="cancelPreviewHide"
    @mouseleave="hidePreview"
  />

  <!-- Floating Toast Notification for Reparsing Lifecycle -->
  <transition name="toast-slide">
    <div
      v-if="notificationState !== 'idle'"
      class="fixed bottom-8 right-8 z-50 max-w-md backdrop-blur-xl bg-white/95 dark:bg-[#161616]/95 border shadow-[0_20px_50px_rgba(0,0,0,0.15)] dark:shadow-[0_20px_50px_rgba(0,0,0,0.7)] rounded-2xl p-4 flex items-center gap-3.5 transition-all duration-300"
      :class="{
        'border-emerald-500/30 dark:border-emerald-500/30': notificationState === 'completed',
        'border-blue-500/30 dark:border-blue-500/30': notificationState === 'running',
        'border-red-500/30 dark:border-red-500/30': notificationState === 'error'
      }"
    >
      <!-- Icon indicator -->
      <div class="flex-shrink-0">
        <!-- Running Spinner -->
        <div v-if="notificationState === 'running'" class="w-8 h-8 rounded-xl bg-blue-500/10 dark:bg-blue-500/20 text-blue-600 dark:text-blue-400 flex items-center justify-center">
          <svg class="animate-spin h-4 w-4" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
          </svg>
        </div>

        <!-- Completed Checkmark -->
        <div v-else-if="notificationState === 'completed'" class="w-8 h-8 rounded-xl bg-emerald-500/10 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 flex items-center justify-center">
          <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"></polyline>
          </svg>
        </div>

        <!-- Error Alert -->
        <div v-else-if="notificationState === 'error'" class="w-8 h-8 rounded-xl bg-red-500/10 dark:bg-red-500/20 text-red-600 dark:text-red-400 flex items-center justify-center">
          <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10"></circle>
            <line x1="12" y1="8" x2="12" y2="12"></line>
            <line x1="12" y1="16" x2="12.01" y2="16"></line>
          </svg>
        </div>
      </div>

      <!-- Message Text -->
      <div class="flex-grow min-w-0 pr-2">
        <p class="text-xs font-bold uppercase tracking-wider text-gray-400 dark:text-gray-500 mb-0.5">
          {{ notificationState === 'running' ? 'Reparsing Article' : (notificationState === 'completed' ? 'Reparse Complete' : 'Agent Error') }}
        </p>
        <p class="text-xs font-medium text-gray-800 dark:text-gray-200 leading-snug">
          {{ notificationMessage }}
        </p>
      </div>

      <!-- Dismiss Button -->
      <button
        @click="dismissNotification"
        class="text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 p-1 rounded-lg hover:bg-gray-100 dark:hover:bg-white/5 transition-colors cursor-pointer"
        aria-label="Dismiss"
      >
        <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>
    </div>
  </transition>
</template>

<style scoped>
.toast-slide-enter-active,
.toast-slide-leave-active {
  transition: all 0.35s cubic-bezier(0.16, 1, 0.3, 1);
}

.toast-slide-enter-from,
.toast-slide-leave-to {
  opacity: 0;
  transform: translateY(16px) scale(0.96);
}
</style>
