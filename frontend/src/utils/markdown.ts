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
 * Extracts YAML frontmatter properties from the start of a markdown file.
 */
export function parseFrontmatter(rawText: string): ArticleFrontmatter {
  if (!rawText || !rawText.startsWith('---')) return {};

  const match = rawText.match(/^---\n([\s\S]*?)\n---/);
  if (!match) return {};

  const yamlContent = match[1];
  const properties: ArticleFrontmatter = {};

  for (const line of yamlContent.split('\n')) {
    const colonIndex = line.indexOf(':');
    if (colonIndex === -1) continue;

    const key = line.slice(0, colonIndex).trim();
    let val = line.slice(colonIndex + 1).trim();

    // Strip wrapping quotes
    if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
      val = val.slice(1, -1);
    }

    if (key === 'tags') {
      if (val.startsWith('[') && val.endsWith(']')) {
        properties.tags = val
          .slice(1, -1)
          .split(',')
          .map((t) => t.trim().replace(/^["']|["']$/g, ''))
          .filter(Boolean);
      } else if (val) {
        properties.tags = val.split(',').map((t) => t.trim()).filter(Boolean);
      } else {
        properties.tags = [];
      }
    } else if (key) {
      properties[key] = val;
    }
  }

  return properties;
}

/**
 * Strips the YAML frontmatter block from the beginning of markdown content.
 */
export function stripFrontmatter(rawText: string): string {
  if (!rawText || !rawText.startsWith('---')) return rawText;
  return rawText.replace(/^---\n[\s\S]*?\n---\n*/, '');
}

/**
 * Replaces [[Title|Label]] and [[Title]] wikilink syntax with HTML anchor tags.
 */
export function replaceWikilinks(content: string): string {
  if (!content) return '';
  const wikilinkRegex = /\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/g;
  return content.replace(wikilinkRegex, (_, title, label) => {
    const targetTitle = title.trim();
    const displayLabel = (label || title).trim();
    return `<a class="wikilink text-emerald-600 dark:text-emerald-400 font-medium border-b border-emerald-500/30 hover:border-emerald-500 transition-colors cursor-pointer" data-target="${targetTitle}">${displayLabel}</a>`;
  });
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
