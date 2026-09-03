import { reactive } from 'vue'

const savedTheme = typeof window !== 'undefined'
  ? (localStorage.getItem('readr_theme') || localStorage.getItem('theme'))
  : null

const savedViewMode = typeof window !== 'undefined'
  ? (localStorage.getItem('readr_viewMode') || 'card')
  : 'card'

if (typeof document !== 'undefined' && savedTheme) {
  if (savedTheme === 'dark') {
    document.documentElement.classList.add('dark')
    document.documentElement.classList.remove('light')
  } else {
    document.documentElement.classList.remove('dark')
    document.documentElement.classList.add('light')
  }
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
  theme: savedTheme || (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'),
  view_mode: (savedViewMode === 'list' || savedViewMode === 'ledger') ? 'list' : 'card',
  graph_context_expansion: true,
})

export const isSettingsLoaded = reactive({ value: false })

export function applyTheme(theme: string) {
  const isDark = theme === 'dark'
  const mode = isDark ? 'dark' : 'light'
  settings.theme = mode
  try {
    localStorage.setItem('readr_theme', mode)
    localStorage.setItem('theme', mode)
  } catch {}
  if (typeof document !== 'undefined') {
    if (isDark) {
      document.documentElement.classList.add('dark')
      document.documentElement.classList.remove('light')
    } else {
      document.documentElement.classList.remove('dark')
      document.documentElement.classList.add('light')
    }
  }
}

export async function toggleTheme() {
  const next = settings.theme === 'dark' ? 'light' : 'dark'
  applyTheme(next)
  try {
    await saveSettingsToServer()
  } catch (e) {
    console.warn('Failed to save theme to server:', e)
  }
}

export async function setViewMode(mode: 'card' | 'list') {
  settings.view_mode = mode
  try {
    localStorage.setItem('readr_viewMode', mode)
  } catch {}
  try {
    await saveSettingsToServer()
  } catch (e) {
    console.warn('Failed to save view mode to server:', e)
  }
}

export async function initSettings() {
  // Sync with cached theme immediately to prevent any flicker
  const cachedTheme = typeof window !== 'undefined'
    ? (localStorage.getItem('readr_theme') || localStorage.getItem('theme'))
    : null
  if (cachedTheme === 'dark' || cachedTheme === 'light') {
    applyTheme(cachedTheme)
  }

  const cachedViewMode = typeof window !== 'undefined'
    ? localStorage.getItem('readr_viewMode')
    : null
  if (cachedViewMode === 'card' || cachedViewMode === 'list') {
    settings.view_mode = cachedViewMode
  }

  try {
    const res = await fetch('/api/settings')
    if (res.ok) {
      const data = await res.json()
      Object.assign(settings, data)
      if (data.theme) {
        applyTheme(data.theme)
      }
      if (data.view_mode) {
        settings.view_mode = (data.view_mode === 'list' || data.view_mode === 'ledger') ? 'list' : 'card'
        try { localStorage.setItem('readr_viewMode', settings.view_mode) } catch {}
      }
    }
  } catch (e) {
    console.error('Failed to load settings:', e)
  } finally {
    isSettingsLoaded.value = true
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
}
