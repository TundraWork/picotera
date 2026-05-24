<script setup lang="ts">
// Display the 8-char pairing code in a consistent visual style across every
// place it appears (CodeLoginView while polling, PairApproveDialog confirm
// view). Accepts the formatted "XXXX-XXXX" form straight from the server.
//
// Size: "lg" for the new-device main screen, "md" for the side panel.
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{ code: string; size?: 'lg' | 'md' }>(),
  { size: 'lg' },
)

const sizeClass = computed(() => (props.size === 'md' ? 'text-2xl' : 'text-3xl'))
</script>

<template>
  <div
    :class="[
      'font-mono tracking-widest text-ink tabular-nums select-all text-center',
      sizeClass,
    ]"
  >{{ code }}</div>
</template>
