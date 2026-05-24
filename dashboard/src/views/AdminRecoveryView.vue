<script setup lang="ts">
// Shown when the user clicked "无 Passkey 登录" → "否" — no working device,
// admin-issued recovery link is the only path.
</script>

<template>
  <div class="bg-surface-0 border border-line rounded-lg shadow-sm p-8 w-full max-w-sm flex flex-col gap-4">
    <h1 class="text-xl font-semibold text-ink">联系管理员</h1>
    <ol class="flex flex-col gap-2 text-sm text-ink-muted list-decimal list-inside pl-1">
      <li>通过可信渠道联系管理员。</li>
      <li>管理员会生成一次性恢复链接并发送给您。</li>
      <li>在本设备上打开恢复链接，按提示注册新的 Passkey。</li>
    </ol>
    <div class="rounded-md bg-err-faint text-err-ink text-xs px-3 py-2">
      使用恢复链接将会删除您的账号上现有的所有 Passkey。
    </div>
    <div class="pt-4 mt-2 border-t border-line">
      <RouterLink to="/login/recover" class="text-sm text-accent hover:underline">返回上一步</RouterLink>
    </div>
  </div>
</template>
