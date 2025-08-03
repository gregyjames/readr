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
  const articleURL = `http://localhost:3000/articles/${articleID}`

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

  console.log(markdownContent.value)
})
</script>

<template>
  <div class="prose prose-base prose-p:text-justify prose-headings:text-black mx-auto max-w-3xl prose-pre:text-left" v-html="markdownContent" />
</template>
