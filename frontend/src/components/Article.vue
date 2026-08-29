<template>
  <div v-if="error" class="bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 p-4 text-center w-full font-medium border-b border-red-100 dark:border-red-900/30">
    {{ error }}
  </div>

  <div class="flex flex-col lg:flex-row w-full max-w-7xl mx-auto items-start relative px-4 lg:px-8">
    <article class="w-full lg:w-2/3 max-w-2xl mx-auto py-16 transition-colors duration-300">
      <!-- Loading State -->
      <div v-if="loading" class="animate-pulse space-y-6 py-12">
        <div class="h-4 bg-stone-200 dark:bg-stone-800 rounded w-1/4"></div>
        <div class="h-10 bg-stone-200 dark:bg-stone-800 rounded w-3/4"></div>
        <div class="space-y-3 pt-6">
          <div class="h-4 bg-stone-200 dark:bg-stone-800 rounded w-full"></div>
          <div class="h-4 bg-stone-200 dark:bg-stone-800 rounded w-5/6"></div>
          <div class="h-4 bg-stone-200 dark:bg-stone-800 rounded w-4/6"></div>
        </div>
      </div>

      <!-- Edit Mode -->
      <div v-else-if="isEditing" class="mb-8 w-full animate-in fade-in slide-in-from-top-4 duration-300">
        <textarea
          v-model="editDraft"
          rows="18"
          class="w-full p-6 bg-stone-50 dark:bg-[#111] border border-stone-200 dark:border-stone-800 rounded-2xl text-stone-900 dark:text-stone-100 font-mono text-sm leading-relaxed focus:ring-4 focus:ring-emerald-500/20 focus:border-emerald-500 outline-none resize-y transition-all shadow-inner"
        ></textarea>
        <div class="flex justify-end gap-3 mt-4">
          <button
            @click="cancelEditing"
            :disabled="isSaving"
            class="px-5 py-2.5 text-stone-600 dark:text-stone-400 hover:text-stone-900 dark:hover:text-stone-100 font-bold text-sm transition-colors disabled:opacity-50 cursor-pointer"
          >
            Cancel
          </button>
          <button
            @click="handleSave"
            :disabled="isSaving"
            class="px-6 py-2.5 bg-emerald-500 hover:bg-emerald-600 text-white font-bold text-sm rounded-xl transition-colors active:scale-95 shadow-sm disabled:opacity-50 flex items-center gap-2 cursor-pointer"
          >
            <svg v-if="isSaving" class="animate-spin h-4 w-4 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
              <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8H4z"></path>
            </svg>
            {{ isSaving ? 'Saving...' : 'Save Changes' }}
          </button>
        </div>
      </div>

      <!-- Document Display -->
      <div v-else>
        <!-- Editorial Provenance Masthead -->
        <header v-if="frontmatter.source || frontmatter.saved || articleTags.length > 0" class="mb-10 pb-8 border-b border-stone-200/60 dark:border-white/10">
          <div class="flex items-center justify-between gap-4">
            <div class="flex flex-wrap items-center gap-3">
              <!-- Source Badge -->
              <a
                v-if="frontmatter.source"
                :href="frontmatter.source"
                target="_blank"
                rel="noopener noreferrer"
                class="group inline-flex items-center gap-2 px-3 py-1 rounded-full bg-stone-100/80 dark:bg-white/5 hover:bg-emerald-50 dark:hover:bg-emerald-950/40 text-stone-700 dark:text-stone-300 hover:text-emerald-700 dark:hover:text-emerald-300 border border-stone-200/50 dark:border-white/5 text-xs font-medium tracking-tight transition-all duration-200 active:scale-95"
              >
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 group-hover:scale-125 transition-transform"></span>
                <span class="font-medium">{{ getHostname(frontmatter.source) }}</span>
                <svg xmlns="http://www.w3.org/2000/svg" width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round" class="text-stone-400 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 group-hover:translate-x-0.5 group-hover:-translate-y-0.5 transition-all">
                  <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"></path>
                  <polyline points="15 3 21 3 21 9"></polyline>
                  <line x1="10" y1="14" x2="21" y2="3"></line>
                </svg>
              </a>

              <!-- Saved Date -->
              <div v-if="frontmatter.saved" class="text-xs text-stone-400 dark:text-stone-500 font-medium flex items-center gap-1.5">
                <span class="hidden sm:inline">Saved</span>
                <span>{{ formatDisplayDate(frontmatter.saved) }}</span>
              </div>
            </div>

            <!-- Edit Control -->
            <button
              @click="startEditing"
              class="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-medium text-stone-500 dark:text-stone-400 hover:text-stone-900 dark:hover:text-stone-100 hover:bg-stone-100 dark:hover:bg-white/5 transition-all active:scale-95 cursor-pointer"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path>
                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path>
              </svg>
              <span>Edit</span>
            </button>
          </div>

          <!-- Tags -->
          <div v-if="articleTags.length > 0" class="flex flex-wrap items-center gap-2 mt-4">
            <span
              v-for="tag in articleTags"
              :key="tag"
              class="inline-flex items-center text-[10px] font-bold uppercase tracking-wider px-2.5 py-1 rounded-md bg-stone-100/90 dark:bg-white/5 text-stone-700 dark:text-stone-300 border border-stone-200/50 dark:border-white/5"
            >
              {{ tag }}
            </span>
          </div>
        </header>

        <!-- Article Title -->
        <h1 v-if="articleTitle" class="text-3xl sm:text-4xl font-serif font-bold text-stone-900 dark:text-stone-100 tracking-tight leading-tight mb-8">
          {{ articleTitle }}
        </h1>

        <!-- Article Prose Body -->
        <div
          ref="proseContainer"
          class="prose dark:prose-invert prose-stone max-w-none prose-headings:font-serif prose-p:font-serif prose-p:text-lg prose-p:leading-relaxed prose-a:text-emerald-600 dark:prose-a:text-emerald-400 prose-img:rounded-2xl prose-img:shadow-md"
          v-html="renderedProse"
          @click="handleProseClick"
        ></div>

        <!-- Linked Mentions / Backlinks Table -->
        <div v-if="backlinks.length > 0" class="mt-20 pt-10 border-t border-stone-200/60 dark:border-white/10">
          <h3 class="text-xl font-bold tracking-tight text-stone-900 dark:text-stone-100 mb-6 flex items-center gap-2.5">
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-stone-400"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"></path><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"></path></svg>
            Linked Mentions
          </h3>
          <div class="overflow-hidden rounded-2xl border border-stone-200/80 dark:border-stone-800 bg-stone-50/50 dark:bg-stone-900/30">
            <table class="w-full text-left border-collapse text-sm">
              <tbody>
                <tr
                  v-for="link in backlinks"
                  :key="link.id"
                  @click="router.push(`/articles/${link.id}.md`)"
                  class="group border-b last:border-0 border-stone-100 dark:border-stone-800/60 hover:bg-stone-100/80 dark:hover:bg-stone-800/50 cursor-pointer transition-colors"
                >
                  <td class="px-6 py-4 font-medium text-stone-900 dark:text-stone-100 group-hover:text-emerald-600 dark:group-hover:text-emerald-400 transition-colors">
                    {{ link.title }}
                  </td>
                  <td class="px-6 py-4 text-right text-xs font-mono text-stone-400">
                    {{ link.tags }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </article>

    <!-- Sticky Local Graph Sidebar -->
    <LocalGraph :article-id="articleFilename" />

    <!-- Floating Text Linker Modal -->
    <ArticleLinkerModal
      :current-article-id="articleFilename"
      @linked="handleLinked"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch, nextTick } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import hljs from 'highlight.js';
