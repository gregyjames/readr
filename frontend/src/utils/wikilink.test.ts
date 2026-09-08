import { describe, it, expect } from 'bun:test';
import { articleFileBase, articleRelativePath, resolveWikilinkTarget } from './wikilink';

// Mirrors the real vault: notes live in topic folders, and three Azure notes
// have a filename that differs from their title because SanitizeTitleFilename
// stripped a "?" or truncated at 100 characters.
const VAULT = [
  { ID: 1, title: 'Pods', article: '/articles/Kubernetes/Pods.md' },
  {
    ID: 2,
    title: 'What is Azure Pipelines? - Azure Pipelines',
    article: '/articles/Microsoft Azure/What is Azure Pipelines - Azure Pipelines.md',
  },
  {
    ID: 3,
    title: 'How to Use the Azure Well-Architected Framework Documentation - Microsoft Azure Well-Architected Framework',
    article: '/articles/Microsoft Azure/How to Use the Azure Well-Architected Framework Documentation - Microsoft Azure Well-Architected Fra.md',
  },
  { ID: 4, title: 'Loose Note', article: '/articles/Loose Note.md' },
];

describe('path helpers', () => {
  it('strips the /articles/ prefix and .md suffix', () => {
    expect(articleRelativePath('/articles/Kubernetes/Pods.md')).toBe('Kubernetes/Pods');
  });

  it('reduces a foldered path to its bare filename', () => {
    expect(articleFileBase('/articles/Kubernetes/Pods.md')).toBe('Pods');
    expect(articleFileBase('/articles/Loose Note.md')).toBe('Loose Note');
    expect(articleFileBase('')).toBe('');
  });
});

describe('resolveWikilinkTarget', () => {
  it('resolves a target that equals the title', () => {
    expect(resolveWikilinkTarget(VAULT, 'Pods')?.ID).toBe(1);
  });

  it('resolves a filename target whose title was sanitized', () => {
    // This is the case that was rendering as dead text: the "?" is legal in a
    // title but not in a filename, so the wikilink names the file.
    expect(
      resolveWikilinkTarget(VAULT, 'What is Azure Pipelines - Azure Pipelines')?.ID
    ).toBe(2);
  });

  it('resolves a filename target that was truncated at 100 characters', () => {
    expect(
      resolveWikilinkTarget(
        VAULT,
        'How to Use the Azure Well-Architected Framework Documentation - Microsoft Azure Well-Architected Fra'
      )?.ID
    ).toBe(3);
  });

  it('resolves a folder-qualified path', () => {
    expect(resolveWikilinkTarget(VAULT, 'Kubernetes/Pods')?.ID).toBe(1);
  });

  it('resolves a numeric id', () => {
    expect(resolveWikilinkTarget(VAULT, '4')?.ID).toBe(4);
  });

  it('is case- and whitespace-insensitive', () => {
    expect(resolveWikilinkTarget(VAULT, '  pODS  ')?.ID).toBe(1);
  });

  it('prefers an exact title over another note whose filename collides', () => {
    const ambiguous = [
      { ID: 10, title: 'Overview', article: '/articles/Azure/Something Else.md' },
      { ID: 11, title: 'Other', article: '/articles/Kubernetes/Overview.md' },
    ];
    expect(resolveWikilinkTarget(ambiguous, 'Overview')?.ID).toBe(10);
  });

  it('returns undefined for an unknown target', () => {
    expect(resolveWikilinkTarget(VAULT, 'Nonexistent Note')).toBeUndefined();
  });

  it('returns undefined for empty input', () => {
    expect(resolveWikilinkTarget(VAULT, '')).toBeUndefined();
    expect(resolveWikilinkTarget([], 'Pods')).toBeUndefined();
  });
});
