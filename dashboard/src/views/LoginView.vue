<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useQuery, useQueryClient } from '@tanstack/vue-query'
import {
  fetchAuthStatus,
  beginLogin,
  completeLogin,
  ApiRequestError,
} from '@/api/client'
import { queryKeys } from '@/api/queryKeys'
import { OPERATIONAL_STALE_TIME } from '@/api/queryClient'
import { Button } from '@/ui'
import { fallbackFor } from '@/router/fallback'
import * as passkey from '@/passkey'

const route = useRoute()
const router = useRouter()
const qc = useQueryClient()
const errorMessage = ref<string | null>(null)

const statusQuery = useQuery({
  queryKey: queryKeys.authStatus.all,
  queryFn: fetchAuthStatus,
  staleTime: OPERATIONAL_STALE_TIME,
})

// Reject anything that could be an open-redirect vector.
function safeNext(): string {
  const n = route.query.next
  if (typeof n !== 'string') return '/overview'
  if (!n.startsWith('/') || n.startsWith('//')) return '/overview'
  if (n === '/login' || n.startsWith('/enroll')) return '/overview'
  return n
}

const signingIn = ref(false)

async function signIn() {
  errorMessage.value = null
  signingIn.value = true
  try {
    const session = await passkey.assert({
      begin: beginLogin,
      complete: completeLogin,
      title: '登录',
    })
    qc.setQueryData(queryKeys.session.current, session)
    const next = safeNext()
    // safeNext defaults to /overview which is admin-only; substitute the
    // role-aware fallback so non-admins land on a page they actually have
    // permission for.
    const target = next === '/overview' && session.role !== 'admin' ? fallbackFor(session) : next
    router.replace(target)
  } catch (err: unknown) {
    if (err instanceof passkey.PasskeyCancelled) return
    errorMessage.value = err instanceof ApiRequestError ? err.message : '登录失败'
  } finally {
    signingIn.value = false
  }
}
</script>

<template>
  <div class="bg-surface-0 border border-line rounded-lg shadow-sm p-8 w-full max-w-sm">
    <h1 class="text-xl font-semibold text-ink mb-6">PicoTera</h1>

    <div v-if="statusQuery.isPending.value" class="text-sm text-ink-faint">加载中…</div>

    <template v-else-if="statusQuery.data.value?.bootstrapped === false">
      <p class="text-sm text-ink-muted mb-4">尚未初始化。请在服务器上运行：</p>
      <pre
        class="bg-surface-100 text-ink text-sm font-mono px-3 py-2 rounded-md overflow-x-auto"
      ><code>picotera enroll-admin</code></pre>
      <p class="text-xs text-ink-faint mt-3">
        命令会输出一次性注册链接，粘贴到此设备的浏览器即可创建管理员用户。
      </p>
    </template>

    <template v-else>
      <p class="text-sm text-ink-muted mb-4">使用 Passkey 登录管理后台。</p>
      <Button :disabled="signingIn" @click="signIn">使用 Passkey 登录</Button>
      <p v-if="errorMessage" class="text-sm text-err mt-3">{{ errorMessage }}</p>
      <div class="mt-6 pt-4 border-t border-line">
        <RouterLink to="/login/recover" class="text-sm text-accent hover:underline">
          无 Passkey 登录
        </RouterLink>
      </div>
    </template>
  </div>
</template>
