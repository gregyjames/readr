<script setup lang="ts">
import { computed, ref, onMounted, watch, nextTick } from 'vue'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'
import type { Message } from '../../types/chat'

const props = withDefaults(
  defineProps<{
    message: Message
    isStreaming?: boolean
    isLast?: boolean
  }>(),
  {
    isStreaming: false,
    isLast: false
  }
)

const proseContainer = ref<HTMLElement | null>(null)

const renderedContent = computed(() => {
  if (!props.message.content) return ''
  try {
    const html = marked.parse(props.message.content, { async: false })
    return typeof html === 'string' ? html : ''
  } catch (err) {
    return props.message.content
  }
})

const highlightCodeBlocks = () => {
  if (proseContainer.value) {
    proseContainer.value.querySelectorAll('pre code').forEach((block) => {
      hljs.highlightElement(block as HTMLElement)
    })
  }
}

onMounted(() => {
  highlightCodeBlocks()
})

watch(
  () => props.message.content,
  () => {
    nextTick(() => {
      highlightCodeBlocks()
    })
  }
)
</script>

<template>
  <div class="flex flex-col space-y-2">
    <!-- User Bubble -->
    <div v-if="message.role === 'user'" class="flex justify-end">
      <div class="max-w-2xl bg-[#111] dark:bg-white text-white dark:text-[#111] px-5 py-3.5 rounded-2xl rounded-tr-sm shadow-sm space-y-2">
        <!-- Attachments in User Bubble -->
        <div v-if="message.attachments && message.attachments.length > 0" class="flex flex-wrap gap-1.5 pb-1">
          <span
            v-for="att in message.attachments"
            :key="att.id"
            class="inline-flex items-center gap-1 bg-white/20 dark:bg-black/15 text-white dark:text-[#111] text-xs px-2.5 py-1 rounded-full font-medium"
          >
            📄 {{ att.title }}
          </span>
        </div>
        <div class="text-sm whitespace-pre-wrap leading-relaxed">{{ message.content }}</div>
      </div>
    </div>

    <!-- Assistant Bubble -->
    <div v-else-if="message.role === 'assistant'" class="flex justify-start">
      <div class="max-w-3xl bg-gray-50/90 dark:bg-[#161616] border border-gray-200/60 dark:border-gray-800/60 text-gray-900 dark:text-gray-100 px-5 py-4 rounded-2xl rounded-tl-sm shadow-xs space-y-2 w-full sm:w-auto">
        <div class="flex items-center gap-2 mb-2 pb-2 border-b border-gray-200/40 dark:border-gray-800/40">
          <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
          <span class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-gray-400">Readr Assistant</span>
        </div>
        <!-- Rendered Markdown -->
        <div
          ref="proseContainer"
          class="chat-prose text-sm leading-relaxed max-w-none text-gray-800 dark:text-gray-200"
          v-html="renderedContent"
        ></div>
        <div v-if="isStreaming && isLast" class="flex items-center gap-1.5 pt-1 text-emerald-600 dark:text-emerald-400 text-xs">
          <span class="w-1.5 h-1.5 bg-emerald-500 rounded-full animate-pulse"></span>
          <span>Generating response...</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style>
/* Prose markdown formatting inside chat */
.chat-prose p {
  margin-bottom: 0.75rem;
}
.chat-prose p:last-child {
  margin-bottom: 0;
}
.chat-prose ul, .chat-prose ol {
  margin-left: 1.25rem;
  margin-bottom: 0.75rem;
}
.chat-prose ul {
  list-style-type: disc;
}
.chat-prose ol {
  list-style-type: decimal;
}
.chat-prose li {
  margin-bottom: 0.25rem;
}
.chat-prose code {
  background-color: rgba(120, 120, 120, 0.15);
  padding: 0.15rem 0.35rem;
  border-radius: 0.25rem;
  font-family: monospace;
  font-size: 0.85em;
}
.chat-prose pre {
  background-color: #0d1117;
  color: #c9d1d9;
  padding: 0.75rem 1rem;
  border-radius: 0.75rem;
  overflow-x: auto;
  margin-top: 0.5rem;
  margin-bottom: 0.75rem;
}
.chat-prose pre code {
  background-color: transparent;
  padding: 0;
  color: inherit;
}
.chat-prose h1, .chat-prose h2, .chat-prose h3, .chat-prose h4 {
  font-weight: 600;
  margin-top: 1rem;
  margin-bottom: 0.5rem;
}
.chat-prose blockquote {
  border-left: 3px solid #10b981;
  padding-left: 0.75rem;
  margin-left: 0;
  margin-bottom: 0.75rem;
  opacity: 0.85;
}
</style>
