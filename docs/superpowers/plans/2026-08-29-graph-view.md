# Graph View & Linking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an Obsidian-style linking system and interactive network graph for connected notes and tags.

**Architecture:** A new database model stores explicit article-to-article links, which are injected into the Markdown content dynamically. The frontend uses `vis-network` to render the force-directed graph (global and local) and adds an inline popover to create connections from text selections.

**Tech Stack:** Go (Fiber, GORM), Vue 3, TailwindCSS, `vis-network` (via `vis-network` directly or a Vue wrapper like `vue-network`).

---

### Task 1: Backend Data Model & `POST /api/link`

**Files:**
- Modify: `backend/main.go`

- [x] **Step 1: Write the failing test**

*(We will use a minimal HTTP test block in `main_test.go` or write it locally)*
```go
// Add to backend/main_test.go (assume basic test setup exists)
func TestCreateLink(t *testing.T) {
	app := setupApp()
	reqBody := `{"sourceId": 1, "targetId": 2, "selectedText": "neural networks"}`
	req := httptest.NewRequest("POST", "/api/link", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -run TestCreateLink`
Expected: FAIL with 404 or method not found.

- [x] **Step 3: Write minimal implementation**

```go
// In backend/main.go
type ArticleLink struct {
	ID       int64 `gorm:"primaryKey"`
	SourceID int64
	TargetID int64
}

type LinkRequest struct {
	SourceID     int64  `json:"sourceId"`
	TargetID     int64  `json:"targetId"`
	SelectedText string `json:"selectedText"`
}

// In main() after db.AutoMigrate(&Article{})
db.AutoMigrate(&ArticleLink{})

api.Post("/link", func(c *fiber.Ctx) error {
	var req LinkRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	// 1. Create DB link
	link := ArticleLink{SourceID: req.SourceID, TargetID: req.TargetID}
	if err := db.Create(&link).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Could not create link"})
	}

	// 2. Fetch target article to get its title
	var target Article
	if err := db.First(&target, req.TargetID).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "Target article not found"})
	}

	// 3. Update markdown file
	sourcePath := fmt.Sprintf("/app/data/articles/%d.md", req.SourceID)
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Could not read source article"})
	}

	// Simple string replacement for the first instance
	wikilink := fmt.Sprintf("[[%s|%s]]", target.Title, req.SelectedText)
	newContent := strings.Replace(string(content), req.SelectedText, wikilink, 1)

	if err := os.WriteFile(sourcePath, []byte(newContent), 0644); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Could not update markdown"})
	}

	return c.JSON(fiber.Map{"status": "success", "linkId": link.ID})
})
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -run TestCreateLink`
Expected: PASS (if mock db handles it, or test passes basic route check)

- [x] **Step 5: Commit**

```bash
git add backend/main.go backend/main_test.go
git commit -m "feat(backend): add ArticleLink model and POST /api/link endpoint"
```

### Task 2: Backend `GET /api/graph`

**Files:**
- Modify: `backend/main.go`

- [x] **Step 1: Write the failing test**

```go
// Add to backend/main_test.go
func TestGetGraph(t *testing.T) {
	app := setupApp()
	req := httptest.NewRequest("GET", "/api/graph", nil)
	resp, _ := app.Test(req)
	
	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}
```

- [x] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -run TestGetGraph`
Expected: FAIL

- [x] **Step 3: Write minimal implementation**

```go
// In backend/main.go

type GraphNode struct {
	Id    string `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"` // "article" or "tag"
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

api.Get("/graph", func(c *fiber.Ctx) error {
	var articles []Article
	if err := db.Find(&articles).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch articles"})
	}

	var links []ArticleLink
	if err := db.Find(&links).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Failed to fetch links"})
	}

	nodes := []GraphNode{}
	edges := []GraphEdge{}
	tagSet := make(map[string]bool)

	for _, article := range articles {
		nodes = append(nodes, GraphNode{
			Id:    fmt.Sprintf("article-%d", article.ID),
			Label: article.Title,
			Group: "article",
		})

		// Process tags
		if article.Tags != "" {
			tags := strings.Split(article.Tags, ",")
			for _, tag := range tags {
				tag = strings.TrimSpace(tag)
				if tag == "" {
					continue
				}
				if !tagSet[tag] {
					nodes = append(nodes, GraphNode{
						Id:    fmt.Sprintf("tag-%s", tag),
						Label: tag,
						Group: "tag",
					})
					tagSet[tag] = true
				}
				edges = append(edges, GraphEdge{
					From: fmt.Sprintf("article-%d", article.ID),
					To:   fmt.Sprintf("tag-%s", tag),
				})
			}
		}
	}

	for _, link := range links {
		edges = append(edges, GraphEdge{
			From: fmt.Sprintf("article-%d", link.SourceID),
			To:   fmt.Sprintf("article-%d", link.TargetID),
		})
	}

	return c.JSON(fiber.Map{
		"nodes": nodes,
		"edges": edges,
	})
})
```

- [x] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -run TestGetGraph`
Expected: PASS

