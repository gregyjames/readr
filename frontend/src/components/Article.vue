<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import axios from 'axios'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
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

interface ArticleData {
  ID: number
  title: string
  image: string
  article: string
  tags: string
}

const allArticles = ref<ArticleData[]>([])

const currentArticle = computed(() => {
  const param = String(route.params.id || '').replace(/\.md$/, '').trim()
  return allArticles.value.find(a => 
    String(a.ID) === param ||
    a.title.trim().toLowerCase() === param.toLowerCase() ||
    a.article.replace(/^\/?articles\//, '').replace(/\.md$/, '').trim().toLowerCase() === param.toLowerCase()
  ) || null
})

const getArticleId = () => {
  if (currentArticle.value) {
    return String(currentArticle.value.ID)
  }
  return String(route.params.id || '').replace(/\.md$/, '').trim()
}

const router = useRouter()
const articleRef = ref<HTMLElement | null>(null)
const readingProgress = ref(0)

const updateReadingProgress = () => {
  if (!articleRef.value) {
    const total = document.documentElement.scrollHeight - window.innerHeight
    readingProgress.value = total > 0 ? Math.min(Math.max((window.scrollY / total) * 100, 0), 100) : 0
    return
  }

  const rect = articleRef.value.getBoundingClientRect()
  const navHeight = 80 // navbar height (h-20)
  const articleTop = rect.top + window.scrollY - navHeight
  const articleHeight = rect.height
  const viewportHeight = window.innerHeight

  const scrollableDistance = articleHeight - (viewportHeight - navHeight)
  if (scrollableDistance <= 0) {
    readingProgress.value = 100
    return
  }

  const progress = ((window.scrollY - articleTop) / scrollableDistance) * 100
  readingProgress.value = Math.min(Math.max(progress, 0), 100)
}

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

const articleTitle = computed(() => {
  return knownProperties.value.title || currentArticle.value?.title || ''
})

const isMOC = computed(() => {
  if (properties.value.some(p => p.key.toLowerCase() === 'type' && String(p.value).toLowerCase() === 'moc')) return true
  if (currentArticle.value) {
    const title = currentArticle.value.title.toLowerCase()
    if (title.startsWith('moc - ') || title.startsWith('moc:') || title.startsWith('moc ') || title === 'moc') return true
    if (currentArticle.value.tags && currentArticle.value.tags.toLowerCase().split(',').map(t => t.trim()).includes('moc')) return true
  }
  return false
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

const filteredArticles = computed(() => {
  const query = searchInput.value.toLowerCase()
  const currentId = Number(getArticleId())
  return allArticles.value.filter(a => 
    a.title.toLowerCase().includes(query) && a.ID !== currentId
  )
})


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
    const param = String(route.params.id || '').trim()
    if (!param) return

    if (allArticles.value.length === 0) {
      await fetchArticles()
    }

    let articleURL = ''
    if (currentArticle.value?.article) {
      articleURL = currentArticle.value.article.startsWith('/api') ? currentArticle.value.article : `/api${currentArticle.value.article}`
    } else {
      const cleanParam = param.endsWith('.md') ? param : `${param}.md`
      articleURL = `/api/articles/${encodeURIComponent(cleanParam)}`
    }

    // Always append cache buster to prevent stale browser cache after navigation
    const fetchUrl = `${articleURL}?t=${Date.now()}`

    const res = await axios.get(fetchUrl)
    const raw = String(res.data)
    rawMarkdown.value = raw
    properties.value = parseFrontmatter(raw)
    
    const parsedRaw = raw.replace(/\[\[([^[\]\n]+?)\]\]/g, (_, p1) => {
      const fullText = String(p1 || '').trim()
      
      // 1. First check if the full un-split string matches an article title (e.g. "Title | Site Name")
      const matchFull = allArticles.value.find(a => 
        a.title.trim().toLowerCase() === fullText.toLowerCase() ||
        String(a.ID) === fullText ||
        a.article.replace(/^\/?articles\//, '').replace(/\.md$/, '').trim().toLowerCase() === fullText.toLowerCase()
      )

      if (matchFull) {
        return `<a href="/articles/${matchFull.ID}" data-article-id="${matchFull.ID}" class="wikilink font-semibold text-emerald-600 dark:text-emerald-400 no-underline hover:underline hover:text-emerald-700 dark:hover:text-emerald-300 transition-colors bg-emerald-50/50 dark:bg-emerald-900/20 px-1.5 py-0.5 rounded-md border border-emerald-100 dark:border-emerald-800/50 cursor-pointer">${matchFull.title}</a>`
      }

      // 2. If no full match and contains '|', treat as [[Target|Display Alias]]
      const parts = p1.split('|')
      const targetTitle = parts[0].trim()
      const display = parts.length > 1 ? parts.slice(1).join('|').trim() : targetTitle
      
      const targetArticle = allArticles.value.find(a => 
        a.title.trim().toLowerCase() === targetTitle.toLowerCase() ||
        String(a.ID) === targetTitle ||
        a.article.replace(/^\/?articles\//, '').replace(/\.md$/, '').trim().toLowerCase() === targetTitle.toLowerCase()
      )

      if (targetArticle) {
        return `<a href="/articles/${targetArticle.ID}" data-article-id="${targetArticle.ID}" class="wikilink font-semibold text-emerald-600 dark:text-emerald-400 no-underline hover:underline hover:text-emerald-700 dark:hover:text-emerald-300 transition-colors bg-emerald-50/50 dark:bg-emerald-900/20 px-1.5 py-0.5 rounded-md border border-emerald-100 dark:border-emerald-800/50 cursor-pointer">${display}</a>`
      } else {
        return `<span class="font-semibold text-emerald-600/70 dark:text-emerald-400/70 bg-emerald-50/30 dark:bg-emerald-900/10 px-1.5 py-0.5 rounded-md border border-dashed border-emerald-200/50 dark:border-emerald-800/30" title="Target article not found in vault: ${targetTitle}">${display}</span>`
      }
    })

    // Strip YAML frontmatter (--- ... ---) so it never renders in the article view                                                                                 
    const strippedRaw = parsedRaw.replace(/^---[\s\S]*?---\n?/, '')  

    // Sanitize before assignment: article bodies and wikilink-injected titles are
    // untrusted, and marked passes raw HTML through.
    markdownContent.value = DOMPurify.sanitize(await marked.parse(strippedRaw, {
      gfm: false,
      async: true
    }))

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
  
  window.addEventListener('scroll', updateReadingProgress, { passive: true })
  window.addEventListener('resize', updateReadingProgress, { passive: true })
  
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

watch(markdownContent, () => {
  nextTick(updateReadingProgress)
})

onBeforeUnmount(() => {
  if (notificationTimer) {
    clearTimeout(notificationTimer)
  }
  window.removeEventListener('scroll', updateReadingProgress)
  window.removeEventListener('resize', updateReadingProgress)
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
  <!-- Reading Progress Bar -->
  <div
    class="fixed top-0 md:left-16 left-0 right-0 h-[2px] z-40 bg-gray-200/50 dark:bg-white/5 pointer-events-none"
    aria-hidden="true"
  >
    <div
      class="h-full bg-emerald-600 dark:bg-emerald-400 transition-[width] duration-100 ease-out"
      :style="{ width: `${readingProgress}%` }"
    ></div>
  </div>

  <div v-if="articleError" class="bg-red-50 dark:bg-red-950/40 text-red-600 dark:text-red-400 p-3 text-center w-full text-xs font-mono border-b border-red-200 dark:border-red-900/40">
    {{ articleError }}
  </div>

  <div class="flex flex-col lg:flex-row w-full max-w-6xl mx-auto items-start relative px-4 sm:px-6">
    <article ref="articleRef" class="w-full lg:w-2/3 max-w-2xl mx-auto py-8 sm:py-12 transition-colors">
      
      <!-- Back to Library Nav -->
      <div class="mb-6">
        <router-link
          to="/"
          class="inline-flex items-center gap-1.5 text-xs font-mono text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors"
        >
          <span>&larr;</span>
          <span>Vault Library</span>
        </router-link>
      </div>

      <!-- If Editing Markdown -->
      <div v-if="isEditing" class="mb-8 w-full">
        <textarea v-model="editContent" rows="18" class="w-full p-4 bg-gray-50/50 dark:bg-[#12151C] border border-gray-200 dark:border-white/10 rounded-xl text-gray-900 dark:text-gray-100 font-mono text-sm leading-relaxed focus:ring-2 focus:ring-emerald-500/20 focus:border-emerald-500 outline-none resize-y transition-all"></textarea>
        <div class="flex justify-end gap-2 mt-3">
          <button @click="cancelEditing" :disabled="isSaving" class="px-4 py-2 text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 text-xs font-medium transition-colors disabled:opacity-50 cursor-pointer">Cancel</button>
          <button @click="saveEdit" :disabled="isSaving" class="px-4 py-2 bg-gray-900 hover:bg-black dark:bg-white dark:hover:bg-gray-100 text-white dark:text-gray-950 text-xs font-medium rounded-lg transition-all active:scale-[0.98] disabled:opacity-50 flex items-center gap-1.5 cursor-pointer shadow-xs">
            <svg v-if="isSaving" class="animate-spin h-3.5 w-3.5 text-white dark:text-gray-950" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path></svg>
            {{ isSaving ? 'Saving...' : 'Save Changes' }}
          </button>
        </div>
      </div>

      <div v-else>
        <!-- Editorial Provenance Masthead -->
        <header v-if="properties.length > 0" class="mb-8 pb-6 border-b border-gray-200/60 dark:border-white/[0.06]">
          <!-- Top Row: Source Publisher, Date, and Actions -->
          <div class="flex items-center justify-between gap-4">
            <div class="flex flex-wrap items-center gap-2">
              <!-- MOC Hub Badge -->
              <span
                v-if="isMOC"
                class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-md bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-500/20 text-xs font-medium"
              >
                <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
                Map of Content
              </span>

              <!-- Source Badge -->
              <a
                v-if="knownProperties.source"
                :href="knownProperties.source"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md bg-gray-100 dark:bg-white/[0.04] hover:bg-gray-200/80 dark:hover:bg-white/[0.08] text-gray-700 dark:text-gray-300 border border-gray-200/60 dark:border-white/[0.06] text-xs font-medium transition-colors"
              >
                <span>{{ getHostname(knownProperties.source) }}</span>
                <svg xmlns="http://www.w3.org/2000/svg" width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path><polyline points="15 3 21 3 21 9"></polyline><line x1="10" y1="14" x2="21" y2="3"></line></svg>
              </a>

              <!-- Captured Date -->
              <div v-if="knownProperties.date" class="text-xs text-gray-400 dark:text-gray-500 font-mono">
                {{ formatDisplayDate(knownProperties.date) }}
              </div>
            </div>

            <!-- Action Controls -->
            <div class="flex items-center gap-1.5">
              <button
                @click="reparseArticle"
                :disabled="isReparsing"
                class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-colors cursor-pointer disabled:opacity-50"
                title="Re-run pipeline agents"
              >
                <svg v-if="isReparsing" class="animate-spin h-3 w-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path></svg>
                <svg v-else xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"></path><path d="M21 3v5h-5"></path></svg>
                <span>{{ isReparsing ? 'Running...' : 'Reparse' }}</span>
              </button>

              <button
                @click="startEditing"
                class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-colors cursor-pointer"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg>
                <span>Edit</span>
              </button>
            </div>
          </div>

          <!-- Tags strip -->
          <div v-if="knownProperties.tags.length > 0" class="flex flex-wrap items-center gap-1.5 mt-3">
            <span
              v-for="tag in knownProperties.tags"
              :key="tag"
              class="text-[11px] font-mono px-2 py-0.5 rounded bg-gray-100/80 dark:bg-white/[0.04] text-gray-600 dark:text-gray-400 border border-gray-200/50 dark:border-white/[0.04]"
            >
              #{{ tag }}
            </span>
          </div>
        </header>

        <!-- Standalone edit button for legacy articles without frontmatter -->
        <div v-else class="flex justify-end mb-6">
          <button @click="startEditing" class="px-3 py-1.5 bg-gray-100 dark:bg-white/[0.06] text-gray-700 dark:text-gray-300 rounded-md text-xs font-medium hover:bg-gray-200 dark:hover:bg-white/[0.1] transition-colors cursor-pointer">Edit Markdown</button>
        </div>

        <!-- Article Main Title -->
        <h1 v-if="articleTitle" class="text-2xl sm:text-3xl font-semibold tracking-tight text-gray-900 dark:text-gray-100 mb-6 font-sans leading-tight">
          {{ articleTitle }}
        </h1>

        <!-- Markdown Prose Body -->
        <div
          class="prose prose-base dark:prose-invert max-w-none prose-p:text-gray-700 dark:prose-p:text-gray-300 prose-p:leading-relaxed prose-p:font-serif prose-headings:font-sans prose-headings:font-semibold prose-headings:tracking-tight prose-a:text-emerald-600 dark:prose-a:text-emerald-400 prose-img:rounded-xl prose-pre:bg-gray-900 dark:prose-pre:bg-[#0E1117] prose-pre:rounded-xl prose-pre:border prose-pre:border-black/10 dark:prose-pre:border-white/10"
          v-html="markdownContent"
        />
      </div>

      <!-- Linked Mentions (Backlinks) -->
      <div v-if="backlinks.length > 0" class="mt-16 pt-8 border-t border-gray-200/60 dark:border-white/[0.06]">
        <h3 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-gray-100 mb-4 flex items-center gap-2">
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-gray-400"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>
          Linked Mentions ({{ backlinks.length }})
        </h3>
        
        <div class="rounded-xl border border-gray-200/80 dark:border-white/[0.08] bg-white dark:bg-[#12151C] overflow-hidden shadow-2xs">
          <div
            v-for="link in backlinks"
            :key="link.ID"
            :data-id="link.ID"
            class="backlink-row p-3.5 border-b last:border-0 border-gray-100 dark:border-white/[0.04] hover:bg-gray-50/80 dark:hover:bg-white/[0.03] transition-colors cursor-pointer flex items-center justify-between gap-4"
            @click="router.push(`/articles/${link.ID}`)"
          >
            <span class="text-xs font-medium text-gray-800 dark:text-gray-200 hover:text-emerald-600 dark:hover:text-emerald-400 truncate">
              {{ link.title }}
            </span>
            <div class="flex items-center gap-1.5 flex-shrink-0">
              <span v-for="tag in (link.tags ? link.tags.split(',') : []).slice(0, 2)" :key="tag" class="text-[10px] font-mono px-2 py-0.5 rounded bg-gray-100 dark:bg-white/[0.05] text-gray-500 dark:text-gray-400">
                #{{ tag.trim() }}
              </span>
            </div>
          </div>
        </div>
      </div>

    </article>
    
    <!-- Local Graph Sidebar -->
    <aside class="hidden lg:block w-1/3 sticky top-24 bg-white dark:bg-[#12151C] rounded-xl border border-gray-200/80 dark:border-white/[0.08] ml-8 overflow-hidden shadow-2xs">
      <div class="px-4 py-3 border-b border-gray-100 dark:border-white/[0.06] font-medium text-xs text-gray-700 dark:text-gray-300 flex items-center justify-between">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          <span>Local Orbit Graph</span>
        </div>
        <GraphZoomControls @zoom-in="localZoomIn" @zoom-out="localZoomOut" @fit="localFitView" />
      </div>
      <div ref="localGraphContainer" class="w-full h-[320px]"></div>
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
