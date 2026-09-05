/**
 * MOC (Map of Content) detection.
 *
 * The Librarian creates hubs with `tags = "moc, <cluster>"`, but that marker
 * does not survive: the OKF enricher rewrites tags from the document's own
 * frontmatter, and every MOC in a live vault ends up tagged with its topic
 * only ("k8s", "azure"). A hub that gets reparsed loses `type: moc` too.
 *
 * The `MOC - ` title prefix is the one marker that survives both agents, so it
 * carries the detection here; the tag check stays as a cheap first test for
 * freshly-created hubs. This mirrors the backend heuristic in
 * internal/agents/cluster_classifier.go, which also tests title before tags.
 */

/** Matches the backend's title test, including the trailing space in "moc - ". */
function hasMocTitle(title: string): boolean {
  const t = (title || '').toLowerCase().trim()
  return t.startsWith('moc - ') || t.startsWith('moc:') || t.startsWith('moc ') || t === 'moc'
}

function hasMocTag(tags: string | string[] | undefined | null): boolean {
  if (!tags) return false
  const list = Array.isArray(tags) ? tags : tags.split(',')
  return list.some(t => String(t).trim().toLowerCase() === 'moc')
}

/**
 * Whether an article is a MOC hub, from its title and tags. Tags may be the raw
 * comma-separated string from the API or an already-parsed array.
 */
export function isMoc(title: string, tags: string | string[] | undefined | null): boolean {
  return hasMocTag(tags) || hasMocTitle(title)
}
