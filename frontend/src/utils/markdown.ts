import * as yaml from 'js-yaml';

export interface ArticleFrontmatter {
  title?: string;
  source?: string;
  tags?: string[];
  cover?: string;
  saved?: string;
  [key: string]: unknown;
}

export interface ParsedDocument {
  frontmatter: ArticleFrontmatter;
  content: string;
}

/**
 * Extracts YAML frontmatter properties from the start of a markdown file using js-yaml.
 */
export function parseFrontmatter(rawText: string): ArticleFrontmatter {
  if (!rawText || !rawText.trimStart().startsWith('---')) return {};

  const match = rawText.match(/^---[ \t]*\r?\n([\s\S]*?)\r?\n---/);
  if (!match) return {};

  const yamlContent = match[1];
  try {
    const parsed = yaml.load(yamlContent);
    if (parsed && typeof parsed === 'object') {
      const result = parsed as Record<string, unknown>;
      // Ensure tags is string array if present
      if (result.tags && Array.isArray(result.tags)) {
        result.tags = result.tags.map((t) => String(t).trim()).filter(Boolean);
      } else if (result.tags && typeof result.tags === 'string') {
        result.tags = result.tags.split(',').map((t) => t.trim()).filter(Boolean);
      }
      return result as ArticleFrontmatter;
    }
  } catch {
    // If YAML parsing fails, return empty object without crashing
  }

  return {};
}

/**
 * Strips the YAML frontmatter block from the beginning of markdown content.
 */
export function stripFrontmatter(rawText: string): string {
  if (!rawText || !rawText.trimStart().startsWith('---')) return rawText;
  return rawText.replace(/^---[ \t]*\r?\n[\s\S]*?\r?\n---[ \t]*\r?\n*/, '');
}

/**
 * Replaces [[Title|Label]] and [[Title]] wikilink syntax with HTML anchor tags,
 * while safely ignoring code blocks, inline code spans, and HTML blocks.
 */
export function replaceWikilinks(content: string): string {
  if (!content) return '';

  const maskedBlocks: string[] = [];
  const mask = (snippet: string) => {
    const token = `__CODE_SPAN_OR_BLOCK_${maskedBlocks.length}__`;
    maskedBlocks.push(snippet);
    return token;
  };

  // 1. Mask fenced code blocks: ```...``` or ~~~...~~~
  let text = content.replace(/(```[\s\S]*?```|~~~[\s\S]*?~~~)/g, (match) => mask(match));

  // 2. Mask inline code spans: `...`
  text = text.replace(/(`+)([\s\S]*?)\1/g, (match) => mask(match));

  // 3. Mask HTML script/style or complex tags if needed
  text = text.replace(/<pre[\s\S]*?<\/pre>/gi, (match) => mask(match));
  text = text.replace(/<code[\s\S]*?<\/code>/gi, (match) => mask(match));

  // 4. Replace wikilinks in prose
  const wikilinkRegex = /\[\[([^[\]\n]+?)\]\]/g;
  text = text.replace(wikilinkRegex, (_, raw) => {
    const trimmed = raw.trim();
    const parts = trimmed.split('|');
    const target = parts[0].trim();
    const label = parts.length > 1 ? parts.slice(1).join('|').trim() : target;
    return `<a class="wikilink text-emerald-600 dark:text-emerald-400 font-medium border-b border-emerald-500/30 hover:border-emerald-500 transition-colors cursor-pointer" data-target="${target}">${label}</a>`;
  });

  // 5. Restore masked blocks in reverse order
  for (let i = maskedBlocks.length - 1; i >= 0; i--) {
    const token = `__CODE_SPAN_OR_BLOCK_${i}__`;
    text = text.replace(token, () => maskedBlocks[i]);
  }

  return text;
}

/**
 * Safely extracts clean hostname from URL string without throwing.
 */
export function getHostname(rawUrl?: string): string {
  if (!rawUrl) return '';
  try {
    const parsed = new URL(rawUrl);
    return parsed.hostname.replace(/^www\./, '');
  } catch {
    return rawUrl;
  }
}

