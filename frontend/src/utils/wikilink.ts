/**
 * Resolving a wikilink target to a vault article.
 *
 * A wikilink's target is a *filename*, not a title — `formatMOCArticleWikilink`
 * in the Librarian writes `[[<file base>]]`, or `[[<file base>|<real title>]]`
 * when the two differ. They differ whenever `SanitizeTitleFilename` had to
 * change the title to make it a legal cross-platform filename: it replaces
 * `? : * " < > | # ^ [ ] / \` with spaces and truncates at 100 characters. So
 * "What is Azure Pipelines? - Azure Pipelines" lives in a file named
 * "What is Azure Pipelines - Azure Pipelines.md".
 *
 * Matching on title alone therefore fails for exactly those notes, which is why
 * resolution has to fall back to the filename.
 */

export interface ResolvableArticle {
  ID: number
  title: string
  /** The vault-relative path, e.g. "/articles/Microsoft Azure/Pods.md". */
  article: string
}

const norm = (value: string) => String(value || '').trim().toLowerCase()

/** The vault-relative path with the leading "/articles/" and ".md" removed. */
export function articleRelativePath(path: string): string {
  return String(path || '')
    .replace(/^\/?articles\//, '')
    .replace(/\.md$/, '')
    .trim()
}

/** Just the filename, without its topic folder or ".md". */
export function articleFileBase(path: string): string {
  const rel = articleRelativePath(path)
  const slash = rel.lastIndexOf('/')
  return slash === -1 ? rel : rel.slice(slash + 1)
}

/**
 * Finds the article a wikilink target (or a route param) names.
 *
 * Tried in descending order of confidence, because the last test is ambiguous:
 * two topic folders may hold files of the same name, so an exact title, id or
 * full-path match must win before a bare filename is considered.
 */
export function resolveWikilinkTarget<T extends ResolvableArticle>(
  articles: T[],
  target: string
): T | undefined {
  const wanted = norm(target)
  if (!wanted || !articles?.length) return undefined

  return (
    articles.find(a => norm(a.title) === wanted) ??
    articles.find(a => String(a.ID) === target.trim()) ??
    articles.find(a => norm(articleRelativePath(a.article)) === wanted) ??
    articles.find(a => norm(articleFileBase(a.article)) === wanted)
  )
}
