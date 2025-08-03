<script setup lang="ts">
import BookmarkIcon from './assets/book.svg'
</script>

<template>
  <nav class="bg-green-600 w-full shadow-md fixed top-0 left-0 z-50">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex justify-between items-center h-16">
        <div class="flex items-center space-x-2">
          <!-- SVG or Image Logo -->
          <BookmarkIcon class="w-6 h-6 text-white" />
      
          <!-- Text -->
          <span class="text-white text-lg font-semibold">Readr</span>
        </div>

        <!-- Menu -->
        <div class="flex space-x-6">
          <router-link to="/" class="text-white hover:underline">Home</router-link>
          <router-link to="/add" class="text-white hover:underline">Add</router-link>
        </div>
      </div>
    </div>
  </nav>
  <div class="container w-full">
    <router-view />
  </div>
</template>
