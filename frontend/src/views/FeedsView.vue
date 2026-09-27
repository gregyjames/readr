<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import DOMPurify from 'dompurify'
import { feedsAPI, ingestAPI, type RssFeed, type TimelineItem } from '../services/feeds'

const feeds = ref<RssFeed[]>([])
const timeline = ref<TimelineItem[]>([])
const selectedFeedId = ref<number | null>(null)

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

const fetchTimeline = async (feedId: number | null = selectedFeedId.value, forceRefresh = false) => {
  isLoadingTimeline.value = true
  try {
    const data = await feedsAPI.getTimeline(feedId ?? undefined, forceRefresh)
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
  <div class="relative min-h-[100dvh] w-full max-w-5xl mx-auto p-4 sm:p-8 flex flex-col space-y-7">
    
    <!-- Subtle Ambient Vignette (Zero harsh neon glows) -->
    <div class="absolute -top-24 left-1/2 -translate-x-1/2 w-full max-w-3xl h-48 bg-emerald-500/[0.025] blur-3xl pointer-events-none rounded-full"></div>

    <!-- Floating Toast Notification (Restrained Frosted Glass) -->
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
        class="fixed top-6 right-6 z-50 flex items-center gap-3 px-4 py-2.5 rounded-xl shadow-xl backdrop-blur-xl border text-xs font-medium"
        :class="toastMessage.type === 'error'
          ? 'bg-red-500/10 text-red-700 dark:text-red-300 border-red-500/20'
          : 'bg-emerald-500/10 text-emerald-800 dark:text-emerald-300 border-emerald-500/20'"
      >
        <span class="w-1.5 h-1.5 rounded-full" :class="toastMessage.type === 'error' ? 'bg-red-500' : 'bg-emerald-500'"></span>
        <span>{{ toastMessage.text }}</span>
        <button @click="toastMessage = null" class="opacity-40 hover:opacity-100 ml-1 cursor-pointer" aria-label="Dismiss notification">✕</button>
      </div>
    </transition>

    <!-- Header Section (Tactile, High-Contrast Typography) -->
    <header class="flex items-end justify-between pb-4 border-b border-gray-200/60 dark:border-white/[0.06]">
      <div class="space-y-1">
        <div class="flex items-center gap-2">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>
          <span class="text-[11px] font-mono text-gray-400 dark:text-gray-500 tracking-wide uppercase">{{ timeline.length }} entries</span>
        </div>
        <h1 class="text-3xl sm:text-4xl font-extrabold tracking-tight text-gray-950 dark:text-[#F3F4F6] font-['Outfit']">
          {{ selectedFeed ? selectedFeed.title : 'Feeds' }}
        </h1>
      </div>

      <!-- Controls: Refresh Button with Tactile Feedback -->
      <button
        @click="fetchTimeline(selectedFeedId, true)"
        :disabled="isLoadingTimeline"
        title="Refresh Timeline"
        aria-label="Refresh Timeline"
        class="p-2.5 rounded-xl text-gray-400 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white bg-gray-100/80 dark:bg-white/[0.03] border border-gray-200/60 dark:border-white/[0.06] hover:bg-gray-200/80 dark:hover:bg-white/[0.08] active:scale-95 transition-all cursor-pointer disabled:opacity-40"
      >
        <svg
          class="w-4 h-4"
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
      </button>
    </header>

    <!-- Glassmorphic Source Shelf (Apple TV Floating Channel Tray) -->
    <section class="space-y-2">
      <div class="flex items-center gap-1.5 overflow-x-auto no-scrollbar py-1">
        
        <!-- "All Feeds" Pill -->
        <div
          data-testid="all-feeds-btn"
          role="button"
          tabindex="0"
          @click="selectFeed(null)"
          @keydown.enter.prevent="selectFeed(null)"
          @keydown.space.prevent="selectFeed(null)"
          class="group shrink-0 inline-flex items-center gap-2 px-3 py-1.5 rounded-xl text-xs font-medium transition-all cursor-pointer border"
          :class="selectedFeedId === null
            ? 'bg-gray-950 text-white dark:bg-white dark:text-gray-950 border-transparent shadow-xs font-semibold'
            : 'bg-gray-100/80 dark:bg-white/[0.03] text-gray-600 dark:text-gray-400 border-gray-200/60 dark:border-white/[0.06] hover:border-gray-300 dark:hover:border-white/15 hover:text-gray-900 dark:hover:text-gray-200'"
        >
          <span>All Feeds</span>
          <span
            class="text-[10px] font-mono px-1.5 py-0.5 rounded"
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
          class="group shrink-0 inline-flex items-center gap-2 px-3 py-1.5 rounded-xl text-xs font-medium transition-all cursor-pointer border"
          :class="selectedFeedId === feed.id
            ? 'bg-gray-950 text-white dark:bg-white dark:text-gray-950 border-transparent shadow-xs font-semibold'
            : 'bg-gray-100/80 dark:bg-white/[0.03] text-gray-600 dark:text-gray-400 border-gray-200/60 dark:border-white/[0.06] hover:border-gray-300 dark:hover:border-white/15 hover:text-gray-900 dark:hover:text-gray-200'"
        >
          <span class="truncate max-w-[140px]">{{ feed.title || feed.url }}</span>
          
          <!-- Delete button inside channel pill -->
          <button
            :data-testid="`remove-feed-${feed.id}`"
            @click.stop="handleRemoveFeed(feed, $event)"
            :disabled="removingFeedId === feed.id"
            title="Unsubscribe feed"
            aria-label="Remove feed"
            class="opacity-40 group-hover:opacity-100 hover:text-red-500 transition-opacity p-0.5 rounded cursor-pointer text-xs"
          >
            <span v-if="removingFeedId !== feed.id">✕</span>
            <span v-else class="inline-block animate-spin text-[10px]">⟳</span>
          </button>
        </div>

        <!-- Inline Subscribe Input Form -->
        <form @submit.prevent="handleAddFeed" class="shrink-0 inline-flex items-center">
          <div class="relative flex items-center">
            <input
              v-model="newFeedUrl"
              type="url"
              placeholder="+ Add feed URL..."
              :disabled="isAddingFeed"
              aria-label="Feed URL"
              class="text-xs px-3 py-1.5 pr-7 rounded-xl border border-dashed border-gray-300 dark:border-white/15 bg-transparent text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:border-emerald-500 dark:focus:border-emerald-400/60 w-36 sm:w-48 focus:w-60 transition-all"
            />
            <button
              type="submit"
              :disabled="isAddingFeed || !newFeedUrl.trim()"
              class="absolute right-1 p-0.5 rounded text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 disabled:opacity-20 cursor-pointer transition-colors"
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
      <div v-if="addError" class="text-xs text-red-500 dark:text-red-400 flex items-center gap-1.5 px-1">
        <span class="w-1.5 h-1.5 rounded-full bg-red-500"></span>
        <span>{{ addError }}</span>
      </div>

      <!-- Empty Subscriptions Prompt -->
      <div v-if="!isLoadingFeeds && feeds.length === 0" class="text-xs text-gray-400 dark:text-gray-500 font-mono py-1 px-1">
        No subscribed feeds. Paste a feed URL into the field above to subscribe.
      </div>
    </section>

    <!-- Main Dispatch Wire Area (Continuous Editorial Slat Stream) -->
    <main class="space-y-4 flex-1">
      
      <!-- Loading State -->
      <div v-if="isLoadingTimeline" class="flex flex-col items-center justify-center py-28 text-gray-400">
        <svg class="w-6 h-6 animate-spin text-emerald-500 mb-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
        </svg>
        <span class="text-xs font-mono">Loading entries...</span>
      </div>

      <!-- Empty State: Zero Subscriptions -->
      <div v-else-if="feeds.length === 0" class="flex flex-col items-center justify-center py-32 text-center px-4">
        <div class="w-12 h-12 rounded-2xl bg-gray-100/80 dark:bg-white/[0.03] border border-gray-200/80 dark:border-white/[0.08] text-gray-400 flex items-center justify-center mb-4">
          <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 11a9 9 0 0 1 9 9" />
            <path d="M4 4a16 16 0 0 1 16 16" />
            <circle cx="5" cy="19" r="1" />
          </svg>
        </div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">No feeds subscribed yet</h3>
        <p class="text-xs text-gray-500 dark:text-gray-400 max-w-sm mt-1 leading-relaxed font-mono">
          Enter an RSS or Atom feed URL above to start aggregating articles into your reading stream.
        </p>
      </div>

      <!-- Empty State: Subscribed but timeline empty -->
      <div v-else-if="timeline.length === 0" class="flex flex-col items-center justify-center py-32 text-center px-4">
        <div class="w-12 h-12 rounded-2xl bg-gray-100/80 dark:bg-white/[0.03] border border-gray-200/80 dark:border-white/[0.08] text-gray-400 flex items-center justify-center mb-4">
          <svg class="w-5 h-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="10" />
            <line x1="8" y1="12" x2="16" y2="12" />
          </svg>
        </div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">Timeline is empty</h3>
        <p class="text-xs text-gray-500 dark:text-gray-400 max-w-sm mt-1 leading-relaxed font-mono">
          No articles were returned for this selection. Try clicking refresh or adding more feeds.
        </p>
      </div>

      <!-- EDITORIAL SLAT STREAM (Hairline Dividers + Edge-to-Edge Hover Spotlight) -->
      <div v-else class="divide-y divide-gray-200/50 dark:divide-white/[0.05]">
        <article
          v-for="item in timeline"
          :key="item.url"
          data-testid="timeline-card"
          class="group -mx-4 px-4 py-5 sm:py-6 rounded-2xl hover:bg-gray-50/80 dark:hover:bg-white/[0.02] transition-all duration-150 flex flex-col md:flex-row md:items-start justify-between gap-5"
        >
          <!-- Left Anchor Rail: Time, Source & Domain (~160px desktop) -->
          <div class="md:w-44 shrink-0 flex flex-wrap md:flex-col items-center md:items-start gap-1.5 md:gap-1 pt-0.5">
            <time v-if="item.published" class="text-xs font-mono text-gray-400 dark:text-gray-500">
              {{ formatDate(item.published) }}
            </time>

            <span class="inline-flex items-center text-[10px] font-mono uppercase tracking-wider px-2 py-0.5 rounded-md bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20 font-medium">
              {{ item.feedTitle || 'Feed' }}
            </span>

            <span v-if="extractHostname(item.url)" class="text-[11px] font-mono text-gray-400 dark:text-gray-600 truncate max-w-[150px]">
              {{ extractHostname(item.url) }}
            </span>
          </div>

          <!-- Center Content: Headline & Excerpt -->
          <div class="flex-1 min-w-0 space-y-2">
            <h2 class="text-lg sm:text-xl font-bold tracking-tight text-gray-950 dark:text-[#F3F4F6] group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors leading-snug font-['Outfit']">
              <a :href="item.url" target="_blank" rel="noopener noreferrer" class="hover:underline">
                {{ item.title }}
              </a>
            </h2>

            <div
              v-if="item.description"
              v-html="sanitizeDescription(item.description)"
              class="text-xs sm:text-sm text-gray-600 dark:text-gray-400 line-clamp-2 leading-relaxed break-words max-w-3xl"
            ></div>
          </div>

          <!-- Right Action Rail: Tactile Save to Vault -->
          <div class="shrink-0 flex items-center md:flex-col md:items-end justify-between md:justify-start gap-2.5 pt-2 md:pt-0">
            <a
              :href="item.url"
              target="_blank"
              rel="noopener noreferrer"
              class="text-xs font-mono text-gray-400 hover:text-emerald-500 transition-colors hidden sm:inline"
              title="Open source in new tab"
            >
              source ↗
            </a>

            <!-- Tactile Apple-Grade Pill Button -->
            <button
              data-testid="save-to-vault-btn"
              @click="handleSaveToVault(item)"
              :disabled="savingUrls[item.url] || savedUrls[item.url]"
              class="px-3.5 py-1.5 rounded-xl text-xs font-semibold transition-all active:scale-95 cursor-pointer shadow-xs shrink-0 flex items-center gap-1.5"
              :class="savedUrls[item.url]
                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30'
                : 'bg-gray-950 text-white dark:bg-white dark:text-gray-950 hover:opacity-90 disabled:opacity-60'"
            >
              <template v-if="savingUrls[item.url]">
                <span class="w-3 h-3 rounded-full border-2 border-current border-t-transparent animate-spin"></span>
                <span>Saving...</span>
              </template>
              <template v-else-if="savedUrls[item.url]">
                <span>✓</span>
                <span>Saved!</span>
              </template>
              <template v-else>
                <svg class="w-3 h-3" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="12" y1="5" x2="12" y2="19" />
                  <line x1="5" y1="12" x2="19" y2="12" />
                </svg>
                <span>Save to Vault</span>
              </template>
            </button>
          </div>
        </article>
      </div>

    </main>

  </div>
</template>