- [x] **Step 5: Commit**

```bash
git add backend/main.go backend/main_test.go
git commit -m "feat(backend): add GET /api/graph endpoint returning nodes and edges"
```

### Task 3: Setup Graph UI Dependencies & Route

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/src/router.ts`
- Create: `frontend/src/components/Graph.vue`

- [x] **Step 1: Install vis-network**

Run: `cd frontend && bun add vis-network`

- [x] **Step 2: Create empty Graph.vue**

```vue
<!-- frontend/src/components/Graph.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'

const container = ref<HTMLElement | null>(null)

onMounted(() => {
  // placeholder
})
</script>
<template>
  <div class="w-full h-screen bg-gray-50 dark:bg-[#0a0a0a] pt-20" ref="container">
    <div class="p-8 text-black dark:text-white">Graph View Loading...</div>
  </div>
</template>
```

- [x] **Step 3: Update router and Nav**

```typescript
// In frontend/src/router.ts
import Graph from './components/Graph.vue'
// Add to routes array:
{ path: '/graph', name: 'graph', component: Graph },
```

```vue
<!-- In frontend/src/App.vue, around line 98 -->
<router-link to="/graph" class="text-gray-500 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 p-2 rounded-full hover:bg-gray-100 dark:hover:bg-gray-800/60 transition-colors">
  <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="18" cy="5" r="3"></circle><circle cx="6" cy="12" r="3"></circle><circle cx="18" cy="19" r="3"></circle><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"></line><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"></line></svg>
</router-link>
```

- [x] **Step 4: Verify navigation**
Run: `cd frontend && bun run build`

- [x] **Step 5: Commit**

```bash
git add frontend/package.json frontend/bun.lock frontend/src/router.ts frontend/src/App.vue frontend/src/components/Graph.vue
git commit -m "feat(frontend): setup Graph.vue route and vis-network dependency"
```

### Task 4: Implement Global Graph Rendering

**Files:**
- Modify: `frontend/src/components/Graph.vue`

- [x] **Step 1: Implement Graph fetching and rendering**

```vue
<!-- frontend/src/components/Graph.vue -->
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Network } from 'vis-network'

const container = ref<HTMLElement | null>(null)
const router = useRouter()
const showTags = ref(true)
let network: Network | null = null
let graphData = { nodes: [], edges: [] }

const loadGraph = async () => {
  const res = await fetch('/api/graph')
  graphData = await res.json()
  renderGraph()
}

const renderGraph = () => {
  if (!container.value) return

  const filteredNodes = showTags.value 
    ? graphData.nodes 
    : graphData.nodes.filter((n: any) => n.group === 'article')

  const filteredEdges = showTags.value
    ? graphData.edges
    : graphData.edges.filter((e: any) => !e.to.startsWith('tag-'))

  const isDark = document.documentElement.classList.contains('dark')
  
  const options = {
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
  }

  network = new Network(container.value, { nodes: filteredNodes, edges: filteredEdges }, options)

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

onMounted(() => {
  loadGraph()
})

const toggleTags = () => {
  showTags.value = !showTags.value
  renderGraph()
}
</script>

<template>
  <div class="relative w-full h-screen bg-gray-50 dark:bg-[#0a0a0a]">
    <div ref="container" class="absolute inset-0 pt-20"></div>
    <div class="absolute top-24 right-8 z-10">
      <button @click="toggleTags" class="bg-white dark:bg-[#111] px-4 py-2 rounded-full shadow border border-gray-200 dark:border-gray-800 text-sm font-medium">
        {{ showTags ? 'Hide Tags' : 'Show Tags' }}
      </button>
    </div>
  </div>
</template>
```

- [x] **Step 2: Check Build**
Run: `cd frontend && bun run build`

- [x] **Step 3: Commit**

```bash
git add frontend/src/components/Graph.vue
git commit -m "feat(frontend): implement global graph visualization with vis-network"
```

### Task 5: Inline Linking UI in `Article.vue`

**Files:**
- Modify: `frontend/src/components/Article.vue`

- [x] **Step 1: Add selection logic & popup**

```vue
<!-- Add to frontend/src/components/Article.vue script -->
<script setup lang="ts">
import { ref, onMounted, nextTick, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import axios from 'axios'

const markdownContent = ref('')
const route = useRoute()
const articleID = route.params.id

// Linker state
const showLinker = ref(false)
const linkerPos = ref({ top: 0, left: 0 })
const selectedText = ref('')
const searchInput = ref('')
const allArticles = ref<any[]>([])

const fetchArticles = async () => {
  const res = await axios.get('/api/getarticles')
  allArticles.value = res.data
}

const handleSelection = () => {
  const selection = window.getSelection()
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
    // Only hide if we aren't clicking inside the linker
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
      sourceId: Number(articleID),
      targetId: targetId,
      selectedText: selectedText.value
    })
    
    showLinker.value = false
    // Reload article content
    loadContent()
  } catch (err) {
    console.error('Link creation failed', err)
  }
}

