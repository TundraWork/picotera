<script setup lang="ts">
import { computed, ref } from 'vue'
import { SidePanel, Button, Input, Field, Icon } from '@/ui'
import {
  pairLookup,
  pairApprove,
  ApiRequestError,
  type PairLookupResponse,
} from '@/api/client'

const emit = defineEmits<{ close: [] }>()

type Phase = 'enter' | 'confirm' | 'approving' | 'done'

const phase = ref<Phase>('enter')
const code = ref('')
const error = ref('')
const lookup = ref<PairLookupResponse | null>(null)

const normalizedCode = computed(() => code.value.replace(/[\s-]/g, '').toUpperCase())
const ready = computed(() => normalizedCode.value.length === 8)

async function onLookup() {
  if (!ready.value) {
    error.value = '配对码为 8 位字符'
    return
  }
  error.value = ''
  try {
    lookup.value = await pairLookup(normalizedCode.value)
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
    await pairApprove(normalizedCode.value)
    phase.value = 'done'
  } catch (e: unknown) {
    error.value = e instanceof ApiRequestError ? e.message : '批准失败'
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
          在希望加入的新设备上，打开 PicoTera 登录页，点击「无 Passkey 登录」并选择「有」。
          页面会显示一个 8 位配对码，将它输入到下方。
        </p>
        <Field label="配对码">
          <Input
            v-model="code"
            mono
            autofocus
            placeholder="例如 ABCD-EFGH"
            maxlength="9"
            @keydown.enter="onLookup"
          />
        </Field>
        <p v-if="error" class="text-sm text-err">{{ error }}</p>
      </div>
    </template>

    <template v-else-if="phase === 'confirm' && lookup">
      <div class="flex flex-col gap-4">
        <p class="text-sm text-ink-muted">
          请确认下方信息确实是您正在使用的新设备，再点击「批准」。
        </p>
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
        <p class="text-xs text-ink-faint">
          如果以上设备不是您本人发起的，请点击「返回」并忽略此次配对。
        </p>
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
        <p class="text-sm text-ink-muted text-center">
          已批准。新设备将自动完成 Passkey 注册并登录。
        </p>
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
