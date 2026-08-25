<script setup lang="ts">
withDefaults(defineProps<{ padded?: boolean; compact?: boolean }>(), {
  padded: true,
  compact: false,
})
</script>

<template>
  <main class="mobile-shell" :class="{ 'mobile-shell--padded': padded, 'mobile-shell--compact': compact }">
    <slot />
  </main>
</template>
