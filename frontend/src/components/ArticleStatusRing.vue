<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  status?: string
  progress?: number
  /** Smaller, text-free ring for the dense list-view meta row. */
  compact?: boolean
  /** Explicit pixel size, overriding the compact/default sizing. */
  size?: number
  /** Draw the percentage inside the ring. Off when a label sits beside it. */
  showValue?: boolean
  /** Inherit stroke from the parent's text color instead of per-status classes. */
  inheritColor?: boolean
}>(), {
  status: 'not_started',
  progress: 0,
  compact: false,
  showValue: true,
  inheritColor: false,
})

const RADIUS = 9
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

const clampedProgress = computed(() => {
  const value = Number(props.progress)
  if (!Number.isFinite(value)) return 0
  return Math.min(Math.max(value, 0), 100)
})

const isFinished = computed(() => props.status === 'finished')
const isNotStarted = computed(() => props.status === 'not_started')

// A finished article always shows a full ring, even if it was marked by hand
// from partial progress.
const arcFraction = computed(() => (isFinished.value ? 1 : clampedProgress.value / 100))

const dashOffset = computed(() => CIRCUMFERENCE * (1 - arcFraction.value))

// Full class strings, never interpolated: Tailwind only generates what it can
// find as a literal in the source.
const arcClass = computed(() => {
  if (props.inheritColor) return 'stroke-current'
  if (isFinished.value) return 'stroke-emerald-500 dark:stroke-emerald-400'
  return 'stroke-blue-500 dark:stroke-blue-400'
})

// Never-opened articles get a visible amber ring rather than the neutral track,
// so "not started" is a state you can see rather than an absence.
const trackClass = computed(() => {
  if (props.inheritColor) {
    // No arc is drawn when nothing has been read, so the track itself has to
    // carry the "not started" state. opacity-80 keeps it clear of the 3:1
    // non-text contrast floor against both the light row and the cover pill.
    return isNotStarted.value ? 'stroke-current opacity-80' : 'stroke-current opacity-25'
  }
  if (isNotStarted.value) return 'stroke-gray-300 dark:stroke-white/25'
  return 'stroke-gray-200 dark:stroke-white/10'
})

const textClass = computed(() => {
  if (isFinished.value) return 'fill-emerald-600 dark:fill-emerald-400'
  return 'fill-blue-600 dark:fill-blue-400'
})

const dimension = computed(() => props.size ?? (props.compact ? 18 : 24))

const roundedProgress = computed(() => Math.round(clampedProgress.value))

// Inner text only reads cleanly at the larger size, and only for two digits.
// A rounded 0 is suppressed everywhere: the arc has no length at 0, so "0"
// sitting inside an empty track reads as a broken control rather than as
// "nothing read yet". An empty ring says that on its own.
const showInnerPercent = computed(
  () => props.showValue && !props.compact && !isFinished.value && !isNotStarted.value && roundedProgress.value >= 1
)
const showAdjacentPercent = computed(
  () => props.showValue && props.compact && !isFinished.value && !isNotStarted.value && roundedProgress.value >= 1
)

// Doubles as the accessible name: the arc, the check and the inner percentage
// all live inside an aria-hidden <svg>, so without this the state is announced
// only in the one case that happens to render a text chip.
const title = computed(() => {
  if (isNotStarted.value) return 'Not started'
  if (isFinished.value) return 'Finished'
  return `${roundedProgress.value}% read`
})
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap"
    role="img"
    :aria-label="title"
    :title="title"
  >
    <svg
      :width="dimension"
      :height="dimension"
      viewBox="0 0 24 24"
      fill="none"
      class="shrink-0 -rotate-90"
      aria-hidden="true"
    >
      <!-- Track -->
      <circle
        cx="12"
        cy="12"
        :r="RADIUS"
        stroke-width="2.5"
        :class="trackClass"
      />
      <!-- Progress arc -->
      <circle
        v-if="!isNotStarted"
        cx="12"
        cy="12"
        :r="RADIUS"
        stroke-width="2.5"
        stroke-linecap="round"
        :stroke-dasharray="CIRCUMFERENCE"
        :stroke-dashoffset="dashOffset"
        :class="arcClass"
        class="transition-[stroke-dashoffset] duration-500 ease-out motion-reduce:transition-none"
      />
      <!-- Check for finished; the group is un-rotated so glyphs sit upright -->
      <g class="rotate-90 origin-center">
        <path
          v-if="isFinished"
          d="M8.5 12.2 L10.9 14.6 L15.5 9.8"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          :class="arcClass"
        />
        <text
          v-else-if="showInnerPercent"
          x="12"
          y="12"
          text-anchor="middle"
          dominant-baseline="central"
          font-size="8.5"
          class="font-mono font-medium"
          :class="textClass"
        >{{ roundedProgress }}</text>
      </g>
    </svg>

    <span
      v-if="isNotStarted && showValue"
      class="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-gray-500/10 text-gray-600 dark:text-gray-300 border border-gray-500/20"
    >
      New!
    </span>
    <span
      v-else-if="showAdjacentPercent"
      class="text-[11px] font-mono text-blue-600 dark:text-blue-400"
    >
      {{ roundedProgress }}%
    </span>
  </span>
</template>
