<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Button } from '@/ui'

const router = useRouter()
</script>

<template>
  <div class="bg-surface-0 border border-line rounded-lg shadow-sm p-8 w-full max-w-sm flex flex-col gap-4">
    <h1 class="text-xl font-semibold text-ink">无 Passkey 登录</h1>
    <p class="text-sm text-ink-muted">
      您现在是否有任何一台已登录 PicoTera 的其他设备？
    </p>
    <div class="flex flex-col gap-2">
      <Button @click="router.push('/login/code')">
        <span>有，我可以在另一台设备上操作</span>
      </Button>
      <p class="text-xs text-ink-faint -mt-1">
        我们会在此页面显示一个配对码；在已登录的设备上输入即可。
      </p>
    </div>
    <div class="flex flex-col gap-2 mt-2">
      <Button variant="ghost" disabled>
        <span>没有，所有设备都已失效</span>
      </Button>
      <p class="text-xs text-ink-faint -mt-1">
        请联系管理员，在「用户管理」页面找到您的账户并点击「获取恢复链接」，将一次性链接发给您。
      </p>
    </div>
    <div class="pt-4 mt-2 border-t border-line">
      <RouterLink to="/login" class="text-sm text-accent hover:underline">返回登录</RouterLink>
    </div>
  </div>
</template>
