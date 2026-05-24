<script setup lang="ts">
// Global host for the passkey ceremony popup. Mount once in App.vue; the
// SDK in src/passkey toggles its visibility. Teleports to body so the
// popup escapes any local stacking context.
//
// Accessibility (per W3C ARIA Authoring Practices + WCAG 2.1.2 No
// Keyboard Trap):
//   • role="dialog" + aria-modal="true" so AT announces it as a modal.
//   • aria-labelledby points at the title h2 inside the dialog.
//   • Focus moves into the dialog when it opens; loops with Tab /
//     Shift+Tab (focus trap); restores to the prior element on close.
//   • Esc closes (also goes through the SDK's cancel handler, which
//     aborts the in-flight WebAuthn ceremony via AbortController).
//   • Backdrop click closes when not in 'waiting' — we don't dismiss
//     mid-OS-prompt because some authenticators don't honor the abort
//     reliably. (Esc still works; users can also use the Cancel button.)
//   • Body scroll is locked while the popup is open.

import { ref, watch, nextTick } from 'vue'
import { passkeyState } from '@/passkey/state'
import { Button, Input, Field, Icon } from '@/ui'

const dialogRef = ref<HTMLDivElement | null>(null)
const titleId = 'passkey-popup-title'

// Element to restore focus to on close.
let previouslyFocused: HTMLElement | null = null

function focusableWithinDialog(): HTMLElement[] {
  if (!dialogRef.value) return []
  return Array.from(
    dialogRef.value.querySelectorAll<HTMLElement>(
      'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ),
  )
}

// Re-focus the first interactive element whenever the phase transitions
// inside an open dialog — going from 'waiting' to 'naming' or 'error'
// swaps the rendered branch, so the previous focus target is gone.
watch(
  () => passkeyState.phase,
  async () => {
    if (!passkeyState.active) return
    await nextTick()
    focusableWithinDialog()[0]?.focus()
  },
)

watch(
  () => passkeyState.active,
  async (active) => {
    if (active) {
      // Capture trigger element BEFORE focus shifts.
      const cur = document.activeElement
      previouslyFocused = cur instanceof HTMLElement ? cur : null
      // Body scroll lock — overlay covers the viewport, scrolling underneath
      // is just a distraction.
      document.body.style.overflow = 'hidden'
      // Wait one tick for the dialog to mount, then focus its first
      // interactive element (or the dialog itself as a last resort).
      await nextTick()
      const items = focusableWithinDialog()
      const first = items[0]
      if (first) {
        first.focus()
      } else if (dialogRef.value) {
        dialogRef.value.focus()
      }
    } else {
      document.body.style.overflow = ''
      previouslyFocused?.focus()
      previouslyFocused = null
    }
  },
)

// Tab / Shift+Tab loop — keep focus inside the dialog so AT users can't
// land on background controls under the modal.
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    e.preventDefault()
    e.stopPropagation()
    passkeyState.cancel()
    return
  }
  if (e.key !== 'Tab') return
  const items = focusableWithinDialog()
  if (items.length === 0) {
    e.preventDefault()
    dialogRef.value?.focus()
    return
  }
  const first = items[0]
  const last = items[items.length - 1]
  if (!first || !last) return
  const active = document.activeElement as HTMLElement | null
  if (e.shiftKey) {
    if (active === first || !dialogRef.value?.contains(active)) {
      e.preventDefault()
      last.focus()
    }
  } else {
    if (active === last || !dialogRef.value?.contains(active)) {
      e.preventDefault()
      first.focus()
    }
  }
}

function onBackdropClick() {
  // Don't dismiss while waiting on the OS authenticator — some
  // implementations (e.g. certain browser extensions) don't honor the
  // AbortController signal cleanly mid-prompt. The Cancel button is
  // still available for users who want out.
  if (passkeyState.phase === 'waiting') return
  passkeyState.cancel()
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="passkeyState.active"
        class="fixed inset-0 z-[900] flex items-center justify-center bg-overlay-bg"
        @click.self="onBackdropClick"
        @keydown="onKeydown"
      >
        <div
          ref="dialogRef"
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          tabindex="-1"
          class="relative w-full max-w-sm mx-4 bg-surface-0 border border-line rounded-lg shadow-lg p-6 flex flex-col gap-4 outline-none"
        >
          <header class="flex items-baseline justify-between gap-3">
            <h2 :id="titleId" class="text-base font-semibold text-ink">
              {{ passkeyState.title }}
            </h2>
            <span
              v-if="passkeyState.subtitle"
              class="text-xs text-ink-faint"
            >{{ passkeyState.subtitle }}</span>
          </header>

          <!-- Waiting: spinner while begin → authenticator → complete is in flight -->
          <div
            v-if="passkeyState.phase === 'waiting'"
            class="flex flex-col items-center gap-3 py-6"
          >
            <div class="w-12 h-12 rounded-full border-2 border-line border-t-accent animate-spin"></div>
            <p class="text-sm text-ink-muted text-center">
              请使用您的 Passkey 完成{{ passkeyState.mode === 'enroll' ? '注册' : '验证' }}…
            </p>
            <p class="text-xs text-ink-faint text-center">
              请在浏览器或密码管理器弹窗中操作。
            </p>
            <div class="flex justify-center w-full">
              <Button variant="ghost" @click="passkeyState.cancel">取消</Button>
            </div>
          </div>

          <!-- Naming: optional post-enroll nickname step -->
          <div
            v-else-if="passkeyState.phase === 'naming'"
            class="flex flex-col gap-3"
          >
            <div class="flex items-center gap-2 text-success">
              <Icon name="check" :size="16" />
              <span class="text-sm font-medium">Passkey 注册成功</span>
            </div>
            <Field label="昵称（可选）">
              <Input
                v-model="passkeyState.nickname"
                maxlength="60"
                placeholder="例如 Laptop"
                autofocus
                @keydown.enter="passkeyState.submitNickname"
              />
            </Field>
            <div
              v-if="passkeyState.error"
              class="bg-err-faint text-err-ink rounded-md px-3 py-2 text-sm"
            >{{ passkeyState.error }}</div>
            <div class="flex justify-end gap-2">
              <Button
                variant="ghost"
                :disabled="passkeyState.saving"
                @click="passkeyState.skipNickname"
              >跳过</Button>
              <Button
                :disabled="passkeyState.saving"
                @click="passkeyState.submitNickname"
              >完成</Button>
            </div>
          </div>

          <!-- Error: with Retry / Cancel -->
          <div
            v-else-if="passkeyState.phase === 'error'"
            class="flex flex-col gap-3"
          >
            <div class="bg-err-faint text-err-ink rounded-md px-3 py-2 text-sm">
              {{ passkeyState.error }}
            </div>
            <div class="flex justify-end gap-2">
              <Button variant="ghost" @click="passkeyState.cancel">取消</Button>
              <Button @click="passkeyState.retry">重试</Button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.12s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
