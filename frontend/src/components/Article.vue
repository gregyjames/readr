<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'
import hljs from 'highlight.js'
import 'highlight.js/styles/github-dark.css'

const markdownContent = ref('')
const route = useRoute()

onMounted(async () => {
  const articleID = route.params.id
  const articleURL = `/articles/${articleID}`

  const res = await fetch(articleURL)
  const raw = await res.text()

  markdownContent.value = await marked.parse(raw, {
    gfm: false,
    async: true
  })

  // highlight after DOM update
  await nextTick()
  document.querySelectorAll('pre code').forEach((block) => {
    hljs.highlightElement(block as HTMLElement)
  })
})
</script>

<template>
  <article class="w-full max-w-2xl mx-auto py-10 transition-colors duration-300">
    <div class="prose prose-lg dark:prose-invert prose-p:text-gray-700 dark:prose-p:text-gray-300 prose-p:leading-relaxed prose-p:font-serif prose-headings:font-sans prose-headings:font-bold prose-headings:tracking-tight prose-headings:text-gray-900 dark:prose-headings:text-gray-100 prose-a:text-emerald-600 dark:prose-a:text-emerald-400 prose-a:no-underline hover:prose-a:underline prose-img:rounded-3xl prose-img:shadow-sm prose-pre:text-left prose-pre:bg-[#111] dark:prose-pre:bg-[#1a1a1a] prose-pre:rounded-2xl transition-colors duration-300" v-html="markdownContent" />
  </article>
</template>
