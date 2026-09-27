<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import DOMPurify from 'dompurify'
import { feedsAPI, ingestAPI, type RssFeed, type TimelineItem } from '../services/feeds'

const feeds = ref<RssFeed[]>([])
const timeline = ref<TimelineItem[]>([])
const selectedFeedId = ref<number | null>(null)

type ViewMode = 'card' | 'list'
const viewMode = ref<ViewMode>('card')

const isLoadingFeeds = ref(false)
const isLoadingTimeline = ref(false)

const newFeedUrl = ref('')
const isAddingFeed = ref(false)
const addError = ref<string | null>(null)
const removingFeedId = ref<number | null>(null)

const savingUrls = ref<Record<string, boolean>>({})
const savedUrls = ref<Record<string, boolean>>({})

const toastMessage = ref<{ type: 'success' | 'error'; text: string } | null>(null)
let toastTimeout: ReturnType<typeof setTimeout> | null = null

// DOMPurify hook to ensure all anchor links open safely in a new tab with noopener
const sanitizeHook = (node: Element) => {
  if (node.tagName === 'A') {
    node.setAttribute('target', '_blank')
    node.setAttribute('rel', 'noopener noreferrer')
  }
}

DOMPurify.addHook('afterSanitizeAttributes', sanitizeHook)

const showToast = (text: string, type: 'success' | 'error' = 'success') => {
  if (toastTimeout) clearTimeout(toastTimeout)
  toastMessage.value = { type, text }
  toastTimeout = setTimeout(() => {
    toastMessage.value = null
    toastTimeout = null
  }, 4000)
}

const selectedFeed = computed(() => {
  if (selectedFeedId.value === null) return null
  return feeds.value.find(f => f.id === selectedFeedId.value) || null
})

// Subdued, elegant procedural gradients matching Home.vue (without loud glows)
const getProceduralGradient = (id: number) => {
  const gradients = [
    'from-emerald-950/40 via-[#121620] to-slate-950',
    'from-slate-900/60 via-[#121620] to-zinc-950',
    'from-teal-950/40 via-[#121620] to-slate-950',
    'from-cyan-950/30 via-[#121620] to-neutral-950',
    'from-indigo-950/40 via-[#121620] to-slate-950',
  ]
  return gradients[Math.abs(Number(id) || 0) % gradients.length]
}

const fetchFeeds = async () => {
  isLoadingFeeds.value = true
  try {
    feeds.value = await feedsAPI.getFeeds()
  } catch (err: any) {
    showToast(err.message || 'Failed to load feeds', 'error')
  } finally {
    isLoadingFeeds.value = false
  }
}

const fetchTimeline = async (feedId: number | null = selectedFeedId.value) => {
  isLoadingTimeline.value = true
  try {
    const data = await feedsAPI.getTimeline(feedId ?? undefined)
    // Race condition guard: ignore if user has switched to another feed in the meantime
    if (selectedFeedId.value !== feedId) {
      return
    }
    timeline.value = data
  } catch (err: any) {
    if (selectedFeedId.value !== feedId) {
      return
    }
    showToast(err.message || 'Failed to load timeline', 'error')
  } finally {
    if (selectedFeedId.value === feedId) {
      isLoadingTimeline.value = false
    }
  }
}

const selectFeed = async (feedId: number | null) => {
  selectedFeedId.value = feedId
  await fetchTimeline(feedId)
}

const handleAddFeed = async () => {
  const url = newFeedUrl.value.trim()
  if (!url) return

  isAddingFeed.value = true
  addError.value = null
  try {
    const created = await feedsAPI.addFeed(url)
    newFeedUrl.value = ''
    await fetchFeeds()
    showToast(`Subscribed to "${created.title || created.url}"`, 'success')
    await fetchTimeline(selectedFeedId.value)
  } catch (err: any) {
    addError.value = err.message || 'Failed to add feed'
  } finally {
    isAddingFeed.value = false
  }
}

const handleRemoveFeed = async (feed: RssFeed, e?: Event) => {
  e?.stopPropagation()
  removingFeedId.value = feed.id
  try {
    await feedsAPI.removeFeed(feed.id)
    feeds.value = feeds.value.filter(f => f.id !== feed.id)
    if (selectedFeedId.value === feed.id) {
      selectedFeedId.value = null
    }
    showToast(`Unsubscribed from "${feed.title || feed.url}"`, 'success')
    await fetchTimeline(selectedFeedId.value)
  } catch (err: any) {
    showToast(err.message || 'Failed to remove feed', 'error')
  } finally {
    removingFeedId.value = null
  }
}

