import { describe, it, expect } from 'bun:test';
import { parseFrontmatter, stripFrontmatter, replaceWikilinks, getHostname } from './markdown';

describe('markdown utilities', () => {
  const sampleMarkdown = `---
title: "Neural Networks Overview"
source: "https://news.ycombinator.com/item?id=123"
tags: [ai, deep-learning, machine-learning]
cover: "/images/123/cover.png"
saved: 2026-08-29
---

# Neural Networks Overview

Here is a link to [[Artificial Intelligence|AI]] and [[Backpropagation]].
`;

  it('parses YAML frontmatter correctly', () => {
    const fm = parseFrontmatter(sampleMarkdown);
    expect(fm.title).toBe('Neural Networks Overview');
    expect(fm.source).toBe('https://news.ycombinator.com/item?id=123');
    expect(fm.tags).toEqual(['ai', 'deep-learning', 'machine-learning']);
    expect(fm.cover).toBe('/images/123/cover.png');
    expect(fm.saved).toBe('2026-08-29');
  });

  it('strips frontmatter cleanly from prose', () => {
    const stripped = stripFrontmatter(sampleMarkdown);
    expect(stripped.startsWith('# Neural Networks Overview')).toBe(true);
    expect(stripped.includes('---')).toBe(false);
  });

  it('replaces [[wikilinks]] with interactive HTML links', () => {
    const content = 'Read [[AI Guide|Artificial Intelligence]] and [[Machine Learning]] and [[Cython Fast|NumPy | Cython]].';
    const replaced = replaceWikilinks(content);
    expect(replaced).toContain('data-target="AI Guide"');
    expect(replaced).toContain('>Artificial Intelligence</a>');
    expect(replaced).toContain('data-target="Machine Learning"');
    expect(replaced).toContain('>Machine Learning</a>');
    expect(replaced).toContain('data-target="Cython Fast"');
    expect(replaced).toContain('>NumPy | Cython</a>');
  });

  it('extracts hostnames cleanly without throwing', () => {
    expect(getHostname('https://www.witsnode.com/post/123')).toBe('witsnode.com');
    expect(getHostname('https://sub.domain.org/path')).toBe('sub.domain.org');
    expect(getHostname('not-a-url')).toBe('not-a-url');
    expect(getHostname('')).toBe('');
  });
});
