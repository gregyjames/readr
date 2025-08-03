<script setup lang="ts">
import { ref } from 'vue'
const url = ref('')
const isLoading = ref(false)

const submitForm = async () => {
  isLoading.value = true
  try{
    await fetch('http://localhost:3000/add', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ url: url.value }),
    })
  }
  finally{
    isLoading.value = false
  }
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
        <button type="submit" class="w-full bg-blue-600 text-white py-2 rounded-md hover:bg-blue-700 transition" :disabled="isLoading">Submit</button>

        <span v-if="!isLoading">Submit</span>
        <svg v-else class="animate-spin h-5 w-5 text-white" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 00-8 8z"/>
        </svg>
  </form>

</template>