const handleSaveToVault = async (item: TimelineItem) => {
  if (savingUrls.value[item.url] || savedUrls.value[item.url]) return

  savingUrls.value[item.url] = true
  try {
    await ingestAPI.ingestUrl(item.url)
    savedUrls.value[item.url] = true
    showToast(`Saved "${item.title}" to vault`, 'success')
  } catch (err: any) {
    showToast(err.message || 'Failed to save to vault', 'error')
  } finally {
    savingUrls.value[item.url] = false
  }
}

const sanitizeDescription = (html: string) => {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: ['b', 'i', 'em', 'strong', 'a', 'p', 'br', 'span', 'code'],
    ALLOWED_ATTR: ['href', 'target', 'rel', 'class'],
  })
}

const formatDate = (isoString: string) => {
  try {
    const d = new Date(isoString)
    if (isNaN(d.getTime())) return ''
    return d.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: d.getFullYear() !== new Date().getFullYear() ? 'numeric' : undefined,
    })
  } catch {
    return ''
  }
}

const extractHostname = (url: string) => {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return ''
  }
}

const leadItem = computed(() => {
  if (viewMode.value !== 'card' || timeline.value.length === 0) return null
  return timeline.value[0]
})

const secondaryItems = computed(() => {
  if (viewMode.value !== 'card') return timeline.value
  return timeline.value.slice(1)
})

onMounted(async () => {
  await Promise.all([fetchFeeds(), fetchTimeline()])
})

onBeforeUnmount(() => {
  if (toastTimeout) {
    clearTimeout(toastTimeout)
    toastTimeout = null
  }
  DOMPurify.removeHook('afterSanitizeAttributes')
})
</script>

