export function getOpenRouterApiKey(): string {
  return (
    localStorage.getItem('OPENROUTER_API_KEY') ||
    localStorage.getItem('openrouter_key') ||
    localStorage.getItem('OPENROUTER_KEY') ||
    localStorage.getItem('openrouter_api_key') ||
    localStorage.getItem('readr_openrouter_key') ||
    localStorage.getItem('apiKey') ||
    localStorage.getItem('api_key') ||
    ''
  ).trim()
}

export function getOpenRouterModel(): string {
  return (
    localStorage.getItem('OPENROUTER_MODEL') ||
    localStorage.getItem('openrouter_model') ||
    'openai/gpt-4o-mini'
  ).trim()
}

export function isAgentEnricherEnabled(): boolean {
  return localStorage.getItem('AGENT_ENRICHER') !== 'false' && localStorage.getItem('readr_agent_enricher') !== 'false'
}

export function isAgentLinkerEnabled(): boolean {
  return localStorage.getItem('AGENT_LINKER') !== 'false' && localStorage.getItem('readr_agent_linker') !== 'false'
}

export function isAgentSummarizerEnabled(): boolean {
  return localStorage.getItem('AGENT_SUMMARIZER') !== 'false' && localStorage.getItem('readr_agent_summarizer') !== 'false'
}

