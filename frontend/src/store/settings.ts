import { reactive } from 'vue'

const savedTheme = typeof window !== 'undefined' ? localStorage.getItem('readr_theme') : null
if (typeof document !== 'undefined' && savedTheme) {
  document.documentElement.className = savedTheme
}

export const settings = reactive({
  api_key: '',
  model: 'openai/gpt-4o-mini',
  agent_enricher: true,
  agent_linker: true,
  agent_summarizer: true,
  librarian_enabled: true,
  librarian_cron: '0 0 * * *',
  librarian_min_cluster_size: 5,
  theme: savedTheme || 'light',
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
    if (settings.theme) {
      try { localStorage.setItem('readr_theme', settings.theme) } catch {}
      document.documentElement.className = settings.theme
    }
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
  if (settings.theme) {
    try { localStorage.setItem('readr_theme', settings.theme) } catch {}
    document.documentElement.className = settings.theme
  }
}
