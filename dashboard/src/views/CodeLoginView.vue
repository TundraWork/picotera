<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQueryClient } from '@tanstack/vue-query'
import { Button, Icon } from '@/ui'
import PairingCode from '@/components/PairingCode.vue'
import {
  pairBegin,
  pairStatus,
  pairComplete,
  addCredentialBegin,
  addCredentialComplete,
  ApiRequestError,
  type PairBeginResponse,
} from '@/api/client'
import { queryKeys } from '@/api/queryKeys'
import { webauthnCreate, WebAuthnUserCancelled } from '@/api/webauthn'
import { fallbackFor } from '@/router/fallback'

const router = useRouter()
const qc = useQueryClient()

// Pairing flow phases — entire lifecycle in one view.
type Phase = 'starting' | 'waiting' | 'completing' | 'registering' | 'error' | 'expired'

const phase = ref<Phase>('starting')
const beginData = ref<PairBeginResponse | null>(null)
const error = ref('')
const copied = ref(false)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let copyTimer: ReturnType<typeof setTimeout> | null = null
let signedInSession: Awaited<ReturnType<typeof pairComplete>>['session'] | null = null

async function start() {
  phase.value = 'starting'
  error.value = ''
  signedInSession = null
  try {
    beginData.value = await pairBegin()
    phase.value = 'waiting'
    schedulePoll()
  } catch (e: unknown) {
    error.value = e instanceof ApiRequestError ? e.message : '获取配对码失败'
    phase.value = 'error'
  }
}

function schedulePoll() {
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = setTimeout(poll, 2500)
}

async function poll() {
  if (!beginData.value || phase.value !== 'waiting') return
  try {
    const s = await pairStatus(beginData.value.pairingId)
    if (s.status === 'approved') {
      await completePairing()
      return
    }
    if (s.status === 'expired') {
      phase.value = 'expired'
      return
    }
    schedulePoll()
  } catch {
    // Transient network error — keep polling; TTL handles the abandoned case.
    schedulePoll()
  }
}

async function completePairing() {
  if (!beginData.value) return
  phase.value = 'completing'
  try {
    const result = await pairComplete(beginData.value.pairingId)
    signedInSession = result.session
    qc.setQueryData(queryKeys.session.current, result.session)
    // We are now signed in via the approver's authority. Immediately offer
    // to register a local passkey on this device so future logins skip the
    // pairing dance.
    await registerLocalPasskey()
  } catch (e: unknown) {
    error.value = e instanceof ApiRequestError ? e.message : '登录失败'
    phase.value = 'error'
  }
}

async function registerLocalPasskey() {
  phase.value = 'registering'
  try {
    const options = await addCredentialBegin()
    const attestation = await webauthnCreate(
      options as Parameters<typeof webauthnCreate>[0],
    )
    await addCredentialComplete(attestation)
    finishRedirect()
  } catch (e: unknown) {
    if (e instanceof WebAuthnUserCancelled) {
      // User declined to register a local passkey — they're still signed
      // in via the pair session; route them on anyway, they can register
      // later from /me.
      finishRedirect()
      return
    }
    error.value = e instanceof ApiRequestError ? e.message : '注册 Passkey 失败'
    phase.value = 'error'
  }
}

function finishRedirect() {
  if (!signedInSession) {
    router.replace('/login')
    return
  }
  router.replace(fallbackFor(signedInSession))
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
    // clipboard unavailable
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
      <div class="flex flex-col items-center gap-3 my-1">
        <PairingCode :code="beginData.displayCode" />
        <Button variant="ghost" size="sm" @click="copyCode">
          <Icon :name="copied ? 'check' : 'copy'" :size="13" />
          <span>{{ copied ? '已复制' : '复制' }}</span>
        </Button>
      </div>
      <p class="text-sm text-ink-muted">
        在已登录设备的「个人设置」点击「添加新设备」，将这串 8 位配对码输入并批准。
      </p>
      <p class="text-xs text-ink-faint">配对码 5 分钟内有效。</p>
    </template>

    <template v-else-if="phase === 'completing'">
      <div class="flex flex-col items-center gap-3 py-4">
        <div class="w-10 h-10 rounded-full border-2 border-line border-t-accent animate-spin"></div>
        <p class="text-sm text-ink-muted">正在登录…</p>
      </div>
    </template>

    <template v-else-if="phase === 'registering'">
      <div class="flex flex-col items-center gap-3 py-4">
        <div class="w-10 h-10 rounded-full border-2 border-line border-t-accent animate-spin"></div>
        <p class="text-sm text-ink-muted text-center">
          请在浏览器或密码管理器弹窗中为此设备注册 Passkey，用于下次登录。
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
