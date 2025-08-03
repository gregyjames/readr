<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { marked } from 'marked'

const markdownContent = ref('')
const route = useRoute()

onMounted(async () => {
  const articleID = route.params.id
  const articleURL = `http://localhost:3000/articles/${articleID}`
  const res = await fetch(articleURL)
  const raw = await res.text()
  markdownContent.value = await marked.parse(raw)
})
</script>

<template>
  <div class="prose max-w-3xl mx-auto p-6" v-html="markdownContent"></div>
</template>