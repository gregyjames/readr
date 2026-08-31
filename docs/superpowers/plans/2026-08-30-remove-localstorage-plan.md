# Remove localStorage Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Strip out frontend `localStorage` entirely, relying on a Vue reactive singleton synced from a strictly authoritative backend `settings.json`.

**Architecture:** We will extend the backend's `ServerSettings` struct to support UI fields (Theme, ViewMode), strip out header-based credential parsing, and build a single `src/store/settings.ts` in the frontend that acts as a reactive global state for the entire Vue app.

**Tech Stack:** Go (Fiber), Vue 3 (Composition API, `reactive`)

---

### Task 1: Extend Backend ServerSettings

**Files:**
- Modify: `backend/main.go`

- [ ] **Step 1: Update ServerSettings Struct**
Add `Theme`, `ViewMode`, and `GraphContextExpansion` to `ServerSettings`.

```go
type ServerSettings struct {
	APIKey                string `json:"api_key"`
	Model                 string `json:"model"`
	AgentEnricher         bool   `json:"agent_enricher"`
	AgentLinker           bool   `json:"agent_linker"`
	AgentSummarizer       bool   `json:"agent_summarizer"`
	Theme                 string `json:"theme"`
	ViewMode              string `json:"view_mode"`
	GraphContextExpansion bool   `json:"graph_context_expansion"`
}
```

- [ ] **Step 2: Update Defaults in `loadServerSettings`**
Add the new UI defaults to the `defaults` struct inside `loadServerSettings`.

```go
	defaults := ServerSettings{
		Model:                 "openai/gpt-4o-mini",
		AgentEnricher:         true,
		AgentLinker:           true,
		AgentSummarizer:       true,
		Theme:                 "light",
		ViewMode:              "card",
		GraphContextExpansion: true,
	}
```

- [ ] **Step 3: Expose New Fields in `GET /api/settings`**
Ensure the JSON response maps these new fields.

```go
		return c.JSON(fiber.Map{
			"api_key":                 freshSettings.APIKey,
			"model":                   freshSettings.Model,
			"agent_enricher":          freshSettings.AgentEnricher,
			"agent_linker":            freshSettings.AgentLinker,
			"agent_summarizer":        freshSettings.AgentSummarizer,
			"theme":                   freshSettings.Theme,
			"view_mode":               freshSettings.ViewMode,
			"graph_context_expansion": freshSettings.GraphContextExpansion,
		})
```

- [ ] **Step 4: Commit**
```bash
git add backend/main.go
git commit -m "feat(backend): expand ServerSettings to include UI state"
```

### Task 2: Remove Header Parsing from Backend

**Files:**
- Modify: `backend/main.go`
- Modify: `backend/internal/agents/pool.go`

- [ ] **Step 1: Simplify `extractOpenRouterCredentials` in `main.go`**
Replace all the `c.Get("X-Openrouter...")` and `cleanAPIKey` logic with a direct read from memory.

```go
	extractOpenRouterCredentials := func(c *fiber.Ctx) (string, string) {
		settingsMu.RLock()
		defer settingsMu.RUnlock()
		return serverSettings.APIKey, serverSettings.Model
	}
```

- [ ] **Step 2: Simplify `resolveCredentials` in `pool.go`**
Remove fallback from payload, strictly read from `settings.json`.

```go
func (p *AgentPool) resolveCredentials(job Job) (string, string) {
	// Strictly read from settings.json
	var apiKey, model string
	candidates := []string{
		filepath.Join(p.dataDirectory, "settings.json"),
		"data/settings.json",
		"../data/settings.json",
	}
	for _, cp := range candidates {
		if data, err := os.ReadFile(cp); err == nil {
			var s struct {
				APIKey string `json:"api_key"`
				Model  string `json:"model"`
			}
			if json.Unmarshal(data, &s) == nil {
				if s.APIKey != "" {
					apiKey = cleanAPIKey(s.APIKey)
				}
				if s.Model != "" {
					model = strings.TrimSpace(s.Model)
				}
				if apiKey != "" && model != "" {
					break
				}
			}
		}
	}
	
	if apiKey == "" {
		apiKey = cleanAPIKey(os.Getenv("OPENROUTER_API_KEY"))
	}
	if model == "" {
		model = "openai/gpt-4o-mini"
	}

	return apiKey, model
}
```

- [ ] **Step 3: Commit**
```bash
git add backend/main.go backend/internal/agents/pool.go
git commit -m "refactor(backend): strictly use settings.json credentials, dropping header support"
```

### Task 3: Create Frontend Reactive Store

**Files:**
- Create: `frontend/src/store/settings.ts`
- Delete: `frontend/src/utils/settings.ts`

- [ ] **Step 1: Create the Store**
Create `frontend/src/store/settings.ts`.