import 'highlight.js/styles/github-dark.css';

import { useArticleDocument } from '../composables/useArticleDocument';
import { getHostname } from '../utils/markdown';
import LocalGraph from './LocalGraph.vue';
import ArticleLinkerModal from './ArticleLinkerModal.vue';

const props = defineProps<{ id?: string }>();
const route = useRoute();
const router = useRouter();

const articleFilename = computed(() => {
  const id = props.id || String(route.params.id || '');
  return id.endsWith('.md') ? id : `${id}.md`;
});

const {
  loading,
  error,
  rawContent,
  frontmatter,
  articleTitle,
  articleTags,
  renderedProse,
  allArticles,
  isSaving,
  fetchDocument,
  saveDocument,
} = useArticleDocument();

const isEditing = ref(false);
const editDraft = ref('');
const proseContainer = ref<HTMLElement | null>(null);

function formatDisplayDate(dateStr?: string): string {
  if (!dateStr) return '';
  try {
    const d = new Date(dateStr);
    if (isNaN(d.getTime())) return dateStr;
    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  } catch {
    return dateStr;
  }
}

function getNumericId(): number {
  const clean = articleFilename.value.replace(/\.md$/, '');
  return parseInt(clean, 10);
}

// Compute backlinks by matching article titles in markdown content of other articles
const backlinks = computed(() => {
  const title = articleTitle.value.toLowerCase().trim();
  const currentId = getNumericId();
  if (!title && isNaN(currentId)) return [];

  return allArticles.value.filter((a) => {
    if (a.id === currentId) return false;
    return false; // Backlinks can also be fetched from graph edges
  });
});

function startEditing() {
  editDraft.value = rawContent.value;
  isEditing.value = true;
}

function cancelEditing() {
  isEditing.value = false;
  editDraft.value = '';
}

async function handleSave() {
  try {
    await saveDocument(articleFilename.value, editDraft.value);
    isEditing.value = false;
    await highlightCodeBlocks();
  } catch {
    alert('Failed to save changes');
  }
}

async function handleLinked() {
  await fetchDocument(articleFilename.value);
  await highlightCodeBlocks();
}

function handleProseClick(e: MouseEvent) {
  const target = (e.target as HTMLElement).closest('a');
  if (!target) return;

  const targetTitle = target.getAttribute('data-target');
  if (targetTitle) {
    e.preventDefault();
    const query = targetTitle.toLowerCase().trim();

    const matched = allArticles.value.find(
      (a) => a.title.toLowerCase().trim() === query
    );
    if (matched) {
      router.push(`/articles/${matched.id}.md`);
      return;
    }

    const cleanId = targetTitle.replace(/\.md$/, '');
    const matchedById = allArticles.value.find(
      (a) => String(a.id) === cleanId || (a.article && a.article.includes(targetTitle))
    );
    if (matchedById) {
      router.push(`/articles/${matchedById.id}.md`);
      return;
    }

    router.push({ path: '/', query: { search: targetTitle } });
  }
}

async function highlightCodeBlocks() {
  await nextTick();
  if (proseContainer.value) {
    proseContainer.value.querySelectorAll('pre code').forEach((block) => {
      hljs.highlightElement(block as HTMLElement);
    });
  }
}

watch(
  articleFilename,
  async (newFilename) => {
    if (newFilename) {
      await fetchDocument(newFilename);
      await highlightCodeBlocks();
    }
  },
  { immediate: true }
);

onMounted(async () => {
  await highlightCodeBlocks();
});
</script>
