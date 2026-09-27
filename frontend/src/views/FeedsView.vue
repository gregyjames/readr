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

// DOMPurify hook to ensure all anchor links open safely in a new tab
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

  savingUrls.value = { ...savingUrls.value, [item.url]: true }
  try {
    await ingestAPI.ingestUrl(item.url)
    savedUrls.value = { ...savedUrls.value, [item.url]: true }
    showToast(`Saved "${item.title || 'article'}" to Vault!`, 'success')
  } catch (err: any) {
    showToast(err.message || 'Failed to save to vault', 'error')
  } finally {
    const updated = { ...savingUrls.value }
    delete updated[item.url]
    savingUrls.value = updated
  }
}

const formatDate = (dateStr: string): string => {
  if (!dateStr) return ''
  try {
    const d = new Date(dateStr)
    if (isNaN(d.getTime())) return dateStr
    return d.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    })
  } catch {
    return dateStr
  }
}

const sanitizeDescription = (desc: string): string => {
  if (!desc) return ''
  return DOMPurify.sanitize(desc, {
    ALLOWED_TAGS: ['b', 'i', 'em', 'strong', 'a', 'p', 'br', 'span', 'code'],
    ALLOWED_ATTR: ['href', 'target', 'rel'],
  })
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
  <div class="p-3 sm:p-6 w-full max-w-7xl mx-auto h-[100dvh] flex flex-col">
    <!-- Floating Toast Notification -->
    <transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="transform -translate-y-2 opacity-0"
      enter-to-class="transform translate-y-0 opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="transform translate-y-0 opacity-100"
      leave-to-class="transform -translate-y-2 opacity-0"
    >
      <div
        v-if="toastMessage"
        role="status"
        aria-live="polite"
        class="fixed top-5 right-5 z-50 flex items-center gap-2 px-4 py-2.5 rounded-xl shadow-lg border text-xs font-medium"
        :class="toastMessage.type === 'error'
          ? 'bg-red-50 text-red-800 border-red-200 dark:bg-red-950/80 dark:text-red-200 dark:border-red-800'
          : 'bg-emerald-50 text-emerald-800 border-emerald-200 dark:bg-emerald-950/80 dark:text-emerald-200 dark:border-emerald-800'"
      >
        <span>{{ toastMessage.text }}</span>
        <button @click="toastMessage = null" class="opacity-60 hover:opacity-100 ml-1">✕</button>
      </div>
    </transition>

    <!-- Main Two-Pane Container -->
    <div class="flex-1 flex flex-col md:flex-row bg-white dark:bg-[#12151C] rounded-2xl border border-gray-200/80 dark:border-white/[0.08] overflow-hidden shadow-2xs">
      
      <!-- Left Sidebar: Subscribed Feeds -->
      <aside class="w-full md:w-72 lg:w-80 flex-shrink-0 border-b md:border-b-0 md:border-r border-gray-200/80 dark:border-white/[0.08] flex flex-col bg-gray-50/60 dark:bg-white/[0.01]">
        
        <!-- Sidebar Header: Title and Add Input -->
        <div class="p-4 border-b border-gray-200/80 dark:border-white/[0.08] space-y-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <svg class="w-4 h-4 text-emerald-600 dark:text-emerald-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M4 11a9 9 0 0 1 9 9" />
                <path d="M4 4a16 16 0 0 1 16 16" />
                <circle cx="5" cy="19" r="1" />
              </svg>
              <h1 class="text-sm font-semibold text-gray-900 dark:text-gray-100">Feeds</h1>
            </div>
            <span class="text-[11px] font-mono px-2 py-0.5 rounded-full bg-gray-200/70 dark:bg-white/10 text-gray-600 dark:text-gray-300">
              {{ feeds.length }}
            </span>
          </div>

          <!-- Add Feed Form -->
          <form @submit.prevent="handleAddFeed" class="space-y-1">
            <div class="relative flex items-center">
              <input
                v-model="newFeedUrl"
                type="url"
                placeholder="Add RSS or Atom URL..."
                :disabled="isAddingFeed"
                aria-label="Feed URL"
                class="w-full text-xs px-3 py-2 pr-8 rounded-lg border border-gray-200 dark:border-white/10 bg-white dark:bg-white/[0.04] text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-1 focus:ring-emerald-500 focus:border-emerald-500 disabled:opacity-50 transition-colors"
              />
              <button
                type="submit"
                :disabled="isAddingFeed || !newFeedUrl.trim()"
                class="absolute right-1.5 p-1 rounded text-gray-400 hover:text-emerald-600 dark:hover:text-emerald-400 disabled:opacity-30 disabled:hover:text-gray-400 transition-colors cursor-pointer"
                title="Subscribe"
                aria-label="Subscribe"
              >
                <svg v-if="!isAddingFeed" class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="12" y1="5" x2="12" y2="19"></line>
                  <line x1="5" y1="12" x2="19" y2="12"></line>
                </svg>
                <svg v-else class="w-3.5 h-3.5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                  <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
                </svg>
              </button>
            </div>
            <p v-if="addError" class="text-[11px] text-red-500 dark:text-red-400 px-1 pt-0.5">
              {{ addError }}
            </p>
          </form>
        </div>

        <!-- Feeds Navigation List -->
        <div class="flex-1 overflow-y-auto p-2 space-y-1">
          <!-- All Feeds Item -->
          <div
            data-testid="all-feeds-btn"
            role="button"
            tabindex="0"
            @click="selectFeed(null)"
            @keydown.enter.prevent="selectFeed(null)"
            @keydown.space.prevent="selectFeed(null)"
            class="group flex items-center justify-between px-3 py-2 rounded-lg text-xs transition-colors cursor-pointer"
            :class="selectedFeedId === null
              ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 font-medium border border-emerald-500/20'
              : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/[0.04]'"
          >
            <div class="flex items-center gap-2 truncate">
              <svg class="w-3.5 h-3.5 text-gray-400 dark:text-gray-500 flex-shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect width="7" height="7" x="3" y="3" rx="1" />
                <rect width="7" height="7" x="14" y="3" rx="1" />
                <rect width="7" height="7" x="14" y="14" rx="1" />
                <rect width="7" height="7" x="3" y="14" rx="1" />
              </svg>
              <span class="truncate">All Feeds</span>
            </div>
            <span class="text-[10px] px-1.5 py-0.2 rounded-full bg-gray-200/50 dark:bg-white/10 font-mono text-gray-500 dark:text-gray-400">
              {{ timeline.length }}
            </span>
          </div>

          <div v-if="isLoadingFeeds" class="text-center py-6 text-xs text-gray-400 font-mono">
            Loading feeds...
          </div>

          <div v-else-if="feeds.length === 0" class="text-center py-6 px-3 text-xs text-gray-400 dark:text-gray-500 leading-relaxed">
            No subscribed feeds.<br />Add an RSS or Atom feed above.
          </div>

          <!-- Feed Items -->
          <div
            v-for="feed in feeds"
            :key="feed.id"
            :data-testid="`feed-item-${feed.id}`"
            role="button"
            tabindex="0"
            @click="selectFeed(feed.id)"
            @keydown.enter.prevent="selectFeed(feed.id)"
            @keydown.space.prevent="selectFeed(feed.id)"
            class="group flex items-center justify-between px-3 py-2 rounded-lg text-xs transition-colors cursor-pointer"
            :class="selectedFeedId === feed.id
              ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300 font-medium border border-emerald-500/20'
              : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/[0.04]'"
          >
            <div class="flex flex-col truncate min-w-0 pr-1 flex-1">
              <span class="truncate font-medium">{{ feed.title || feed.url }}</span>
              <span v-if="feed.siteUrl || feed.url" class="truncate text-[10px] text-gray-400 dark:text-gray-500">
                {{ feed.siteUrl || feed.url }}
              </span>
            </div>

            <!-- Remove Button -->
            <button
              :data-testid="`remove-feed-${feed.id}`"
              @click.stop="handleRemoveFeed(feed, $event)"
              :disabled="removingFeedId === feed.id"
              title="Remove feed"
              class="opacity-0 group-hover:opacity-100 p-1 text-gray-400 hover:text-red-500 rounded transition-opacity cursor-pointer"
              aria-label="Remove feed"
            >
              <svg v-if="removingFeedId !== feed.id" class="w-3.5 h-3.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="3 6 5 6 21 6"></polyline>
                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path>
              </svg>
              <svg v-else class="w-3.5 h-3.5 animate-spin text-red-500" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
              </svg>
            </button>
          </div>
        </div>
      </aside>

      <!-- Right Main Timeline Area -->
      <main class="flex-1 flex flex-col min-w-0 bg-white dark:bg-[#12151C] overflow-hidden">
        
        <!-- Timeline Header -->
        <header class="px-5 py-3 border-b border-gray-100 dark:border-white/[0.06] flex items-center justify-between bg-white/80 dark:bg-[#12151C]/80 backdrop-blur-xs sticky top-0 z-10">
          <div class="flex flex-col truncate min-w-0 pr-4">
            <div class="flex items-center gap-2">
              <h2 class="font-semibold text-sm text-gray-900 dark:text-gray-100 truncate">
                {{ selectedFeed ? selectedFeed.title : 'All Feeds' }}
              </h2>
              <span class="text-[11px] font-mono px-2 py-0.5 rounded-full bg-gray-100 dark:bg-white/[0.06] text-gray-500 dark:text-gray-400">
                {{ timeline.length }} {{ timeline.length === 1 ? 'article' : 'articles' }}
              </span>
            </div>
            <a
              v-if="selectedFeed?.siteUrl"
              :href="selectedFeed.siteUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="text-[11px] text-gray-400 dark:text-gray-500 hover:text-emerald-600 dark:hover:text-emerald-400 truncate mt-0.5 inline-flex items-center gap-1"
            >
              <span>{{ selectedFeed.siteUrl }}</span>
              <svg class="w-2.5 h-2.5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
                <polyline points="15 3 21 3 21 9"></polyline>
                <line x1="10" y1="14" x2="21" y2="3"></line>
              </svg>
            </a>
          </div>

          <!-- Refresh Button -->
          <button
            @click="fetchTimeline(selectedFeedId)"
            :disabled="isLoadingTimeline"
            title="Refresh Timeline"
            aria-label="Refresh Timeline"
            class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs text-gray-600 dark:text-gray-300 hover:text-gray-900 dark:hover:text-white bg-gray-100 dark:bg-white/[0.05] hover:bg-gray-200 dark:hover:bg-white/[0.08] transition-colors cursor-pointer disabled:opacity-50"
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
        </header>

        <!-- Timeline Content Scroll Area -->
        <div class="flex-1 overflow-y-auto p-4 sm:p-6">
          <!-- Loading State -->
          <div v-if="isLoadingTimeline" class="flex flex-col items-center justify-center py-20 text-gray-400">
            <svg class="w-7 h-7 animate-spin text-emerald-500 mb-3" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            <span class="text-xs font-mono">Fetching timeline articles...</span>
          </div>

          <!-- Empty State (No Feeds) -->
          <div v-else-if="feeds.length === 0" class="flex flex-col items-center justify-center py-24 text-center px-4">
            <div class="w-12 h-12 rounded-2xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center mb-3">
              <svg class="w-6 h-6" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M4 11a9 9 0 0 1 9 9" />
                <path d="M4 4a16 16 0 0 1 16 16" />
                <circle cx="5" cy="19" r="1" />
              </svg>
            </div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">No feeds subscribed yet</h3>
            <p class="text-xs text-gray-500 dark:text-gray-400 max-w-sm mt-1 mb-4 leading-relaxed">
              Add your favorite RSS or Atom feeds using the sidebar to aggregate articles into a unified reading timeline.
            </p>
          </div>

          <!-- Empty State (No Articles in Timeline) -->
          <div v-else-if="timeline.length === 0" class="flex flex-col items-center justify-center py-24 text-center px-4">
            <div class="w-12 h-12 rounded-2xl bg-gray-100 dark:bg-white/[0.05] text-gray-400 flex items-center justify-center mb-3">
              <svg class="w-6 h-6" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="10" />
                <line x1="8" y1="12" x2="16" y2="12" />
              </svg>
            </div>
            <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">Timeline is empty</h3>
            <p class="text-xs text-gray-500 dark:text-gray-400 max-w-sm mt-1 leading-relaxed">
              No recent entries were found for this feed. Try clicking refresh or adding other feeds.
            </p>
          </div>

          <!-- Timeline Items List -->
          <div v-else class="space-y-4 max-w-4xl mx-auto">
            <article
              v-for="item in timeline"
              :key="item.url"
              data-testid="timeline-card"
              class="group bg-white dark:bg-[#151921] border border-gray-200/80 dark:border-white/[0.07] rounded-xl p-4 sm:p-5 hover:border-gray-300 dark:hover:border-white/15 transition-all shadow-2xs hover:shadow-xs space-y-3"
            >
              <!-- Card Meta Row -->
              <div class="flex items-center justify-between gap-2">
                <span class="inline-flex items-center text-[11px] font-medium px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border border-emerald-500/20">
                  {{ item.feedTitle || 'Feed' }}
                </span>
                <time v-if="item.published" class="text-[11px] text-gray-400 dark:text-gray-500 font-mono">
                  {{ formatDate(item.published) }}
                </time>
              </div>

              <!-- Article Title -->
              <div>
                <a
                  :href="item.url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-sm sm:text-base font-semibold text-gray-900 dark:text-gray-100 hover:text-emerald-600 dark:hover:text-emerald-400 transition-colors inline-flex items-start gap-1.5 group/link"
                >
                  <span>{{ item.title }}</span>
                  <svg class="w-3.5 h-3.5 opacity-0 group-hover/link:opacity-100 text-gray-400 hover:text-emerald-600 transition-opacity mt-1 flex-shrink-0" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                    <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
                    <polyline points="15 3 21 3 21 9"></polyline>
                    <line x1="10" y1="14" x2="21" y2="3"></line>
                  </svg>
                </a>
              </div>

              <!-- Sanitized Short Description -->
              <div
                v-if="item.description"
                v-html="sanitizeDescription(item.description)"
                class="text-xs sm:text-sm text-gray-600 dark:text-gray-300 line-clamp-3 leading-relaxed break-words"
              ></div>

              <!-- Action Footer -->
              <div class="pt-1 flex items-center justify-between border-t border-gray-100 dark:border-white/[0.04]">
                <button
                  data-testid="save-to-vault-btn"
                  @click="handleSaveToVault(item)"
                  :disabled="savingUrls[item.url] || savedUrls[item.url]"
                  class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all cursor-pointer"
                  :class="savedUrls[item.url]
                    ? 'bg-emerald-50 dark:bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/30'
                    : 'bg-gray-100 hover:bg-gray-200 dark:bg-white/[0.06] dark:hover:bg-white/[0.1] text-gray-700 dark:text-gray-200 border border-gray-200/80 dark:border-white/10 active:scale-95 disabled:opacity-75'"
                >
                  <!-- Saving State -->
                  <template v-if="savingUrls[item.url]">
                    <svg class="w-3.5 h-3.5 animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
                      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
                    </svg>
                    <span>Saving...</span>
                  </template>

                  <!-- Saved State -->
                  <template v-else-if="savedUrls[item.url]">
                    <svg class="w-3.5 h-3.5 text-emerald-600 dark:text-emerald-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                      <polyline points="20 6 9 17 4 12"></polyline>
                    </svg>
                    <span>Saved!</span>
                  </template>

                  <!-- Default State -->
                  <template v-else>
                    <svg class="w-3.5 h-3.5 text-gray-500 dark:text-gray-400" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                      <path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"></path>
                      <polyline points="17 21 17 13 7 13 7 21"></polyline>
                      <polyline points="7 3 7 8 15 8"></polyline>
                    </svg>
                    <span>Save to Vault</span>
                  </template>
                </button>

                <a
                  :href="item.url"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-[11px] text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 transition-colors"
                >
                  Visit original &rarr;
                </a>
              </div>
            </article>
          </div>
        </div>
      </main>
    </div>
  </div>
</template>
