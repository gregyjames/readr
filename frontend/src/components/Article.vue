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
    else if (p.type === 'tags') meta.tags = Array.isArray(p.value) ? p.value : String(p.value).split(',').map((t: string) => t.trim()).filter(Boolean)
    else if (p.type === 'date') meta.date = p.value
    else if (p.type === 'image') meta.cover = p.value
    else meta.custom.push({ key: p.key, label: p.label, value: p.value })
  }

  if (meta.tags.length === 0 && currentArticle.value?.tags) {
    meta.tags = currentArticle.value.tags.split(',').map((t: string) => t.trim()).filter(Boolean)
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

const heroImage = computed(() => {
  return knownProperties.value.cover || currentArticle.value?.image || ''
})

const wordCount = computed(() => {
  if (!rawMarkdown.value) return 0
  return rawMarkdown.value.trim().split(/\s+/).length
})

const estimatedReadingTime = computed(() => {
  const words = wordCount.value
  const minutes = Math.max(1, Math.ceil(words / 200))
  return `${minutes} min read`
})

const readerFont = ref<'serif' | 'sans'>('serif')
const readerFontSize = ref<'sm' | 'base' | 'lg'>('base')

const getProceduralGradient = (id: number) => {
  const gradients = [
    'from-emerald-50 via-teal-50/70 to-slate-100 border-emerald-200/60 text-emerald-900 dark:from-emerald-950/60 dark:via-teal-950/70 dark:to-slate-950 dark:border-white/10 dark:text-emerald-300',
    'from-indigo-50 via-purple-50/70 to-slate-100 border-indigo-200/60 text-indigo-900 dark:from-indigo-950/60 dark:via-purple-950/70 dark:to-slate-950 dark:border-white/10 dark:text-indigo-300',
    'from-rose-50 via-orange-50/70 to-stone-100 border-rose-200/60 text-rose-900 dark:from-rose-950/60 dark:via-stone-950 dark:to-slate-950 dark:border-white/10 dark:text-rose-300',
    'from-amber-50 via-yellow-50/70 to-stone-100 border-amber-200/60 text-amber-900 dark:from-amber-950/60 dark:via-orange-950/70 dark:to-slate-950 dark:border-white/10 dark:text-amber-300',
    'from-sky-50 via-blue-50/70 to-slate-100 border-sky-200/60 text-sky-900 dark:from-sky-950/60 dark:via-blue-950/70 dark:to-slate-950 dark:border-white/10 dark:text-cyan-300',
    'from-violet-50 via-fuchsia-50/70 to-slate-100 border-violet-200/60 text-violet-900 dark:from-violet-950/60 dark:via-fuchsia-950/70 dark:to-slate-950 dark:border-white/10 dark:text-violet-300',
  ]
  return gradients[Math.abs(Number(id) || 0) % gradients.length]
}

function removeDuplicateHeroImage(markdown: string, heroUrl: string): string {
  if (!heroUrl || !markdown) return markdown

  const cleanHero = heroUrl.trim()
  const heroMatch = cleanHero.match(/\/images\/(\d+)\//)
  const heroArticleId = heroMatch ? heroMatch[1] : ''
  const heroBaseFilename = cleanHero.split('/').pop()?.split('?')[0] || ''

  const isMatch = (src: string) => {
    if (!src) return false
    const cleanSrc = src.trim()
    if (cleanSrc === cleanHero) return true
    if (cleanSrc.split('?')[0] === cleanHero.split('?')[0]) return true

    if (heroArticleId) {
      const srcMatch = cleanSrc.match(/\/images\/(\d+)\//)
      if (srcMatch && srcMatch[1] === heroArticleId) {
        const srcFilename = cleanSrc.split('/').pop()?.split('?')[0] || ''
        const isHeroOrCover = (name: string) => /hero|cover/i.test(name)
        if (isHeroOrCover(srcFilename) && isHeroOrCover(heroBaseFilename)) {
          return true
        }
        if (srcFilename.replace(/^cover_/, '') === heroBaseFilename.replace(/^cover_/, '')) {
          return true
        }
      }
    }
    return false
  }

  // 1. Linked image: [![alt](src)](href)
  markdown = markdown.replace(/\[\s*!\[([^\]]*)\]\(([^)]+)\)\s*\]\([^)]+\)/g, (fullMatch, _alt, src) => {
    return isMatch(src) ? '' : fullMatch
  })

  // 2. Standard markdown image: ![alt](src)
  markdown = markdown.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (fullMatch, _alt, src) => {
    return isMatch(src) ? '' : fullMatch
  })

  // 3. HTML img: <img ... src="..." ...>
  markdown = markdown.replace(/<img[^>]+src=["']([^"']+)["'][^>]*>/gi, (fullMatch, src) => {
    return isMatch(src) ? '' : fullMatch
  })

  return markdown
}