const loadContent = async () => {
  const res = await fetch(`/articles/${articleID}`)
  const raw = await res.text()
  
  // Custom Wikilink renderer
  const renderer = new marked.Renderer()
  const defaultText = renderer.text.bind(renderer)
  
  // Actually, marked hooks or preprocessing is easier for wikilinks
  const parsedRaw = raw.replace(/\[\[(.*?)\]\]/g, (match, p1) => {
    const parts = p1.split('|')
    const targetTitle = parts[0]
    const display = parts.length > 1 ? parts[1] : parts[0]
    
    // Find target ID locally based on title
    const targetArticle = allArticles.value.find(a => a.title === targetTitle)
    const url = targetArticle ? `/${targetArticle.ID}` : '#'
    
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
}

onMounted(async () => {
  await fetchArticles()
  await loadContent()
  document.addEventListener('mouseup', handleSelection)
  document.addEventListener('mousedown', hideLinker)
})

onBeforeUnmount(() => {
  document.removeEventListener('mouseup', handleSelection)
  document.removeEventListener('mousedown', hideLinker)
})
</script>
```

- [x] **Step 2: Add linker template**

```vue
<!-- Add this at the bottom of the template in Article.vue -->
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
        v-for="article in allArticles.filter(a => a.title.toLowerCase().includes(searchInput.toLowerCase()) && a.ID !== Number(articleID))"
        :key="article.ID"
        @click="createLink(article.ID)"
        class="w-full text-left px-2 py-1.5 text-sm hover:bg-gray-100 dark:hover:bg-gray-800 rounded-lg text-gray-700 dark:text-gray-300 truncate"
      >
        {{ article.title }}
      </button>
      <div v-if="allArticles.filter(a => a.title.toLowerCase().includes(searchInput.toLowerCase()) && a.ID !== Number(articleID)).length === 0" class="text-xs text-gray-500 text-center py-2">
        No matching articles
      </div>
    </div>
  </div>
```

- [x] **Step 3: Check Build**
Run: `cd frontend && bun run build`

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/Article.vue
git commit -m "feat(frontend): implement inline highlighting UI for creating article links"
```

### Task 6: Local Graph View in `Article.vue`

**Files:**
- Modify: `frontend/src/components/Article.vue`

- [x] **Step 1: Embed graph container and logic**

```vue
<!-- Add to script in frontend/src/components/Article.vue -->
import { Network } from 'vis-network'

const localGraphContainer = ref<HTMLElement | null>(null)
let localNetwork: Network | null = null

const loadLocalGraph = async () => {
  if (!localGraphContainer.value) return
  
  const res = await fetch('/api/graph')
  const graphData = await res.json()
  
  // Filter for local neighborhood (1st degree)
  const currentArticleNodeId = `article-${articleID}`
  
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

  localNetwork = new Network(localGraphContainer.value, { nodes: localNodes, edges: connectedEdges }, options)
}

// Add to onMounted after loadContent()
await loadLocalGraph()
```

- [x] **Step 2: Add template for local graph**

```vue
<!-- Add to Article.vue template, wrapper around <article> -->
<template>
  <div class="flex flex-col lg:flex-row w-full max-w-7xl mx-auto items-start">
    <article class="w-full lg:w-2/3 max-w-2xl mx-auto py-10 transition-colors duration-300">
      <!-- existing prose div -->
    </article>
    
    <!-- Local Graph Sidebar -->
    <aside class="hidden lg:block w-1/3 sticky top-24 h-[400px] bg-gray-50 dark:bg-[#1a1a1a] rounded-3xl border border-gray-100 dark:border-gray-800 ml-8 overflow-hidden">
      <div class="px-6 py-4 border-b border-gray-100 dark:border-gray-800 font-semibold text-gray-900 dark:text-gray-100 text-sm">Local Graph</div>
      <div ref="localGraphContainer" class="w-full h-[340px]"></div>
    </aside>
  </div>
</template>
```

- [x] **Step 3: Check Build**
Run: `cd frontend && bun run build`

- [x] **Step 4: Commit**

```bash
git add frontend/src/components/Article.vue
git commit -m "feat(frontend): add local graph view sidebar to article page"
```
