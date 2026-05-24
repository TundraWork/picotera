<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Button } from '@/ui'

const router = useRouter()
</script>

<template>
  <div class="bg-surface-0 border border-line rounded-lg shadow-sm p-8 w-full max-w-sm flex flex-col gap-5">
    <h1 class="text-xl font-semibold text-ink">无 Passkey 登录</h1>
    <p class="text-sm text-ink-muted">
      您现在是否有任何一台已登录 PicoTera 的其他设备？
    </p>
    <div class="flex flex-col gap-2">
      <Button @click="router.push('/login/code')">是</Button>
      <Button variant="ghost" @click="router.push('/login/recover/admin')">否</Button>
    </div>
    <div class="pt-4 border-t border-line">
      <RouterLink to="/login" class="text-sm text-accent hover:underline">返回登录</RouterLink>
    </div>
  </div>
</template>
