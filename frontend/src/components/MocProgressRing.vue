<script setup lang="ts">
import { computed } from 'vue'
import { mocRingSegments, type MocProgress } from '../utils/moc'

/**
 * The segmented ring for a Map of Content's collection.
 *
 * A sibling of ArticleStatusRing rather than a mode on it: this ring shows two
 * colors at once, so it cannot use that component's single inherited stroke,
 * and its arcs are counts of notes rather than one document's scroll position.
 * Geometry is copied deliberately — same radius, box and stroke width — so the
 * two rings sit at identical optical weight in the same card row.
 */
const props = withDefaults(defineProps<{
  progress?: MocProgress | null
  /**
   * 'overlay' sits on the dark translucent pill over a cover image, which is
   * dark in both themes; 'meta' sits in a normal text row and needs a
   * light/dark pair.
   */
  variant?: 'overlay' | 'meta'
  size?: number
}>(), {
  variant: 'meta',
  size: 14,
})

// Thicker and slightly tighter than ArticleStatusRing's 9/2.5. A hub's ring has
// to resolve a tenth of a turn — 36 degrees — where the article ring only ever
// shows one arc whose value is restated verbatim in the number beside it. At
// r=9/stroke=2.5 rendered at 12px, that tenth is a 2.8px speck on a 1.25px
// stroke and the ring reads as empty. 8.25 + 3.5/2 = 10 keeps the extent inside
// the 24-unit box.
const RADIUS = 8.25
const STROKE = 3.5
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

const segments = computed(() => mocRingSegments(props.progress))

const startedOffset = computed(() => CIRCUMFERENCE * (1 - segments.value.started))
const readOffset = computed(() => CIRCUMFERENCE * (1 - segments.value.read))

// Full class strings, never interpolated: Tailwind only generates literals.
//
// The track is not decorative — it encodes the unread remainder, and on a hub
// whose arcs are both tiny it is the only thing drawn. gray-300 on a white card
// is ~1.5:1, well under the 3:1 non-text floor, so it goes a step darker.
const TONES = {
  overlay: {
    track: 'stroke-white/40',
    started: 'stroke-blue-300',
    read: 'stroke-emerald-300',
  },
  meta: {
    track: 'stroke-gray-400 dark:stroke-white/30',
    started: 'stroke-blue-500 dark:stroke-blue-400',
    read: 'stroke-emerald-500 dark:stroke-emerald-400',
  },
} as const

const tone = computed(() => TONES[props.variant])
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    class="shrink-0 -rotate-90"
    aria-hidden="true"
  >
    <!-- Track: the not-started remainder, left exposed by the arcs above it -->
    <circle cx="12" cy="12" :r="RADIUS" :stroke-width="STROKE" :class="tone.track" />
    <!--
      Started and read are nested arcs from the same origin, painted in that
      order. Butt caps, not the round caps ArticleStatusRing uses: a round cap
      would overhang the boundary and let green bleed past its own count.

      No transition, unlike ArticleStatusRing: that ring fills live as you
      scroll your own reading, while a hub's counts only ever change between
      page loads. Animating them would sweep arcs across a grid on re-render
      for no reason a reader could connect to an action.
    -->
    <circle
      v-if="segments.started > 0"
      cx="12"
      cy="12"
      :r="RADIUS"
      :stroke-width="STROKE"
      stroke-linecap="butt"
      :stroke-dasharray="CIRCUMFERENCE"
      :stroke-dashoffset="startedOffset"
      :class="tone.started"
    />
    <circle
      v-if="segments.read > 0"
      cx="12"
      cy="12"
      :r="RADIUS"
      :stroke-width="STROKE"
      stroke-linecap="butt"
      :stroke-dasharray="CIRCUMFERENCE"
      :stroke-dashoffset="readOffset"
      :class="tone.read"
    />
  </svg>
</template>