const copiedLink = ref(false)

const copyArticleLink = () => {
  try {
    navigator.clipboard.writeText(window.location.href)
    copiedLink.value = true
    setTimeout(() => {
      copiedLink.value = false
    }, 2000)
  } catch {}
}

const isGraphOpen = ref(localStorage.getItem('readr_local_graph_open') !== 'false')

const toggleGraphSidebar = () => {
  isGraphOpen.value = !isGraphOpen.value
  try {
    localStorage.setItem('readr_local_graph_open', String(isGraphOpen.value))
  } catch {}
  
  if (isGraphOpen.value) {
    nextTick(() => {
      setTimeout(() => {
        if (!localNetwork || !localGraphContainer.value?.querySelector('canvas')) {
          loadLocalGraph()
        } else {
          localNetwork.setSize('100%', '100%')
          localNetwork.redraw()
          localNetwork.fit({ animation: { duration: 300, easingFunction: 'easeInOutQuad' } })
        }
      }, 50)
    })
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

    if (localNetwork && localGraphContainer.value && localGraphContainer.value.querySelector('canvas')) {
      localNetwork.setData({ nodes: localNodes, edges: connectedEdges })
      localNetwork.setOptions(options)
      localNetwork.setSize('100%', '100%')
      localNetwork.redraw()
      localNetwork.once('afterDrawing', () => { localNetwork?.fit({ animation: { duration: 400, easingFunction: 'easeInOutQuad' } }); });
    } else {
      if (localNetwork) {
        localNetwork.destroy()
        localNetwork = null
      }
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
    let strippedRaw = parsedRaw.replace(/^---[\s\S]*?---\n?/, '')  

    // Deduplicate: If hero cover image is displayed in the masthead, strip the duplicate from body
    if (heroImage.value) {
      strippedRaw = removeDuplicateHeroImage(strippedRaw, heroImage.value)
    }  

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

  <div class="flex w-full min-h-screen relative">
    
    <!-- Main Article Reading Column -->
    <div class="flex-1 min-w-0 flex justify-center px-4 sm:px-8 py-6 sm:py-10 transition-all duration-300 relative">
      <!-- Atmospheric Backdrop Glow on reading canvas -->
      <div class="pointer-events-none absolute -top-10 left-1/2 -translate-x-1/2 w-full max-w-4xl h-80 bg-gradient-to-b from-emerald-500/[0.03] dark:from-emerald-500/[0.04] to-transparent blur-3xl -z-10"></div>

      <article
        ref="articleRef"
        class="w-full transition-all duration-300"
        :class="isGraphOpen ? 'max-w-2xl' : 'max-w-3xl'"
      >
      
      <!-- Floating Reader HUD & Companion Bar (Pinned at top) -->
      <div class="sticky top-16 md:top-4 z-30 flex items-center justify-between gap-3 mb-8 p-2 rounded-2xl bg-white/80 dark:bg-[#12151C]/90 backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] shadow-[0_8px_30px_rgba(0,0,0,0.05)] dark:shadow-md transition-all">
        <div class="flex items-center gap-2">
          <router-link
            to="/"
            class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl text-xs font-mono font-medium text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white hover:bg-gray-100 dark:hover:bg-white/[0.06] transition-all"
          >
            <span>&larr;</span>
            <span>Vault</span>
          </router-link>
          <div class="h-4 w-px bg-gray-200 dark:bg-white/10 hidden sm:block"></div>
          <span class="text-xs font-mono text-gray-400 dark:text-gray-500 hidden sm:inline truncate max-w-[180px]">
            {{ articleTitle }}
          </span>
        </div>

        <div class="flex items-center gap-1.5 sm:gap-2">
          <!-- Reading Progress Pill -->
          <div class="flex items-center gap-1.5 px-2.5 py-1 rounded-xl bg-gray-100/80 dark:bg-white/[0.04] text-[11px] font-mono text-gray-600 dark:text-gray-400 border border-gray-200/50 dark:border-white/[0.04]">
            <div class="w-2 h-2 rounded-full border border-emerald-500/40 flex items-center justify-center relative overflow-hidden bg-emerald-500/10">
              <div class="w-full bg-emerald-500 transition-all duration-150" :style="{ height: `${readingProgress}%` }"></div>
            </div>
            <span>{{ Math.round(readingProgress) }}%</span>
          </div>

          <!-- Typography Toggle: Newsreader Serif vs Geist Sans -->
          <div class="flex items-center bg-gray-100/70 dark:bg-white/[0.04] p-0.5 rounded-xl border border-gray-200/50 dark:border-white/[0.04]">
            <button
              @click="readerFont = 'serif'"
              class="px-2 py-1 rounded-lg text-xs font-serif transition-all cursor-pointer"
              :class="readerFont === 'serif' ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs font-medium' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'"
              title="Serif typography (Newsreader)"
            >
              Aa
            </button>
            <button
              @click="readerFont = 'sans'"
              class="px-2 py-1 rounded-lg text-xs font-sans transition-all cursor-pointer"
              :class="readerFont === 'sans' ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs font-medium' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'"
              title="Sans-serif typography (Geist)"
            >
              Aa
            </button>
          </div>

          <!-- Font Size Adjuster -->
          <button
            @click="readerFontSize = readerFontSize === 'sm' ? 'base' : (readerFontSize === 'base' ? 'lg' : 'sm')"
            class="px-2 py-1 rounded-xl text-xs font-mono text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white bg-gray-100/70 dark:bg-white/[0.04] border border-gray-200/50 dark:border-white/[0.04] transition-all cursor-pointer"
            :title="`Text size: ${readerFontSize}`"
          >
            <span v-if="readerFontSize === 'sm'">A-</span>
            <span v-else-if="readerFontSize === 'base'">A</span>
            <span v-else class="font-bold text-emerald-600 dark:text-emerald-400">A+</span>
          </button>

          <!-- Copy Link -->
          <button
            @click="copyArticleLink"
            class="p-1.5 rounded-xl text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-white/[0.06] transition-all cursor-pointer"
            :title="copiedLink ? 'Link copied to clipboard!' : 'Copy link'"
          >
            <svg v-if="copiedLink" class="w-3.5 h-3.5 text-emerald-500" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg>
            <svg v-else class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>
          </button>

          <!-- Edit Action -->
          <button
            @click="startEditing"
            class="p-1.5 rounded-xl text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-white/[0.06] transition-all cursor-pointer"
            title="Edit markdown"
          >
            <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg>
          </button>
        </div>
      </div>

      <!-- If Editing Markdown -->
      <div v-if="isEditing" class="mb-8 w-full">
        <textarea v-model="editContent" rows="18" class="w-full p-4 bg-gray-50/50 dark:bg-[#12151C] border border-gray-200 dark:border-white/10 rounded-2xl text-gray-900 dark:text-gray-100 font-mono text-sm leading-relaxed focus:ring-2 focus:ring-emerald-500/20 focus:border-emerald-500 outline-none resize-y transition-all"></textarea>
        <div class="flex justify-end gap-2 mt-3">
          <button @click="cancelEditing" :disabled="isSaving" class="px-4 py-2 text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 text-xs font-medium transition-colors disabled:opacity-50 cursor-pointer">Cancel</button>
          <button @click="saveEdit" :disabled="isSaving" class="px-4 py-2 bg-gray-900 hover:bg-black dark:bg-white dark:hover:bg-gray-100 text-white dark:text-gray-950 text-xs font-medium rounded-xl transition-all active:scale-[0.98] disabled:opacity-50 flex items-center gap-1.5 cursor-pointer shadow-xs">
            <svg v-if="isSaving" class="animate-spin h-3.5 w-3.5 text-white dark:text-gray-950" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path></svg>
            {{ isSaving ? 'Saving...' : 'Save Changes' }}
          </button>
        </div>
      </div>

      <div v-else>
        <!-- Hero Cover Masthead (Option 1: Luminous Aura & Cinematic Framing) -->
        <div v-if="heroImage" class="relative mb-10 group isolate">
          <!-- Ambient Luminous Glow Behind the Card (Soft watercolor in light mode, luminous aurora in dark mode) -->
          <div class="absolute -inset-4 sm:-inset-6 -z-10 overflow-visible pointer-events-none">
            <img
              :src="heroImage"
              alt=""
              aria-hidden="true"
              class="w-full h-full object-cover rounded-[2.5rem] filter blur-3xl opacity-20 dark:opacity-65 scale-105 sm:scale-110 transition-opacity duration-700 group-hover:opacity-35 dark:group-hover:opacity-95 dark:saturate-150"
            />
          </div>

          <!-- Cinematic Framed Container -->
          <div class="relative rounded-[2rem] overflow-hidden border border-black/[0.07] dark:border-white/10 shadow-[0_20px_50px_-15px_rgba(0,0,0,0.08)] dark:shadow-2xl aspect-[16/9] sm:aspect-[21/9] max-h-[460px] bg-white dark:bg-[#12151C] z-10">
            <img
              :src="heroImage"
              :alt="articleTitle"
              class="w-full h-full object-cover object-center group-hover:scale-105 transition-transform duration-1000 ease-out"
            />
            
            <!-- Subtle Vignette Glaze for Top/Bottom Glass Badges -->
            <div class="absolute inset-0 bg-gradient-to-t from-black/35 via-transparent to-black/15 dark:from-black/60 dark:to-black/20 pointer-events-none"></div>

            <!-- Top Left Provenance Pill -->
            <div v-if="knownProperties.source" class="absolute top-4 left-4 z-10">
              <a
                :href="knownProperties.source"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-black/40 dark:bg-black/50 hover:bg-black/60 dark:hover:bg-black/80 backdrop-blur-xl text-white text-xs font-mono font-medium border border-white/25 shadow-sm transition-all hover:scale-105"
              >
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                <span>{{ getHostname(knownProperties.source) }}</span>
                <span class="text-white/60">&nearr;</span>
              </a>
            </div>

            <!-- Bottom Right Date Pill -->
            <div v-if="knownProperties.date" class="absolute bottom-4 right-4 z-10">
              <span class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-black/40 dark:bg-black/50 backdrop-blur-xl text-white/95 text-xs font-mono font-medium border border-white/25 shadow-sm">
                <svg class="w-3 h-3 text-emerald-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>
                {{ formatDisplayDate(knownProperties.date) }}
              </span>
            </div>
          </div>
        </div>

        <!-- Procedural Aurora Banner for Articles without an image -->
        <div v-else class="relative mb-10 group isolate">
          <!-- Ambient Glow for Procedural Banner -->
          <div
            class="absolute -inset-4 sm:-inset-6 -z-10 rounded-[2.5rem] filter blur-3xl opacity-25 dark:opacity-35 scale-105 sm:scale-110 pointer-events-none transition-opacity duration-700 bg-gradient-to-br"
            :class="getProceduralGradient(Number(getArticleId()) || 0)"
          ></div>

          <div
            class="relative rounded-[2rem] overflow-hidden border shadow-[0_20px_50px_-15px_rgba(0,0,0,0.06)] dark:shadow-xl h-44 sm:h-52 flex flex-col justify-between p-6 bg-gradient-to-br z-10"
            :class="getProceduralGradient(Number(getArticleId()) || 0)"
          >
            <!-- Dot matrix pattern overlay -->
            <div class="absolute inset-0 opacity-15 bg-[radial-gradient(#000_1px,transparent_1px)] dark:bg-[radial-gradient(#fff_1px,transparent_1px)] [background-size:16px_16px] pointer-events-none"></div>

            <!-- Top Row Badges -->
            <div class="flex items-center justify-between w-full z-10">
              <div v-if="knownProperties.source">
                <a
                  :href="knownProperties.source"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-white/80 dark:bg-black/40 hover:bg-white dark:hover:bg-black/60 backdrop-blur-xl text-gray-800 dark:text-white text-xs font-mono font-medium border border-black/[0.08] dark:border-white/15 shadow-2xs transition-all"
                >
                  <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
                  <span>{{ getHostname(knownProperties.source) }}</span>
                  <span class="text-gray-400 dark:text-white/60">&nearr;</span>
                </a>
              </div>
              <div v-else class="text-[10px] font-mono uppercase tracking-widest text-gray-500 dark:text-white/50 font-semibold">
                READR VAULT
              </div>

              <div v-if="knownProperties.date" class="text-xs font-mono text-gray-500 dark:text-white/70 font-medium">
                {{ formatDisplayDate(knownProperties.date) }}
              </div>
            </div>

            <!-- Monospace Watermark in bottom corner -->
            <div class="flex items-end justify-between z-10">
              <span class="font-mono text-[10px] tracking-widest uppercase text-gray-400 dark:text-white/40 font-semibold">#DOCUMENT</span>
              <span class="font-mono text-xs font-semibold text-gray-600 dark:text-white/60">{{ wordCount }} WORDS</span>
            </div>
          </div>
        </div>

        <!-- Editorial Masthead Header -->
        <header class="mb-8 pb-6 border-b border-gray-200/60 dark:border-white/[0.06]">
          <!-- Top Row: Eyebrow Metadata & Agent Actions -->
          <div class="flex items-center justify-between gap-4 mb-3">
            <div class="flex flex-wrap items-center gap-2">
              <span
                v-if="isMOC"
                class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-md bg-amber-500/10 text-amber-700 dark:text-amber-300 border border-amber-500/20 text-xs font-mono font-semibold"
              >
                ★ MOC HUB
              </span>
              <span class="text-xs font-mono text-emerald-600 dark:text-emerald-400 font-semibold tracking-wide">
                // VAULT ARTICLE
              </span>
              <span class="text-gray-300 dark:text-gray-700">•</span>
              <span class="text-xs font-mono text-gray-400 dark:text-gray-500">
                {{ estimatedReadingTime }} · {{ wordCount }} words
              </span>
            </div>

            <!-- Reparse Agent Action -->
            <button
              @click="reparseArticle"
              :disabled="isReparsing"
              class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs font-mono text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 hover:bg-gray-100 dark:hover:bg-white/[0.05] transition-colors cursor-pointer disabled:opacity-50"
              title="Re-run pipeline agents"
            >
              <svg v-if="isReparsing" class="animate-spin h-3 w-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path></svg>
              <svg v-else xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8"></path><path d="M21 3v5h-5"></path></svg>
              <span>{{ isReparsing ? 'Agent Running...' : 'Reparse' }}</span>
            </button>
          </div>

          <!-- Article Main Title -->
          <h1 v-if="articleTitle" class="text-2xl sm:text-4xl font-bold tracking-tight text-gray-900 dark:text-gray-100 mb-4 font-['Outfit'] leading-tight">
            {{ articleTitle }}
          </h1>

          <!-- Tags Bar -->
          <div v-if="knownProperties.tags.length > 0" class="flex flex-wrap items-center gap-1.5 mt-4">
            <span
              v-for="tag in knownProperties.tags"
              :key="tag"
              class="text-[11px] font-mono px-2.5 py-0.5 rounded-full bg-gray-100/80 dark:bg-white/[0.04] text-gray-600 dark:text-gray-400 border border-gray-200/50 dark:border-white/[0.04]"
            >
              #{{ tag }}
            </span>
          </div>
        </header>

        <!-- Markdown Prose Body with Custom Typography -->
        <div
          :class="[
            'prose dark:prose-invert max-w-none transition-all duration-200',
            readerFont === 'serif' ? 'font-[\'Newsreader\'] prose-p:font-[\'Newsreader\'] prose-p:text-gray-800 dark:prose-p:text-gray-200 prose-p:leading-[1.85]' : 'font-sans prose-p:font-sans prose-p:text-gray-800 dark:prose-p:text-gray-200 prose-p:leading-relaxed',
            readerFontSize === 'lg' ? 'text-lg sm:text-xl' : (readerFontSize === 'sm' ? 'text-sm sm:text-base' : 'text-base sm:text-lg'),
            'prose-headings:font-[\'Outfit\'] prose-headings:font-bold prose-headings:tracking-tight',
            'prose-a:text-emerald-600 dark:prose-a:text-emerald-400 prose-a:underline-offset-4',
            'prose-blockquote:border-l-2 prose-blockquote:border-emerald-500 prose-blockquote:bg-emerald-500/[0.04] prose-blockquote:py-1.5 prose-blockquote:px-5 prose-blockquote:rounded-r-2xl prose-blockquote:italic',
            'prose-img:rounded-2xl prose-img:shadow-sm',
            'prose-pre:bg-gray-950 dark:prose-pre:bg-[#0A0C10] prose-pre:rounded-2xl prose-pre:border prose-pre:border-black/10 dark:prose-pre:border-white/10'
          ]"
          v-html="markdownContent"
        />
      </div>

      <!-- Linked Mentions (Backlinks) -->
      <div v-if="backlinks.length > 0" class="mt-16 pt-8 border-t border-gray-200/60 dark:border-white/[0.06]">
        <h3 class="text-sm font-semibold tracking-tight text-gray-900 dark:text-gray-100 mb-4 flex items-center gap-2 font-mono">
          <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
          Linked Mentions ({{ backlinks.length }})
        </h3>
        
        <div class="rounded-2xl border border-gray-200/80 dark:border-white/[0.08] bg-white dark:bg-[#12151C] overflow-hidden shadow-2xs">
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
    </div>
    
    <!-- Obsidian-Style Dedicated Right Docked Sidebar (Full Height, Pinned / Sticky) -->
    <aside
      v-show="isGraphOpen"
      class="hidden lg:flex flex-col w-80 xl:w-96 h-screen sticky top-0 right-0 border-l border-black/[0.06] dark:border-white/[0.06] bg-white/95 dark:bg-[#0C0E14]/95 backdrop-blur-xl z-30 select-none flex-shrink-0 transition-all duration-300"
    >
      <!-- Obsidian Sidebar Header Strip -->
      <div class="h-12 border-b border-black/[0.06] dark:border-white/[0.06] flex items-center justify-between px-3.5 flex-shrink-0 bg-gray-50/50 dark:bg-white/[0.01]">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
          <span class="font-mono text-xs font-semibold text-gray-700 dark:text-gray-300 uppercase tracking-wider">
            Local Graph
          </span>
          <span class="text-[10px] font-mono px-1.5 py-0.5 rounded-md bg-gray-200/60 dark:bg-white/[0.06] text-gray-500 dark:text-gray-400">
            {{ backlinks.length + 1 }}
          </span>
        </div>

        <div class="flex items-center gap-1">
          <GraphZoomControls @zoom-in="localZoomIn" @zoom-out="localZoomOut" @fit="localFitView" />
          <button
            @click="toggleGraphSidebar"
            class="p-1.5 rounded-lg text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-white/[0.06] transition-colors cursor-pointer"
            title="Collapse sidebar"
          >
            <!-- Obsidian Sidebar Collapse Icon -->
            <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect width="18" height="18" x="3" y="3" rx="2"/>
              <path d="M15 3v18"/>
              <path d="m8 9 3 3-3 3"/>
            </svg>
          </button>
        </div>
      </div>

      <!-- Graph Canvas Pane -->
      <div class="h-[280px] xl:h-[320px] relative bg-gray-50/30 dark:bg-black/20 flex-shrink-0 border-b border-black/[0.06] dark:border-white/[0.06]">
        <div ref="localGraphContainer" class="absolute inset-0 w-full h-full"></div>
      </div>

      <!-- Scrollable Document Metadata & Backlinks Section -->
      <div class="flex-1 overflow-y-auto divide-y divide-black/[0.06] dark:divide-white/[0.06]">
        <!-- Document Properties -->
        <div class="p-3.5 space-y-3 bg-white dark:bg-[#0C0E14]">
          <div class="flex items-center justify-between text-[10px] font-mono text-gray-400 uppercase tracking-wider font-semibold">
            <span>Properties</span>
            <span v-if="knownProperties.date" class="text-gray-400 font-normal">
              {{ formatDisplayDate(knownProperties.date) }}
            </span>
          </div>

          <div class="grid grid-cols-2 gap-2 text-xs font-mono">
            <div class="p-2.5 rounded-xl bg-gray-50 dark:bg-white/[0.02] border border-black/[0.04] dark:border-white/[0.04]">
              <span class="text-[10px] text-gray-400 block mb-0.5">READ TIME</span>
              <span class="font-semibold text-gray-800 dark:text-gray-200">{{ estimatedReadingTime }}</span>
            </div>
            <div class="p-2.5 rounded-xl bg-gray-50 dark:bg-white/[0.02] border border-black/[0.04] dark:border-white/[0.04]">
              <span class="text-[10px] text-gray-400 block mb-0.5">WORD COUNT</span>
              <span class="font-semibold text-gray-800 dark:text-gray-200">{{ wordCount }} words</span>
            </div>
          </div>

          <!-- Tags in Properties -->
          <div v-if="knownProperties.tags && knownProperties.tags.length > 0" class="pt-0.5 space-y-1.5">
            <span class="text-[10px] font-mono text-gray-400 block uppercase font-semibold">TAGS</span>
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="tag in knownProperties.tags"
                :key="tag"
                class="text-[11px] font-mono px-2 py-0.5 rounded-md bg-gray-100/90 dark:bg-white/[0.05] text-gray-600 dark:text-gray-300 border border-black/[0.04] dark:border-white/[0.04]"
              >
                #{{ tag }}
              </span>
            </div>
          </div>

          <div v-if="knownProperties.source" class="pt-0.5">
            <a
              :href="knownProperties.source"
              target="_blank"
              rel="noopener noreferrer"
              class="w-full flex items-center justify-between px-3 py-2 rounded-xl text-xs font-mono bg-gray-50 hover:bg-emerald-500/10 dark:bg-white/[0.03] dark:hover:bg-emerald-500/10 border border-black/[0.04] dark:border-white/[0.04] text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 transition-all group"
            >
              <span class="truncate">Original Source</span>
              <span class="group-hover:translate-x-0.5 group-hover:-translate-y-0.5 transition-transform text-[11px]">&nearr;</span>
            </a>
          </div>
        </div>

        <!-- Obsidian-Style Backlinks / Connected Notes Section -->
        <div class="flex flex-col bg-white dark:bg-[#0C0E14]">
          <div class="px-3.5 py-2 border-b border-black/[0.06] dark:border-white/[0.06] bg-gray-50/50 dark:bg-white/[0.01] flex items-center justify-between text-[10px] font-mono text-gray-400 uppercase tracking-wider font-semibold">
            <span>Linked Mentions</span>
            <span>{{ backlinks.length }}</span>
          </div>
          <div class="p-2 space-y-1">
            <div v-if="backlinks.length === 0" class="text-xs text-gray-400 font-mono py-4 text-center">
              No incoming backlinks
            </div>
            <router-link
              v-for="link in backlinks"
              :key="link.ID"
              :to="`/articles/${link.ID}`"
              class="flex items-center justify-between px-2.5 py-1.5 rounded-lg text-xs text-gray-700 dark:text-gray-300 hover:text-emerald-600 dark:hover:text-emerald-400 hover:bg-gray-100 dark:hover:bg-white/[0.04] transition-all group"
            >
              <span class="truncate pr-2 font-medium">{{ link.title }}</span>
              <span class="text-gray-400 text-[10px] group-hover:translate-x-0.5 transition-transform">&rarr;</span>
            </router-link>
          </div>
        </div>
      </div>

    </aside>

    <!-- Obsidian-Style Collapsed Edge Button (Desktop) -->
    <button
      v-show="!isGraphOpen"
      @click="toggleGraphSidebar"
      class="hidden lg:flex fixed right-4 top-4 z-30 items-center gap-1.5 p-2 rounded-xl bg-white/90 dark:bg-[#0C0E14]/90 backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.06] shadow-sm hover:border-emerald-500/40 text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 transition-all cursor-pointer group"
      title="Expand Local Graph (Obsidian Sidebar)"
    >
      <svg class="w-4 h-4" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect width="18" height="18" x="3" y="3" rx="2"/>
        <path d="M15 3v18"/>
        <path d="m11 15-3-3 3-3"/>
      </svg>
      <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse"></span>
    </button>

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