```typescript
import { reactive } from 'vue'

export const settings = reactive({
  api_key: '',
  model: 'openai/gpt-4o-mini',
  agent_enricher: true,
  agent_linker: true,
  agent_summarizer: true,
  theme: 'light',
  view_mode: 'card',
  graph_context_expansion: true,
})

export const isSettingsLoaded = reactive({ value: false })

export async function initSettings() {
  try {
    const res = await fetch('/api/settings')
    if (res.ok) {
      const data = await res.json()
      Object.assign(settings, data)
    }
  } catch (e) {
    console.error('Failed to load settings:', e)
  } finally {
    isSettingsLoaded.value = true
    document.documentElement.className = settings.theme
  }
}

export async function saveSettingsToServer() {
  try {
    await fetch('/api/settings', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings)
    })
    document.documentElement.className = settings.theme
  } catch (e) {
    console.error('Failed to save settings:', e)
  }
}
```

- [ ] **Step 2: Delete Old Utility**
```bash
rm frontend/src/utils/settings.ts
```

- [ ] **Step 3: Commit**
```bash
git add frontend/src/store/settings.ts frontend/src/utils/settings.ts
git commit -m "feat(frontend): add reactive settings store and delete old util"
```

### Task 4: Initialize Store in App.vue

**Files:**
- Modify: `frontend/src/App.vue`

- [ ] **Step 1: Initialize Settings on Mount**
Remove old `localStorage` logic and call `initSettings()`. Wrap router-view in `v-if="isSettingsLoaded.value"`.

```vue
<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { settings, isSettingsLoaded, initSettings } from './store/settings';

// ... other imports ...

onMounted(async () => {
  await initSettings();
  // Keep SSE connection setup here ...
});

const submitForm = async () => {
  // Remove header injection for X-Openrouter-Key and X-Openrouter-Model
  // ...
  const res = await fetch('/api/add', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Agent-Enricher': settings.agent_enricher.toString(),
      'X-Agent-Linker': settings.agent_linker.toString(),
      'X-Agent-Summarizer': settings.agent_summarizer.toString(),
    },
    // ...
```

- [ ] **Step 2: Wait for Settings to Load**
Wrap `<router-view>` in template:

```html
<main class="flex-grow flex relative">
  <div v-if="!isSettingsLoaded.value" class="flex-grow flex items-center justify-center text-gray-500">
    Loading settings...
  </div>
  <router-view v-else />
</main>
```

- [ ] **Step 3: Commit**
```bash
git add frontend/src/App.vue
git commit -m "refactor(frontend): init global store on mount and strip headers"
```

### Task 5: Refactor SettingsView

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

- [ ] **Step 1: Bind directly to store**
Import `settings` and `saveSettingsToServer`. Replace all `localStorage` bindings with direct mutations of `settings`.
Remove `onMounted` logic that fetches `GET /api/settings` (it's already done in `App.vue`).

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { settings, saveSettingsToServer } from '../store/settings'

const models = ref<ModelItem[]>([])
// ...
const saveSettings = async () => {
  await saveSettingsToServer()
  showSavedMessage('Settings saved successfully!')
}

const clearKey = async () => {
  settings.api_key = ''
  await saveSettingsToServer()
  showSavedMessage('API key cleared.')
}

// Map toggle functions to settings.theme and settings.view_mode directly
const toggleTheme = (mode: string) => {
  settings.theme = mode
  saveSettingsToServer()
}
const toggleViewMode = (mode: string) => {
  settings.view_mode = mode
  saveSettingsToServer()
}
</script>
<template>
  <!-- Replace v-model="apiKey" with v-model="settings.api_key" -->
  <!-- Replace v-model="selectedModel" with v-model="settings.model" -->
  <!-- Replace v-model="enableAgentLinker" with v-model="settings.agent_linker" -->
  <!-- Update buttons to check settings.theme === 'dark' -->
</template>
```

- [ ] **Step 2: Commit**
```bash
git add frontend/src/views/SettingsView.vue
git commit -m "refactor(frontend): bind settings view directly to global store"
```

### Task 6: Refactor ChatView & Article

**Files:**
- Modify: `frontend/src/views/ChatView.vue`
- Modify: `frontend/src/components/Article.vue`

- [ ] **Step 1: ChatView Update**
Read `settings.api_key` and `settings.model` instead of old utils. Remove `X-Openrouter...` headers from its `fetch`.

```vue
<script setup lang="ts">
import { settings } from '../store/settings'

// ...
const sendMessage = async () => {
  // Remove X-Openrouter-* headers from fetch('/api/chat')
}
</script>
```

- [ ] **Step 2: Article Component Update**
Remove old utils imports. Use `settings` for agent toggles. Remove headers from `fetch('/api/summarize')`.

- [ ] **Step 3: Commit**
```bash
git add frontend/src/views/ChatView.vue frontend/src/components/Article.vue
git commit -m "refactor(frontend): bind remaining views to global store and drop headers"
```

