<script setup lang="ts">
import { computed } from 'vue'
import ArticleStatusRing from './ArticleStatusRing.vue'

/**
 * The single reading indicator for an article in the vault, sitting in the
 * cover badge row where reading time used to be.
 *
 *   never opened  →  ◌ New!   (neutral)
 *   in progress   →  ◔ 60%    (blue)
 *   finished      →  ✓ Read   (green)
 *
 * The ring inherits the wrapper's text color, so one class per state drives
 * both the glyph and the label, and the percentage is never drawn inside the
 * ring — the number already sits beside it.
 */
const props = withDefaults(defineProps<{
  status?: string
  progress?: number
  /**
   * 'overlay' sits on the dark translucent pill over a cover image, which is
   * dark in both themes; 'meta' sits in a normal text row and needs a
   * light/dark pair.
   */
  variant?: 'overlay' | 'meta'
  /** Override the glyph size; defaults to the variant's optical match. */
  size?: number
}>(), {
  status: 'not_started',
  progress: 0,
  variant: 'meta',
})

const isFinished = computed(() => props.status === 'finished')
const isInProgress = computed(() => props.status === 'not_finished')
const percent = computed(() => Math.round(Number(props.progress) || 0))

// Full class strings, never interpolated: Tailwind only generates literals.
const TONES = {
  overlay: {
    // Neutral, not amber: "new" is the default state, so it should recede and
    // leave blue/green as the only saturated marks on the page. White/75 on the
    // dark cover pill reads as quiet secondary text rather than disabled.
    not_started: 'text-white/75',
    not_finished: 'text-blue-300',
    finished: 'text-emerald-300',
  },
  meta: {
    not_started: 'text-gray-500 dark:text-gray-400',
    not_finished: 'text-blue-600 dark:text-blue-400',
    finished: 'text-emerald-600 dark:text-emerald-400',
  },
} as const

// The label used to inherit its type from whatever row it landed in — 12px on
// the lead card, 11px in list rows, 10px in cover pills — so the same state
// rendered at three sizes against a fixed 14px ring. Each variant now owns its
// scale: 10px matches the other cover-pill badges, 11px the meta rows.
const SIZES = {
  overlay: 'text-[10px]',
  meta: 'text-[11px]',
} as const

// The ring's drawn circle fills ~85% of its box, while mono text only reaches
// its cap height (~0.7em). Matching box heights therefore leaves the ring
// looking far heavier than the glyphs, so size it just above the cap height.
const GLYPH_SIZES = { overlay: 11, meta: 12 } as const

const glyphSize = computed(() => props.size ?? GLYPH_SIZES[props.variant])

const toneClass = computed(() => {
  const key = isFinished.value ? 'finished' : isInProgress.value ? 'not_finished' : 'not_started'
  return [TONES[props.variant][key], SIZES[props.variant]]
})

const title = computed(() => {
  if (isFinished.value) return 'Finished'
  if (isInProgress.value) return `${percent.value}% read`
  return 'Not started'
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1 shrink-0 whitespace-nowrap font-mono font-medium"
    :class="toneClass"
    role="img"
    :aria-label="title"
    :title="title"
  >
    <svg
      v-if="isFinished"
      class="shrink-0"
      :width="glyphSize"
      :height="glyphSize"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2.5"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <polyline points="20 6 9 17 4 12"></polyline>
    </svg>
    <ArticleStatusRing
      v-else
      :status="status"
      :progress="progress"
      :size="glyphSize"
      :show-value="false"
      inherit-color
    />

    <span v-if="isFinished">Read</span>
    <span v-else-if="isInProgress">{{ percent }}%</span>
    <span v-else>New!</span>
  </span>
</template>
