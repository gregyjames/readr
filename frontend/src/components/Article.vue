<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, computed, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import axios from 'axios'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import { Network } from 'vis-network'

defineProps<{ id?: string }>()

const route = useRoute()

const getArticleId = () => String(route.params.id || '').replace('.md', '')
const router = useRouter()
const markdownContent = ref('')
const rawMarkdown = ref('')
const isEditing = ref(false)
const editContent = ref('')
const isSaving = ref(false)

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
    await loadContent(true)
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
    loadContent(true)
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
      if (n.group === 'article') {
        return { ...n, title: n.label, label: undefined }
      }
      return n
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
      interaction: { hover: true, tooltipDelay: 200 }
    }

    if (localNetwork) {
      localNetwork.setData({ nodes: localNodes, edges: connectedEdges })
      localNetwork.setOptions(options)
      localNetwork.once('afterDrawing', () => { localNetwork?.fit({ animation: { duration: 500 } }); });
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

const loadContent = async (forceRefresh = false) => {
  try {
    articleError.value = ''
    const articleID = getArticleId()
    if (!articleID) return

    const articleURL = `/articles/${articleID}.md`
    const fetchUrl = forceRefresh ? `${articleURL}?t=${Date.now()}` : articleURL

    const res = await axios.get(fetchUrl)
    const raw = String(res.data)
    rawMarkdown.value = raw
    
    const parsedRaw = raw.replace(/\[\[(.*?)\]\]/g, (_, p1) => {
      const parts = p1.split('|')
      const targetTitle = parts[0]
      const display = parts.length > 1 ? parts[1] : parts[0]
      
      const targetArticle = allArticles.value.find(a => a.title === targetTitle)
      const url = targetArticle ? `/articles/${targetArticle.ID}` : '#'
      
      return `<a href="${url}" class="wikilink font-semibold text-emerald-600 dark:text-emerald-400 no-underline hover:underline hover:text-emerald-700 dark:hover:text-emerald-300 transition-colors bg-emerald-50/50 dark:bg-emerald-900/20 px-1.5 py-0.5 rounded-md border border-emerald-100 dark:border-emerald-800/50">${display}</a>`
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

watch(() => route.params.id, (newId) => {
  if (newId) {
    loadContent()
    loadLocalGraph()
  }
})

const handleWikilinkClick = (e: MouseEvent) => {
  const target = (e.target as HTMLElement).closest('a');
  if (target) {
    const href = target.getAttribute('href');
    if (href && href.startsWith('/articles/')) {
      e.preventDefault();
      router.push(href);
    }
  }
}

onMounted(async () => {
  await fetchArticles()
  await loadContent()
  await loadLocalGraph()
  document.addEventListener('mouseup', handleSelection)
  document.addEventListener('mousedown', hideLinker)
  
  // Intercept wikilink clicks
  document.addEventListener('click', handleWikilinkClick)

  
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
  document.removeEventListener('click', handleWikilinkClick)
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
      
      
      <div class="flex justify-end mb-4">
        <button v-if="!isEditing" @click="startEditing" class="px-4 py-2 bg-gray-100 dark:bg-gray-800 text-gray-700 dark:text-gray-300 rounded-lg text-sm font-bold hover:bg-gray-200 dark:hover:bg-gray-700 transition-colors active:scale-95">Edit Markdown</button>
      </div>
      
      <div v-if="isEditing" class="mb-8 w-full animate-in fade-in slide-in-from-top-4 duration-300">
        <textarea v-model="editContent" rows="15" class="w-full p-6 bg-gray-50 dark:bg-[#111] border border-gray-200 dark:border-gray-800 rounded-2xl text-gray-900 dark:text-gray-100 font-mono text-sm leading-relaxed focus:ring-4 focus:ring-emerald-500/20 focus:border-emerald-500 outline-none resize-y transition-all shadow-inner"></textarea>
        <div class="flex justify-end gap-3 mt-4">
          <button @click="cancelEditing" :disabled="isSaving" class="px-5 py-2.5 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 font-bold text-sm transition-colors disabled:opacity-50">Cancel</button>
          <button @click="saveEdit" :disabled="isSaving" class="px-6 py-2.5 bg-emerald-500 hover:bg-emerald-600 text-white font-bold text-sm rounded-xl transition-colors active:scale-95 shadow-sm disabled:opacity-50 flex items-center gap-2">
            <svg v-if="isSaving" class="animate-spin h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path></svg>
            {{ isSaving ? 'Saving...' : 'Save Changes' }}
          </button>
        </div>
      </div>

      <div v-else class="prose prose-lg md:prose-xl dark:prose-invert prose-p:text-gray-700 dark:prose-p:text-gray-300 prose-p:leading-relaxed prose-p:font-serif prose-headings:font-sans prose-headings:font-bold prose-headings:tracking-tight prose-headings:text-gray-900 dark:prose-headings:text-gray-50 prose-a:text-emerald-600 dark:prose-a:text-emerald-400 prose-a:no-underline hover:prose-a:underline prose-img:rounded-3xl prose-img:shadow-sm prose-pre:text-left prose-pre:bg-[#111] dark:prose-pre:bg-[#1a1a1a] prose-pre:rounded-[2rem] transition-colors duration-300" v-html="markdownContent" />

      
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
              <tr v-for="link in backlinks" :key="link.ID" class="group border-b last:border-0 border-gray-100 dark:border-white/5 hover:bg-gray-50/80 dark:hover:bg-white/5 transition-all duration-300 cursor-pointer active:scale-[0.99]" @click="router.push(`/articles/${link.ID}`)">
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

</template>
