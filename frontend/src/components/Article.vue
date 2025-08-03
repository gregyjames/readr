<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { marked } from 'marked'

const markdownContent = ref('')
const articleURL = 'http://localhost:3000/articles/1754177659.md' // can be dynamic

onMounted(async () => {
  const res = await fetch(articleURL)
  const raw = await res.text()
  markdownContent.value = await marked.parse(raw)
})
</script>

<template>
  <div class="prose max-w-3xl mx-auto" v-html="markdownContent"></div>
</template>
