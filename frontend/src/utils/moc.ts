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

/**
 * A MOC's collection reading rollup, as sent by the API on hub articles only.
 * Absent on plain notes and on hubs with no resolvable members.
 */
export interface MocProgress {
  total: number
  read: number
  in_progress: number
}

/**
 * The smallest arc a nonzero count is allowed to draw, as a fraction of the
 * circumference — about 30 degrees.
 *
 * This deliberately overstates small counts. At the size these rings render, a
 * true 1/20 is a hairline that disappears into the track, so the ring would say
 * "none read" while the text beside it says "1/20" — a worse lie than rounding
 * the arc up, and a contradiction the eye catches immediately. The exact counts
 * are always in the label, so the arc is free to be the approximate one.
 */
const MIN_VISIBLE_ARC = 0.085

/**
 * Fractions of the ring for a MOC's collection, as two nested arcs drawn from
 * the same origin rather than three abutting segments: `started` (read plus
 * in-progress) is painted first and `read` over it, so the not-started
 * remainder is just the exposed track. That nesting is what keeps the three
 * bands seamless without any per-segment offset arithmetic.
 *
 * Clamped and ordered so `read <= started <= 1` whatever the server sent.
 */
export function mocRingSegments(p?: MocProgress | null): { read: number; started: number } {
  const total = Number(p?.total)
  if (!Number.isFinite(total) || total <= 0) return { read: 0, started: 0 }

  const clamp = (n: number) => (Number.isFinite(n) ? Math.min(Math.max(n, 0), total) : 0)
  const readCount = clamp(Number(p?.read))
  const startedCount = Math.min(readCount + clamp(Number(p?.in_progress)), total)

  // Zero stays zero — the floor makes small counts visible, it never invents a
  // band for a count that does not exist.
  const floor = (count: number) =>
    count === 0 ? 0 : Math.max(count / total, MIN_VISIBLE_ARC)

  const read = floor(readCount)

  // The blue band is what shows *past* the green one, so it needs its own floor
  // measured from the end of green — otherwise 1 read + 1 in progress out of 40
  // floors both to the same arc and green hides blue completely.
  const started =
    startedCount > readCount
      ? Math.max(floor(startedCount), read + MIN_VISIBLE_ARC)
      : Math.max(floor(startedCount), read)

  return { read: Math.min(read, 1), started: Math.min(started, 1) }
}
