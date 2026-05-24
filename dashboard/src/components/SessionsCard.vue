<script setup lang="ts">
import { computed } from 'vue'
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { listMySessions, revokeMySession } from '@/api/client'
import { queryKeys } from '@/api/queryKeys'
import { useConfirm } from '@/composables/useConfirm'
import {
  Badge,
  Button,
  DataCard,
  DataTable,
  Th,
  Td,
  Tr,
  StateText,
  IconButton,
  Icon,
} from '@/ui'
import type { components } from '@/openapi-types'

type SessionItem = components['schemas']['SessionListItem']

const qc = useQueryClient()
const confirm = useConfirm()

const sessionsQuery = useQuery({
  queryKey: queryKeys.sessions.mine,
  queryFn: listMySessions,
})
const sessions = computed<SessionItem[]>(() => sessionsQuery.data.value ?? [])

const revokeMutation = useMutation({
  mutationFn: (id: string) => revokeMySession(id),
  onSuccess: () => {
    qc.invalidateQueries({ queryKey: queryKeys.sessions.mine })
  },
})

function fmtTime(iso?: string | null): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('zh-CN')
}

function onRevoke(s: SessionItem) {
  confirm.require({
    message: `撤销该会话后，该设备将立即登出。`,
    accept: async () => {
      await revokeMutation.mutateAsync(s.id)
    },
  })
}
</script>

<template>
  <DataCard>
    <div>
      <div class="px-6 pt-6 pb-4">
        <h2 class="text-sm font-semibold text-ink">登录会话</h2>
      </div>
      <StateText v-if="sessionsQuery.isPending.value" class="px-6 pb-6">加载中…</StateText>
      <DataTable v-else>
        <thead>
          <tr>
            <Th>设备</Th>
            <Th>来源 IP</Th>
            <Th>登录时间</Th>
            <Th>过期时间</Th>
            <Th actions />
          </tr>
        </thead>
        <tbody>
          <Tr v-for="s in sessions" :key="s.id">
            <Td>
              <div class="flex items-center gap-1.5">
                <code class="font-mono text-2xs text-ink-muted break-all">{{ s.userAgent || '—' }}</code>
                <Badge v-if="s.isCurrent" variant="accent">当前</Badge>
              </div>
            </Td>
            <Td>
              <code class="font-mono text-2xs text-ink-muted">{{ s.lastSeenIp || '—' }}</code>
            </Td>
            <Td>{{ fmtTime(s.issuedAt) }}</Td>
            <Td>{{ fmtTime(s.expiresAt) }}</Td>
            <Td actions>
              <div class="inline-flex gap-1 opacity-55 group-hover:opacity-100 transition-opacity">
                <IconButton
                  v-if="!s.isCurrent"
                  variant="danger"
                  title="撤销"
                  aria-label="撤销"
                  :disabled="revokeMutation.isPending.value"
                  @click="onRevoke(s)"
                >
                  <Icon name="trash" :size="13" />
                </IconButton>
              </div>
            </Td>
          </Tr>
        </tbody>
      </DataTable>
    </div>
  </DataCard>
</template>