<template>
  <div class="p-4 sm:p-8 w-full max-w-7xl mx-auto min-h-[100dvh] flex flex-col space-y-8">
    
    <!-- Floating Toast Notification (Restrained Glass) -->
    <transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="transform -translate-y-2 opacity-0 scale-95"
      enter-to-class="transform translate-y-0 opacity-100 scale-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="transform translate-y-0 opacity-100 scale-100"
      leave-to-class="transform -translate-y-2 opacity-0 scale-95"
    >
      <div
        v-if="toastMessage"
        role="status"
        aria-live="polite"
        class="fixed top-6 right-6 z-50 flex items-center gap-3 px-4 py-2.5 rounded-xl shadow-xl backdrop-blur-md border text-xs font-medium"
        :class="toastMessage.type === 'error'
          ? 'bg-red-500/10 text-red-700 dark:text-red-300 border-red-500/20'
          : 'bg-emerald-500/10 text-emerald-800 dark:text-emerald-300 border-emerald-500/20'"
      >
        <span class="w-1.5 h-1.5 rounded-full" :class="toastMessage.type === 'error' ? 'bg-red-500' : 'bg-emerald-500'"></span>
        <span>{{ toastMessage.text }}</span>
        <button @click="toastMessage = null" class="opacity-40 hover:opacity-100 ml-1 cursor-pointer" aria-label="Dismiss notification">✕</button>
      </div>
    </transition>

    <!-- Header Section (Apple TV Clean Aesthetic) -->
    <header class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2 mb-1">
          <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
          <span class="text-xs font-mono uppercase tracking-wider text-gray-400 dark:text-gray-500">RSS Dispatch</span>
        </div>
        <h1 class="text-2xl sm:text-3xl font-bold tracking-tight text-gray-900 dark:text-gray-100 font-['Outfit']">
          {{ selectedFeed ? selectedFeed.title : 'Feeds' }}
        </h1>
      </div>

      <!-- Controls: View Mode & Refresh -->
      <div class="flex items-center gap-2 self-start sm:self-auto">
        <!-- View Switcher -->
        <div class="flex items-center bg-gray-100/70 dark:bg-white/[0.04] p-0.5 rounded-lg border border-gray-200/50 dark:border-white/[0.05]">
          <button
            @click="viewMode = 'card'"
            class="p-1.5 rounded-md transition-all cursor-pointer"
            :class="viewMode === 'card' ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'"
            title="Card View"
            aria-label="Card View"
          >
            <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect width="7" height="7" x="3" y="3" rx="1" />
              <rect width="7" height="7" x="14" y="3" rx="1" />
              <rect width="7" height="7" x="14" y="14" rx="1" />
              <rect width="7" height="7" x="3" y="14" rx="1" />
            </svg>
          </button>
          <button
            @click="viewMode = 'list'"
            class="p-1.5 rounded-md transition-all cursor-pointer"
            :class="viewMode === 'list' ? 'bg-white dark:bg-white/10 text-gray-900 dark:text-white shadow-2xs' : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'"
            title="Stream View"
            aria-label="Stream View"
          >
            <svg class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="6" y1="3" x2="6" y2="21" />
              <circle cx="6" cy="8" r="2" fill="currentColor" />
              <circle cx="6" cy="16" r="2" fill="currentColor" />
              <line x1="12" y1="8" x2="20" y2="8" />
              <line x1="12" y1="16" x2="18" y2="16" />
            </svg>
          </button>
        </div>

        <!-- Refresh Button -->
        <button
          @click="fetchTimeline(selectedFeedId)"
          :disabled="isLoadingTimeline"
          title="Refresh Timeline"
          aria-label="Refresh Timeline"
          class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white bg-gray-100/70 dark:bg-white/[0.04] border border-gray-200/50 dark:border-white/[0.05] hover:bg-gray-200/70 dark:hover:bg-white/[0.08] transition-all cursor-pointer disabled:opacity-50"
        >
          <svg
            class="w-3.5 h-3.5"
            :class="{ 'animate-spin': isLoadingTimeline }"
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
            <path d="M3 3v5h5" />
            <path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16" />
            <path d="M16 21h5v-5" />
          </svg>
          <span class="hidden sm:inline">Refresh</span>
        </button>
      </div>
    </header>

    <!-- Apple TV Horizontal Channel Shelf (Segmented Pills & Add Form) -->
    <section class="space-y-3">
      <div class="flex items-center justify-between text-xs font-mono text-gray-400 dark:text-gray-500">
        <span>CHANNELS</span>
        <span>{{ feeds.length }} SUBSCRIBED</span>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <!-- "All Feeds" Pill -->
        <div
          data-testid="all-feeds-btn"
          role="button"
          tabindex="0"
          @click="selectFeed(null)"
          @keydown.enter.prevent="selectFeed(null)"
          @keydown.space.prevent="selectFeed(null)"
          class="group inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full text-xs font-medium transition-all cursor-pointer border"
          :class="selectedFeedId === null
            ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900 border-transparent shadow-xs'
            : 'bg-gray-100/70 dark:bg-white/[0.04] text-gray-600 dark:text-gray-300 border-gray-200/60 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20'"
        >
          <span>All Feeds</span>
          <span
            class="text-[10px] font-mono px-1.5 py-0.2 rounded-full"
            :class="selectedFeedId === null ? 'bg-white/20 dark:bg-black/10' : 'bg-gray-200/60 dark:bg-white/10 text-gray-500 dark:text-gray-400'"
          >
            {{ timeline.length }}
          </span>
        </div>

        <!-- Feed Channel Pills -->
        <div
          v-for="feed in feeds"
          :key="feed.id"
          :data-testid="`feed-item-${feed.id}`"
          role="button"
          tabindex="0"
          @click="selectFeed(feed.id)"
          @keydown.enter.prevent="selectFeed(feed.id)"
          @keydown.space.prevent="selectFeed(feed.id)"
          class="group inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full text-xs font-medium transition-all cursor-pointer border"
          :class="selectedFeedId === feed.id
            ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900 border-transparent shadow-xs'
            : 'bg-gray-100/70 dark:bg-white/[0.04] text-gray-600 dark:text-gray-300 border-gray-200/60 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20'"
        >
          <span class="truncate max-w-[150px]">{{ feed.title || feed.url }}</span>
          
          <!-- Delete button inside channel pill -->
          <button
            :data-testid="`remove-feed-${feed.id}`"
            @click.stop="handleRemoveFeed(feed, $event)"
            :disabled="removingFeedId === feed.id"
            title="Unsubscribe feed"
            aria-label="Remove feed"
            class="opacity-40 group-hover:opacity-100 hover:text-red-500 transition-opacity p-0.5 rounded cursor-pointer"
          >
            <span v-if="removingFeedId !== feed.id">✕</span>
            <span v-else class="inline-block animate-spin text-[10px]">⟳</span>
          </button>
        </div>

        <!-- Inline Quick Add Feed Form -->
        <form @submit.prevent="handleAddFeed" class="inline-flex items-center">
          <div class="relative flex items-center">
            <input
              v-model="newFeedUrl"
              type="url"
              placeholder="+ Add RSS / Atom URL..."
              :disabled="isAddingFeed"
              aria-label="Feed URL"
              class="text-xs px-3 py-1.5 pr-7 rounded-full border border-gray-200/70 dark:border-white/[0.08] bg-white/60 dark:bg-white/[0.03] text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:border-emerald-500 dark:focus:border-emerald-500/50 w-44 sm:w-56 focus:w-64 transition-all"
            />
            <button
              type="submit"
              :disabled="isAddingFeed || !newFeedUrl.trim()"
              class="absolute right-1.5 p-1 rounded-full text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 disabled:opacity-20 cursor-pointer transition-colors"
              title="Add feed"
              aria-label="Subscribe"
            >
              <svg v-if="!isAddingFeed" class="w-3 h-3" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <line x1="12" y1="5" x2="12" y2="19" />
                <line x1="5" y1="12" x2="19" y2="12" />
              </svg>
              <span v-else class="inline-block w-3 h-3 animate-spin text-[10px]">⟳</span>
            </button>
          </div>
        </form>
      </div>

      <!-- Add Feed Error Banner -->
      <div v-if="addError" class="text-xs text-red-500 dark:text-red-400 flex items-center gap-1.5 px-2">
        <span class="w-1.5 h-1.5 rounded-full bg-red-500"></span>
        <span>{{ addError }}</span>
      </div>

      <!-- Empty Feeds Prompt -->
      <div v-if="!isLoadingFeeds && feeds.length === 0" class="text-xs text-gray-400 dark:text-gray-500 font-mono py-1 px-1">
        No subscribed feeds. Paste a feed URL into the field above to subscribe.
      </div>
    </section>

    <!-- Main Timeline Area -->
    <main class="space-y-8 flex-1">
      
      <!-- Loading State -->
      <div v-if="isLoadingTimeline" class="flex flex-col items-center justify-center py-28 text-gray-400">
        <svg class="w-6 h-6 animate-spin text-emerald-500 mb-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
        </svg>
        <span class="text-xs font-mono">Fetching latest stories...</span>
      </div>

      <!-- Empty State: Zero Subscriptions -->
      <div v-else-if="feeds.length === 0" class="flex flex-col items-center justify-center py-32 text-center px-4">
        <div class="w-12 h-12 rounded-2xl bg-gray-100 dark:bg-white/[0.04] border border-gray-200/80 dark:border-white/[0.08] text-gray-400 flex items-center justify-center mb-4">
          <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 11a9 9 0 0 1 9 9" />
            <path d="M4 4a16 16 0 0 1 16 16" />
            <circle cx="5" cy="19" r="1" />
          </svg>
        </div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">No feeds subscribed yet</h3>
        <p class="text-xs text-gray-500 dark:text-gray-400 max-w-sm mt-1 leading-relaxed font-mono">
          Enter an RSS or Atom feed URL above to start aggregating articles into your reading stream.
        </p>
      </div>

      <!-- Empty State: Subscribed but timeline empty -->
      <div v-else-if="timeline.length === 0" class="flex flex-col items-center justify-center py-32 text-center px-4">
        <div class="w-12 h-12 rounded-2xl bg-gray-100 dark:bg-white/[0.04] border border-gray-200/80 dark:border-white/[0.08] text-gray-400 flex items-center justify-center mb-4">
          <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10" />
            <line x1="8" y1="12" x2="16" y2="12" />
          </svg>
        </div>
        <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">Timeline is empty</h3>
        <p class="text-xs text-gray-500 dark:text-gray-400 max-w-sm mt-1 leading-relaxed font-mono">
          No articles were returned for this selection. Try clicking refresh or adding more feeds.
        </p>
      </div>

      <!-- CONTENT: CARD VIEW (Apple TV Bento Layout) -->
      <div v-else-if="viewMode === 'card'" class="space-y-8">
        
        <!-- Spotlight Lead Card (Widescreen Hero Tile) -->
        <article
          v-if="leadItem"
          data-testid="timeline-card"
          class="relative group bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 transition-all duration-300 overflow-hidden shadow-xs hover:shadow-md"
        >
          <div class="flex flex-col lg:flex-row items-stretch">
            
            <!-- Left Editorial Text Content -->
            <div class="flex-1 p-6 sm:p-8 flex flex-col justify-between">
              <div>
                <!-- Metadata Row -->
                <div class="flex flex-wrap items-center gap-2 mb-3">
                  <span class="inline-flex items-center text-[10px] font-mono uppercase tracking-wider px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/20 font-medium">
                    {{ leadItem.feedTitle || 'Feed' }}
                  </span>
                  <time v-if="leadItem.published" class="text-[11px] text-gray-400 dark:text-gray-500 font-mono">
                    {{ formatDate(leadItem.published) }}
                  </time>
                </div>

                <!-- Title -->
                <h2 class="text-xl sm:text-2xl font-bold tracking-tight text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors mb-3 leading-snug font-['Outfit']">
                  <a :href="leadItem.url" target="_blank" rel="noopener noreferrer">
                    {{ leadItem.title }}
                  </a>
                </h2>

                <!-- Excerpt -->
                <div
                  v-if="leadItem.description"
                  v-html="sanitizeDescription(leadItem.description)"
                  class="text-xs sm:text-sm text-gray-600 dark:text-gray-400 line-clamp-3 leading-relaxed max-w-[65ch] break-words"
                ></div>
              </div>

              <!-- Action Bar -->
              <div class="flex items-center justify-between pt-6 mt-6 border-t border-gray-100 dark:border-white/[0.04]">
                <a
                  :href="leadItem.url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="inline-flex items-center gap-1.5 text-xs font-mono font-medium text-emerald-600 dark:text-emerald-400 group-hover:translate-x-0.5 transition-transform"
                >
                  <span>Read source</span>
                  <span>&rarr;</span>
                </a>

                <!-- Tactile Apple-Style Save Button -->
                <button
                  data-testid="save-to-vault-btn"
                  @click="handleSaveToVault(leadItem)"
                  :disabled="savingUrls[leadItem.url] || savedUrls[leadItem.url]"
                  class="px-4 py-1.5 rounded-lg text-xs font-medium transition-all active:scale-95 cursor-pointer shadow-xs"
                  :class="savedUrls[leadItem.url]
                    ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30'
                    : 'bg-gray-900 dark:bg-white text-white dark:text-gray-900 hover:opacity-90 disabled:opacity-60'"
                >
                  <template v-if="savingUrls[leadItem.url]">
                    <span class="inline-flex items-center gap-1.5">
                      <span class="w-2.5 h-2.5 rounded-full border-2 border-current border-t-transparent animate-spin"></span>
                      <span>Saving...</span>
                    </span>
                  </template>
                  <template v-else-if="savedUrls[leadItem.url]">
                    <span class="inline-flex items-center gap-1">
                      <span>✓</span>
                      <span>Saved!</span>
                    </span>
                  </template>
                  <template v-else>
                    <span>Save to Vault</span>
                  </template>
                </button>
              </div>
            </div>

            <!-- Right Spotlight Media Tile (Subtle Apple TV ambient depth) -->
            <div
              class="lg:w-80 h-36 lg:h-auto overflow-hidden p-6 flex flex-col justify-between border-t lg:border-t-0 lg:border-l border-gray-100 dark:border-white/[0.06] flex-shrink-0 relative bg-gradient-to-br"
              :class="getProceduralGradient(1)"
            >
              <div class="absolute inset-0 opacity-10 bg-[radial-gradient(#fff_1px,transparent_1px)] [background-size:14px_14px]"></div>
              <div class="relative z-10 flex justify-end">
                <span class="px-2 py-0.5 rounded-full text-[10px] font-mono text-white/70 bg-white/10 backdrop-blur-md border border-white/10">
                  {{ extractHostname(leadItem.url) || 'Latest Story' }}
                </span>
              </div>
              <div class="relative z-10 font-mono text-3xl font-black text-white/10 uppercase select-none">
                #TOP
              </div>
            </div>

          </div>
        </article>

        <!-- Secondary Grid (Bento Cards) -->
        <div v-if="secondaryItems.length > 0" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          <article
            v-for="item in secondaryItems"
            :key="item.url"
            data-testid="timeline-card"
            class="group relative bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 transition-all duration-300 shadow-2xs hover:shadow-md overflow-hidden flex flex-col justify-between p-6 space-y-4"
          >
            <div>
              <!-- Meta header -->
              <div class="flex items-center justify-between gap-2 mb-3">
                <span class="inline-flex items-center text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/20 font-medium">
                  {{ item.feedTitle || 'Feed' }}
                </span>
                <time v-if="item.published" class="text-[11px] text-gray-400 dark:text-gray-500 font-mono">
                  {{ formatDate(item.published) }}
                </time>
              </div>

              <!-- Title -->
              <h3 class="text-base sm:text-lg font-bold text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors leading-snug mb-2 font-['Outfit']">
                <a :href="item.url" target="_blank" rel="noopener noreferrer">
                  {{ item.title }}
                </a>
              </h3>

              <!-- Sanitized Excerpt -->
              <div
                v-if="item.description"
                v-html="sanitizeDescription(item.description)"
                class="text-xs text-gray-600 dark:text-gray-400 line-clamp-3 leading-relaxed break-words"
              ></div>
            </div>

            <!-- Action Bar -->
            <div class="pt-4 flex items-center justify-between border-t border-gray-100 dark:border-white/[0.04]">
              <a
                :href="item.url"
                target="_blank"
                rel="noopener noreferrer"
                class="text-xs font-mono text-gray-400 hover:text-emerald-500 transition-colors inline-flex items-center gap-1"
              >
                <span>{{ extractHostname(item.url) || 'Source' }}</span>
                <span>↗</span>
              </a>

              <!-- Tactile Save Button -->
              <button
                data-testid="save-to-vault-btn"
                @click="handleSaveToVault(item)"
                :disabled="savingUrls[item.url] || savedUrls[item.url]"
                class="px-3 py-1.5 rounded-lg text-xs font-medium transition-all active:scale-95 cursor-pointer shadow-xs"
                :class="savedUrls[item.url]
                  ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30'
                  : 'bg-gray-900 dark:bg-white text-white dark:text-gray-900 hover:opacity-90 disabled:opacity-60'"
              >
                <template v-if="savingUrls[item.url]">
                  <span class="inline-flex items-center gap-1">
                    <span class="w-2.5 h-2.5 rounded-full border-2 border-current border-t-transparent animate-spin"></span>
                    <span>Saving...</span>
                  </span>
                </template>
                <template v-else-if="savedUrls[item.url]">
                  <span>Saved!</span>
                </template>
                <template v-else>
                  <span>Save to Vault</span>
                </template>
              </button>
            </div>
          </article>
        </div>

      </div>

      <!-- CONTENT: STREAM / LIST VIEW (Linear Stream) -->
      <div v-else class="space-y-4 max-w-4xl">
        <article
          v-for="item in timeline"
          :key="item.url"
          data-testid="timeline-card"
          class="group bg-white dark:bg-[#12151C] rounded-xl border border-gray-200/80 dark:border-white/[0.08] hover:border-gray-300 dark:hover:border-white/20 p-5 transition-all shadow-2xs hover:shadow-xs flex flex-col sm:flex-row sm:items-center justify-between gap-4"
        >
          <div class="space-y-1.5 flex-1 min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 border border-emerald-500/20 font-medium">
                {{ item.feedTitle || 'Feed' }}
              </span>
              <time v-if="item.published" class="text-[11px] text-gray-400 dark:text-gray-500 font-mono">
                {{ formatDate(item.published) }}
              </time>
            </div>

            <h3 class="text-base font-bold text-gray-900 dark:text-gray-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors leading-snug font-['Outfit'] truncate">
              <a :href="item.url" target="_blank" rel="noopener noreferrer">
                {{ item.title }}
              </a>
            </h3>

            <div
              v-if="item.description"
              v-html="sanitizeDescription(item.description)"
              class="text-xs text-gray-600 dark:text-gray-400 line-clamp-1 leading-relaxed break-words"
            ></div>
          </div>

          <div class="shrink-0 flex items-center gap-3">
            <a
              :href="item.url"
              target="_blank"
              rel="noopener noreferrer"
              class="text-xs font-mono text-gray-400 hover:text-emerald-500 transition-colors hidden md:inline"
            >
              {{ extractHostname(item.url) }} ↗
            </a>

            <button
              data-testid="save-to-vault-btn"
              @click="handleSaveToVault(item)"
              :disabled="savingUrls[item.url] || savedUrls[item.url]"
              class="px-3.5 py-1.5 rounded-lg text-xs font-medium transition-all active:scale-95 cursor-pointer shadow-xs shrink-0"
              :class="savedUrls[item.url]
                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30'
                : 'bg-gray-900 dark:bg-white text-white dark:text-gray-900 hover:opacity-90 disabled:opacity-60'"
            >
              <template v-if="savingUrls[item.url]">
                <span>Saving...</span>
              </template>
              <template v-else-if="savedUrls[item.url]">
                <span>Saved!</span>
              </template>
              <template v-else>
                <span>Save to Vault</span>
              </template>
            </button>
          </div>
        </article>
      </div>

    </main>

  </div>
</template>
