<script setup lang="ts">
// Global host for the passkey ceremony popup. Mount once in App.vue (or
// the root layout); the SDK in src/passkey toggles its visibility.
// Teleports to body to escape any local stacking context — the popup is
// always centered on the viewport with a dimmed backdrop.
import { passkeyState } from '@/passkey/state'
import { Button, Input, Field, Icon } from '@/ui'

// During 'waiting' the backdrop is non-dismissive — the OS/authenticator
// dialog is the user's exit. In 'error' / 'naming' the user has explicit
// buttons; Esc also cancels (browsers fire keydown.escape on top-level).
function onBackdropKeydown(e: KeyboardEvent) {
  if (e.key !== 'Escape') return
  if (passkeyState.phase === 'waiting') return
  e.preventDefault()
  passkeyState.cancel()
}
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="passkeyState.active"
        class="fixed inset-0 z-[900] flex items-center justify-center bg-overlay-bg"
        tabindex="-1"
        @keydown="onBackdropKeydown"
      >
        <div
          role="dialog"
          aria-modal="true"
          class="relative w-full max-w-sm mx-4 bg-surface-0 border border-line rounded-lg shadow-lg p-6 flex flex-col gap-4"
        >
          <header class="flex items-baseline justify-between gap-3">
            <h2 class="text-base font-semibold text-ink">{{ passkeyState.title }}</h2>
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
