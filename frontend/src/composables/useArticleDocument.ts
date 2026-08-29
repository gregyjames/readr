import { ref, computed } from 'vue';
import axios from 'axios';
import { marked } from 'marked';
import {
  parseFrontmatter,
  stripFrontmatter,
  replaceWikilinks,
  type ArticleFrontmatter,
} from '../utils/markdown';

export interface ArticleRecord {
  id: number;
  title: string;
  image?: string;
  article: string;
  tags?: string;
  CreatedAt?: string;
}

export function useArticleDocument() {
  const loading = ref(true);
  const error = ref<string | null>(null);
  const rawContent = ref('');
  const editContent = ref('');
  const isSaving = ref(false);
  const articleRecord = ref<ArticleRecord | null>(null);

  const frontmatter = computed<ArticleFrontmatter>(() => {
    return parseFrontmatter(rawContent.value);
  });

  const articleTitle = computed(() => {
    return frontmatter.value.title || articleRecord.value?.title || '';
  });

  const articleTags = computed<string[]>(() => {
    if (frontmatter.value.tags && frontmatter.value.tags.length > 0) {
      return frontmatter.value.tags;
    }
    if (articleRecord.value?.tags) {
      return articleRecord.value.tags.split(',').map((t) => t.trim()).filter(Boolean);
    }
    return [];
  });

  const renderedProse = computed(() => {
    if (!rawContent.value) return '';
    const bodyMarkdown = stripFrontmatter(rawContent.value);
    const withLinks = replaceWikilinks(bodyMarkdown);
    return marked.parse(withLinks) as string;
  });

  const allArticles = ref<ArticleRecord[]>([]);

  async function fetchDocument(filename: string) {
    loading.value = true;
    error.value = null;

    try {
      // 1. Fetch metadata
      const metaRes = await axios.get<any[]>('/api/getarticles');
      allArticles.value = metaRes.data.map((a: any) => ({
        id: a.ID ?? a.id,
        title: a.title,
        image: a.image,
        article: a.article,
        tags: a.tags,
        CreatedAt: a.CreatedAt,
      }));

      const numericId = parseInt(filename.replace(/\.md$/, ''), 10);
      const matched = allArticles.value.find(
        (a) => a.id === numericId || (a.article && a.article.includes(filename))
      );
      if (matched) {
        articleRecord.value = matched;
      }

      // 2. Fetch raw markdown with cache-control headers
      const contentRes = await axios.get<string>(`/articles/${filename}`, {
        headers: {
          'Cache-Control': 'no-cache',
          Pragma: 'no-cache',
        },
      });
      rawContent.value = contentRes.data;
      editContent.value = contentRes.data;
    } catch (err: unknown) {
      console.error('Failed to load article document:', err);
      error.value = 'Failed to load article document.';
    } finally {
      loading.value = false;
    }
  }

  async function saveDocument(filename: string, newContent: string) {
    isSaving.value = true;
    const numericId = filename.replace(/\.md$/, '');

    try {
      await axios.post(`/api/edit/${numericId}`, {
        content: newContent,
      });
      rawContent.value = newContent;
      editContent.value = newContent;
      return true;
    } catch (err: unknown) {
      console.error('Failed to save article edit:', err);
      throw err;
    } finally {
      isSaving.value = false;
    }
  }

  return {
    loading,
    error,
    rawContent,
    editContent,
    isSaving,
    articleRecord,
    allArticles,
    frontmatter,
    articleTitle,
    articleTags,
    renderedProse,
    fetchDocument,
    saveDocument,
  };
}
