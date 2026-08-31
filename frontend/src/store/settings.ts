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
  const res = await fetch('/api/settings', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings)
  })
  if (!res.ok) {
    throw new Error('Network response was not ok')
  }
  document.documentElement.className = settings.theme
}
