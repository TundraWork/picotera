<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { Button, Icon } from '@/ui'
import {
  pairBegin,
  pairStatus,
  pairComplete,
  ApiRequestError,
  type PairBeginResponse,
} from '@/api/client'
import { queryKeys } from '@/api/queryKeys'
import { webauthnCreate, WebAuthnUserCancelled } from '@/api/webauthn'
import { fallbackFor } from '@/router/fallback'

const router = useRouter()
const qc = useQueryClient()

type Phase = 'starting' | 'waiting' | 'approved' | 'registering' | 'done' | 'expired' | 'error'

const phase = ref<Phase>('starting')
const beginData = ref<PairBeginResponse | null>(null)
const error = ref('')
const copied = ref(false)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let copyTimer: ReturnType<typeof setTimeout> | null = null

async function start() {
  phase.value = 'starting'
  error.value = ''
  try {
    const data = await pairBegin()
    beginData.value = data
    phase.value = 'waiting'
    schedulePoll()
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : '获取配对码失败'
    phase.value = 'error'
  }
}

function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(poll, 2500)
}

async function poll() {
  if (!beginData.value) return
  if (phase.value !== 'waiting') return
  try {
    const s = await pairStatus(beginData.value.pairingId)
    if (s.status === 'approved') {
      phase.value = 'approved'
      await runRegistration()
      return
    }
    if (s.status === 'expired' || s.status === 'consumed') {
      phase.value = 'expired'
      return
    }
    schedulePoll()
  } catch (e: unknown) {
    // Transient network error — keep polling. Surface only if it persists
    // beyond the TTL window (caller will hit 'expired' eventually).
    if (e instanceof ApiRequestError) {
      error.value = e.message
    }
    schedulePoll()
  }
}

async function runRegistration() {
  if (!beginData.value) return
  phase.value = 'registering'
  try {
    const attestation = await webauthnCreate(
      beginData.value.publicKey as Parameters<typeof webauthnCreate>[0],
    )
    const result = await pairComplete(beginData.value.pairingId, attestation)
    qc.setQueryData(queryKeys.session.current, result.session)
    phase.value = 'done'
    router.replace(fallbackFor(result.session))
  } catch (e: unknown) {
    if (e instanceof WebAuthnUserCancelled) {
      error.value = '已取消注册。请重新尝试。'
    } else if (e instanceof ApiRequestError) {
      error.value = e.message
    } else {
      error.value = '注册失败'
    }
    phase.value = 'error'
  }
}

async function copyCode() {
  if (!beginData.value) return
  try {
    await navigator.clipboard.writeText(beginData.value.displayCode)
    copied.value = true
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copied.value = false
    }, 1500)
  } catch {
    // clipboard unavailable — ignore
  }
}

onMounted(start)

onBeforeUnmount(() => {
  if (pollTimer) clearTimeout(pollTimer)
  if (copyTimer) clearTimeout(copyTimer)
})
</script>

<template>
  <div class="bg-surface-0 border border-line rounded-lg shadow-sm p-8 w-full max-w-sm flex flex-col gap-4">
    <h1 class="text-xl font-semibold text-ink">在新设备上登录</h1>

    <template v-if="phase === 'starting'">
      <p class="text-sm text-ink-faint">正在生成配对码…</p>
    </template>

    <template v-else-if="phase === 'waiting' && beginData">
      <p class="text-sm text-ink-muted">
        在另一台已登录 PicoTera 的设备上打开「个人设置」，点击「添加新设备」并输入下方配对码：
      </p>
      <div class="flex flex-col items-center gap-3 my-2">
        <div class="font-mono text-3xl tracking-widest text-ink tabular-nums select-all">
          {{ beginData.displayCode }}
        </div>
        <Button variant="ghost" size="sm" @click="copyCode">
          <Icon :name="copied ? 'check' : 'copy'" :size="13" />
          <span>{{ copied ? '已复制' : '复制配对码' }}</span>
        </Button>
      </div>
      <p class="text-xs text-ink-faint text-center">
        等待对方批准…配对码 5 分钟内有效。
      </p>
    </template>

    <template v-else-if="phase === 'approved' || phase === 'registering'">
      <div class="flex flex-col items-center gap-3 py-4">
        <div class="w-10 h-10 rounded-full border-2 border-line border-t-accent animate-spin"></div>
        <p class="text-sm text-ink-muted text-center">
          {{ phase === 'approved' ? '配对已批准，准备注册 Passkey…' : '请在浏览器或密码管理器弹窗中操作' }}
        </p>
      </div>
    </template>

    <template v-else-if="phase === 'expired'">
      <p class="text-sm text-err">配对码已过期。</p>
      <Button @click="start">重新生成</Button>
    </template>

    <template v-else-if="phase === 'error'">
      <p class="text-sm text-err">{{ error || '出错了' }}</p>
      <Button @click="start">重试</Button>
    </template>

    <div class="pt-4 mt-2 border-t border-line">
      <RouterLink to="/login" class="text-sm text-accent hover:underline">返回登录</RouterLink>
    </div>
  </div>
</template>
