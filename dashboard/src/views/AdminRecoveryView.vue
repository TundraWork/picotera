<script setup lang="ts">
// Shown when the user clicked "无 Passkey 登录" → "否" — they have no
// working device, so admin-issued recovery link is the only path.
</script>

<template>
  <div class="bg-surface-0 border border-line rounded-lg shadow-sm p-8 w-full max-w-sm flex flex-col gap-4">
    <h1 class="text-xl font-semibold text-ink">联系管理员</h1>
    <p class="text-sm text-ink-muted">
      请按以下步骤请管理员为您生成一次性恢复链接：
    </p>
    <ol class="flex flex-col gap-2 text-sm text-ink-muted list-decimal list-inside pl-1">
      <li>通过可信渠道联系管理员（当面、电话或工作即时通讯）。</li>
      <li>请管理员打开「用户管理」页面，找到您的账户。</li>
      <li>请管理员点击该用户行上的「获取恢复链接」按钮。</li>
      <li>管理员会复制生成的一次性链接，通过可信渠道发给您。</li>
      <li>在本设备上打开该链接，按提示注册新的 Passkey 即可登录。</li>
    </ol>
    <div class="rounded-md bg-err-faint text-err-ink text-xs px-3 py-2">
      恢复链接会删除该账户上现有的所有 Passkey。请确认这是您本人发起的请求。
    </div>
    <div class="pt-4 mt-2 border-t border-line">
      <RouterLink to="/login/recover" class="text-sm text-accent hover:underline">返回上一步</RouterLink>
    </div>
  </div>
</template>
