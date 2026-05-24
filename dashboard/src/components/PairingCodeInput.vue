<script setup lang="ts">
// 8-char pairing code input. Accepts user input with or without the visual
// XXXX-XXXX separator, strips anything that isn't alphanumeric, normalizes
// to uppercase, caps at 8 characters. v-model is the normalized string;
// the displayed value re-inserts the hyphen after the 4th char so the user
// can see how the code lines up while typing.
//
// Used in PairApproveDialog. Emits "enter" so the parent can submit on
// Return without manually wiring keydown handling.
import { computed } from 'vue'

const props = defineProps<{ modelValue: string }>()
const emit = defineEmits<{
  'update:modelValue': [value: string]
  enter: []
}>()

const displayValue = computed(() => {
  const v = props.modelValue
  if (v.length <= 4) return v
  return v.slice(0, 4) + '-' + v.slice(4, 8)
})

function onInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value
  const normalized = raw.replace(/[^A-Za-z0-9]/g, '').toUpperCase().slice(0, 8)
  emit('update:modelValue', normalized)
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    e.preventDefault()
    emit('enter')
  }
}
</script>

<template>
  <input
    :value="displayValue"
    type="text"
    autocomplete="off"
    autocorrect="off"
    autocapitalize="characters"
    spellcheck="false"
    inputmode="text"
    maxlength="9"
    placeholder="ABCD-EFGH"
    class="font-mono text-2xl tracking-widest text-center text-ink tabular-nums border border-line rounded-md bg-surface-0 px-4 py-3 w-full transition-colors hover:border-surface-300 focus:outline-none focus:border-accent focus:ring-[3px] focus:ring-accent/20 placeholder:text-ink-faint placeholder:tracking-widest"
    @input="onInput"
    @keydown="onKeydown"
  />
</template>
