<script setup lang="ts">
import { computed } from 'vue'
import MocProgressRing from './MocProgressRing.vue'
import type { MocProgress } from '../utils/moc'

/**
 * The reading indicator for a Map of Content, standing where
 * ArticleProgressLabel stands on a plain note.
 *
 * A hub is a collection, not reading material — it never accumulates a status
 * row of its own — so it reports how much of its collection is read rather
 * than how far you scrolled through the index:
 *
 *   nothing read   →  ◌ 0/10 notes    (neutral)
 *   partly read    →  ◕ 4/10 notes    (neutral, green + blue bands)
 *   fully read     →  ● 10/10 notes   (green)
 *
 * "notes", not "read": the same meta row already carries "5 min read" a few
 * characters away, and one row should not use one word for a duration and a
 * count of finished things. The ring and the tooltip carry "read".
 *
 * Renders nothing at all when the hub has no resolvable members: the ★ MOC
 * badge already identifies the card, and an empty 0/0 ring would only be noise.
 * That check living here is what keeps every call site a plain v-if on isMoc.
 */
const props = withDefaults(defineProps<{
  progress?: MocProgress | null
  /**
   * 'overlay' sits on the dark translucent pill over a cover image, which is
   * dark in both themes; 'meta' sits in a normal text row; 'reader' is the
   * sticky reader toolbar, where there is room for a larger glyph and the
   * ★ MOC HUB badge has scrolled out of view.
   */
  variant?: 'overlay' | 'meta' | 'reader'
  /** Override the glyph size; defaults to the variant's optical match. */
  size?: number
}>(), {
  variant: 'meta',
})

const total = computed(() => Number(props.progress?.total) || 0)
const read = computed(() => Math.min(Number(props.progress?.read) || 0, total.value))
const inProgress = computed(() =>
  Math.min(Number(props.progress?.in_progress) || 0, total.value - read.value)
)

const hasMembers = computed(() => total.value > 0)
const isComplete = computed(() => hasMembers.value && read.value === total.value)

// Full class strings, never interpolated: Tailwind only generates literals.
//
// Two tones, not the article label's three: no blue. On an article, blue is the
// state — "60%" in gray would say nothing. Here the state is already spelled out
// as a fraction, and blue would fight the ring rather than agree with it: a hub
// with 1 read and 0 in progress has a green-only ring, and blue text beside it
// states a second, contradictory thing. The extreme is 0 read / 1 in progress —
// a saturated "active" chip whose number is zero. Blue stays in the ring, where
// it sits against green at proportional length and can be compared.
const TONES = {
  overlay: {
    idle: 'text-white/75',
    complete: 'text-emerald-300',
  },
  meta: {
    idle: 'text-gray-500 dark:text-gray-400',
    complete: 'text-emerald-600 dark:text-emerald-400',
  },
  reader: {
    idle: 'text-gray-600 dark:text-gray-300',
    complete: 'text-emerald-600 dark:text-emerald-400',
  },
} as const

// Text scales with the glyph. An 11px mono cap height is ~7.7px, so pairing it
// with an 18px ring leaves a small number hanging off a big circle.
const SIZES = {
  overlay: 'text-[10px]',
  meta: 'text-[11px]',
  reader: 'text-xs',
} as const

// A notch above ArticleProgressLabel's 11/12 on purpose: this ring resolves
// tenths of a turn, so it needs the extra pixels the article ring does not.
const GLYPH_SIZES = { overlay: 13, meta: 14, reader: 18 } as const

const glyphSize = computed(() => props.size ?? GLYPH_SIZES[props.variant])

// The ring has two palettes, not three: 'reader' sits on a normal light/dark
// surface, so it takes the same strokes as 'meta'.
const ringVariant = computed(() => (props.variant === 'overlay' ? 'overlay' : 'meta'))

const toneClass = computed(() => {
  const key = isComplete.value ? 'complete' : 'idle'
  return [TONES[props.variant][key], SIZES[props.variant]]
})

// Doubles as the accessible name: the bands live inside an aria-hidden <svg>,
// and "4/10" alone does not say what was counted.
const title = computed(() => {
  const noun = total.value === 1 ? 'note' : 'notes'
  if (isComplete.value) return `All ${total.value} ${noun} read`
  if (inProgress.value > 0) {
    return `${read.value} of ${total.value} ${noun} read, ${inProgress.value} in progress`
  }
  return `${read.value} of ${total.value} ${noun} read`
})
</script>

<template>
  <span
    v-if="hasMembers"
    class="inline-flex items-center gap-1 shrink-0 whitespace-nowrap font-mono font-medium"
    :class="toneClass"
    role="img"
    :aria-label="title"
    :title="title"
  >
    <MocProgressRing :progress="progress" :variant="ringVariant" :size="glyphSize" />
    <!-- The reader spells the unit out: its ★ MOC HUB badge scrolls away. -->
    <span v-if="variant === 'reader'">{{ read }} of {{ total }} notes read</span>
    <span v-else>{{ read }}/{{ total }} notes</span>
  </span>
</template>
