<script setup lang="ts">
// Shown when the user clicked "无 Passkey 登录" → "否" — no working device,
// admin-issued recovery link is the only path.
</script>

<template>
  <div class="bg-surface-0 border border-line rounded-lg shadow-sm p-8 w-full max-w-sm flex flex-col gap-4">
    <h1 class="text-xl font-semibold text-ink">联系管理员</h1>
    <p class="text-sm text-ink-muted">
      请管理员在「用户管理」页面找到您的账户，点击「获取恢复链接」生成一次性链接，
      并通过可信渠道发给您。在本设备上打开该链接即可注册新的 Passkey。
    </p>
    <div class="rounded-md bg-err-faint text-err-ink text-xs px-3 py-2">
      使用恢复链接后，账户上现有的所有 Passkey 都会被删除。
    </div>
    <div class="pt-4 mt-2 border-t border-line">
      <RouterLink to="/login/recover" class="text-sm text-accent hover:underline">返回上一步</RouterLink>
    </div>
  </div>
</template>
