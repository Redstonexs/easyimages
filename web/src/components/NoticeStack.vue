<script setup lang="ts">
import type { Notice } from '../shared/notify'

defineProps<{
  notices: Notice[]
}>()
</script>

<template>
  <div class="ei-notices" aria-live="polite" aria-atomic="true">
    <div v-for="notice in notices" :key="notice.id" class="ei-notice" :class="notice.type">
      {{ notice.message }}
    </div>
  </div>
</template>
