<template>
  <div>
    <!-- Floating Link Button -->
    <div
      v-if="showFloatingButton && !showSearchModal"
      class="fixed z-50 transform -translate-x-1/2 -translate-y-full pb-2 animate-fade-in"
      :style="{ top: `${buttonPosition.top}px`, left: `${buttonPosition.left}px` }"
    >
      <button
        @mousedown.prevent="openSearchModal"
        class="flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold rounded-full shadow-lg bg-stone-900 text-white hover:bg-emerald-600 dark:bg-stone-100 dark:text-stone-900 dark:hover:bg-emerald-400 dark:hover:text-stone-950 transition-all duration-200 cursor-pointer border border-stone-700/50 dark:border-stone-300/50"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
          <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
        </svg>
        Link Note
      </button>
    </div>

    <!-- Target Article Search Modal -->
    <div
      v-if="showSearchModal"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-stone-900/60 backdrop-blur-sm animate-fade-in"
      @mousedown.self="closeSearchModal"
    >
      <div class="w-full max-w-md bg-white dark:bg-stone-900 rounded-2xl shadow-2xl border border-stone-200 dark:border-stone-800 overflow-hidden flex flex-col max-h-[85vh]">
        <!-- Modal Header -->
        <div class="p-4 border-b border-stone-100 dark:border-stone-800 flex items-center justify-between">
          <div class="flex items-center gap-2">
            <div class="w-2 h-2 rounded-full bg-emerald-500"></div>
            <h3 class="text-sm font-semibold text-stone-900 dark:text-stone-100">
              Link "<span class="text-emerald-600 dark:text-emerald-400 font-serif italic">{{ selectedText }}</span>" to…
            </h3>
          </div>
          <button
            @click="closeSearchModal"
            class="text-stone-400 hover:text-stone-600 dark:hover:text-stone-200 text-lg leading-none cursor-pointer"
          >
            &times;
          </button>
        </div>

        <!-- Search Input -->
        <div class="p-4 border-b border-stone-100 dark:border-stone-800">
          <input
            v-model="searchQuery"
            type="text"
            placeholder="Search target article..."
            class="w-full px-3 py-2 text-sm bg-stone-50 dark:bg-stone-800/60 border border-stone-200 dark:border-stone-700 rounded-xl focus:outline-none focus:ring-2 focus:ring-emerald-500 dark:text-stone-100 placeholder-stone-400"
            autofocus
          />
        </div>

        <!-- Article Options List -->
        <div class="overflow-y-auto p-2 space-y-1 flex-1">
          <div
            v-for="article in filteredArticles"
            :key="getArticleId(article)"
            @click="createLink(article)"
            class="p-3 rounded-xl hover:bg-stone-100 dark:hover:bg-stone-800/80 cursor-pointer transition-colors flex items-center justify-between group"
          >
            <div>
              <div class="text-sm font-medium text-stone-800 dark:text-stone-200 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors">
                {{ article.title }}
              </div>
              <div v-if="article.tags" class="text-xs text-stone-400 font-mono mt-0.5">
                {{ article.tags }}
              </div>
            </div>
            <span class="text-xs font-mono text-stone-400 opacity-0 group-hover:opacity-100 transition-opacity">
              Connect ↗
            </span>
          </div>

          <div v-if="filteredArticles.length === 0" class="p-6 text-center text-xs text-stone-400 font-mono">
            No matching articles found
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue';
import axios from 'axios';

interface ArticleItem {
  ID?: number;
  id?: number;
  title: string;
  tags?: string;
  article: string;
}

const props = defineProps<{
  currentArticleId: string | number;
}>();

const emit = defineEmits<{
  (e: 'linked'): void;
}>();

const selectedText = ref('');
const showFloatingButton = ref(false);
const showSearchModal = ref(false);
const searchQuery = ref('');
const buttonPosition = ref({ top: 0, left: 0 });
const allArticles = ref<ArticleItem[]>([]);

function getNumericId(): number {
  if (typeof props.currentArticleId === 'number') return props.currentArticleId;
  const clean = String(props.currentArticleId || '')
    .replace(/^\/articles\//, '')
    .replace(/\.md$/, '');
  return parseInt(clean, 10);
}

function getArticleId(item: ArticleItem): number {
  return Number(item.ID ?? item.id ?? 0);
}

const filteredArticles = computed(() => {
  const currentId = getNumericId();
  const query = searchQuery.value.toLowerCase().trim();
  return allArticles.value.filter((a) => {
    const aId = getArticleId(a);
    if (aId === currentId) return false;
    if (!query) return true;
    return (
      a.title.toLowerCase().includes(query) ||
      (a.tags && a.tags.toLowerCase().includes(query))
    );
  });
});

function handleMouseUp() {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed) {
    showFloatingButton.value = false;
    return;
  }

  const text = selection.toString().trim();
  if (!text || text.length > 80) {
    showFloatingButton.value = false;
    return;
  }

  const range = selection.getRangeAt(0);
  const rect = range.getBoundingClientRect();

  selectedText.value = text;
  buttonPosition.value = {
    top: rect.top - 8,
    left: rect.left + rect.width / 2,
  };
  showFloatingButton.value = true;
}

async function openSearchModal() {
  showFloatingButton.value = false;
  showSearchModal.value = true;
  searchQuery.value = '';

  try {
    const res = await axios.get<ArticleItem[]>('/api/getarticles');
    allArticles.value = res.data;
  } catch (err) {
    console.error('Failed to fetch articles for linker:', err);
  }
}

function closeSearchModal() {
  showSearchModal.value = false;
  showFloatingButton.value = false;
  window.getSelection()?.removeAllRanges();
}

async function createLink(targetArticle: ArticleItem) {
  const sourceId = getNumericId();
  const targetId = getArticleId(targetArticle);
  if (isNaN(sourceId) || !targetId || !selectedText.value) return;

  try {
    await axios.post('/api/link', {
      sourceId,
      targetId,
      selectedText: selectedText.value,
    });
    closeSearchModal();
    emit('linked');
  } catch (err) {
    console.error('Failed to create article link:', err);
  }
}

onMounted(() => {
  document.addEventListener('mouseup', handleMouseUp);
});

onUnmounted(() => {
  document.removeEventListener('mouseup', handleMouseUp);
});
</script>
