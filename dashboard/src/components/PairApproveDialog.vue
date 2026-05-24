<script setup lang="ts">
import { computed, ref } from 'vue'
import { SidePanel, Button, Field, Icon } from '@/ui'
import PairingCode from '@/components/PairingCode.vue'
import PairingCodeInput from '@/components/PairingCodeInput.vue'
import {
  pairLookup,
  pairApprove,
  ApiRequestError,
  type PairLookupResponse,
} from '@/api/client'
import { runWithSudo, WebAuthnUserCancelled } from '@/composables/useSudo'

const emit = defineEmits<{ close: [] }>()

type Phase = 'enter' | 'confirm' | 'approving' | 'done'

const phase = ref<Phase>('enter')
// code is the already-normalized 8-char string (PairingCodeInput strips
// separators and uppercases as the user types).
const code = ref('')
const error = ref('')
const lookup = ref<PairLookupResponse | null>(null)

const ready = computed(() => code.value.length === 8)

async function onLookup() {
  if (!ready.value) {
    error.value = '配对码为 8 位字符'
    return
  }
  error.value = ''
  try {
    lookup.value = await pairLookup(code.value)
    phase.value = 'confirm'
  } catch (e: unknown) {
    error.value = e instanceof ApiRequestError ? e.message : '查找失败'
  }
}

async function onApprove() {
  if (!lookup.value) return
  phase.value = 'approving'
  error.value = ''
  try {
    // The server requires a fresh WebAuthn assertion before binding a
    // new credential. runWithSudo catches the sudo_required response,
    // runs the assertion ceremony, and retries the approve.
    await runWithSudo(() => pairApprove(code.value))
    phase.value = 'done'
  } catch (e: unknown) {
    if (e instanceof WebAuthnUserCancelled) {
      error.value = '已取消验证，配对未批准。'
    } else {
      error.value = e instanceof ApiRequestError ? e.message : '批准失败'
    }
    phase.value = 'confirm'
  }
}

function onBack() {
  phase.value = 'enter'
  lookup.value = null
}

function fmtTime(iso?: string | null): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('zh-CN')
}
</script>

<template>
  <SidePanel title="添加新设备" kicker="设备" @close="emit('close')">
    <template v-if="phase === 'enter'">
      <div class="flex flex-col gap-4">
        <p class="text-sm text-ink-muted">
          在新设备打开登录页，点击「无 Passkey 登录」 → 「有」，在下方输入显示的 8 位配对码。
        </p>
        <Field label="配对码">
          <PairingCodeInput v-model="code" @enter="onLookup" />
        </Field>
        <p v-if="error" class="text-sm text-err">{{ error }}</p>
      </div>
    </template>

    <template v-else-if="phase === 'confirm' && lookup">
      <div class="flex flex-col gap-4">
        <div class="rounded-md bg-err-faint text-err-ink text-xs px-3 py-2">
          批准后该设备将以您的账户登录，并可注册新的 Passkey。如非本人发起请取消。
        </div>
        <div class="flex flex-col items-center gap-1 my-1">
          <span class="text-xs text-ink-faint">请核对以下信息与要登录的新设备一致：</span>
          <PairingCode :code="lookup.displayCode" size="md" />
        </div>
        <div class="rounded-md border border-line bg-surface-100 px-4 py-3 flex flex-col gap-2">
          <div class="flex items-baseline gap-3">
            <span class="text-xs text-ink-faint w-16 shrink-0">设备</span>
            <code class="font-mono text-xs text-ink break-all">{{ lookup.initiatorUa || '—' }}</code>
          </div>
          <div class="flex items-baseline gap-3">
            <span class="text-xs text-ink-faint w-16 shrink-0">来源 IP</span>
            <code class="font-mono text-xs text-ink">{{ lookup.initiatorIp || '—' }}</code>
          </div>
          <div class="flex items-baseline gap-3">
            <span class="text-xs text-ink-faint w-16 shrink-0">发起时间</span>
            <span class="text-xs text-ink">{{ fmtTime(lookup.createdAt) }}</span>
          </div>
          <div class="flex items-baseline gap-3">
            <span class="text-xs text-ink-faint w-16 shrink-0">过期时间</span>
            <span class="text-xs text-ink">{{ fmtTime(lookup.expiresAt) }}</span>
          </div>
        </div>
        <p v-if="error" class="text-sm text-err">{{ error }}</p>
      </div>
    </template>

    <template v-else-if="phase === 'approving'">
      <div class="flex flex-col items-center gap-3 py-6">
        <div class="w-10 h-10 rounded-full border-2 border-line border-t-accent animate-spin"></div>
        <p class="text-sm text-ink-muted">批准中…</p>
      </div>
    </template>

    <template v-else-if="phase === 'done'">
      <div class="flex flex-col items-center gap-3 py-6">
        <Icon name="check" :size="32" class="text-accent" />
        <p class="text-sm text-ink-muted">已批准，新设备将自动登录。</p>
      </div>
    </template>

    <template #footer>
      <template v-if="phase === 'enter'">
        <Button variant="ghost" @click="emit('close')">取消</Button>
        <Button :disabled="!ready" @click="onLookup">下一步</Button>
      </template>
      <template v-else-if="phase === 'confirm'">
        <Button variant="ghost" @click="onBack">返回</Button>
        <Button @click="onApprove">批准</Button>
      </template>
      <template v-else-if="phase === 'done'">
        <Button @click="emit('close')">完成</Button>
      </template>
    </template>
  </SidePanel>
</template>
