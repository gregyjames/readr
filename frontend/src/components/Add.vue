<script setup lang="ts">
import { ref } from 'vue'
const url = ref('')

const submitForm = async () => {
  await fetch('http://localhost:3000/add', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url: url.value }),
  })
}
</script>

<template>
    <h1 class="mb-4 text-4xl font-extrabold leading-none tracking-tight text-gray-900 md:text-5xl lg:text-6xl dark:text-white">Add</h1>
    <form @submit.prevent="submitForm" class="bg-white shadow-md rounded-lg p-6 w-full max-w-md space-y-4">
        <h2 class="text-xl font-semibold text-gray-800">Submit a URL</h2>

        <!-- URL Input -->
        <div>
            <label for="url" class="block text-sm font-medium text-gray-700 mb-1">Website URL</label>
            <input
                v-model="url"
                type="url"
                id="url"
                name="url"
                placeholder="https://example.com"
                required
                class="w-full px-4 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
        </div>

        <!-- Optional error message -->
        <p class="text-red-500 text-sm hidden" id="error-message">
            Please enter a valid URL.
        </p>

        <!-- Submit Button -->
        <button type="submit" class="w-full bg-blue-600 text-white py-2 rounded-md hover:bg-blue-700 transition">Submit</button>
  </form>

</template>