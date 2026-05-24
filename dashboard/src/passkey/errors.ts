// Translate raw error shapes (DOMException, ApiRequestError, generic Error)
// into user-facing Chinese messages. Centralized so every place that runs a
// WebAuthn ceremony surfaces the same wording.
//
// Anti-phishing note: the strings here intentionally don't leak whether
// the failure was server-side authentication vs. browser-side cancellation
// vs. authenticator policy — the user only needs to know "it didn't work,
// here's what you can do next".

import { ApiRequestError } from '@/api/client'
import { WebAuthnUserCancelled } from '@/api/webauthn'

function mapDOMException(err: DOMException, mode: 'enroll' | 'assert'): string {
  switch (err.name) {
    case 'NotAllowedError':
    case 'AbortError':
      return '操作已取消或超时，请重试。'
    case 'SecurityError':
      return '安全错误：请检查访问地址（必须为 https 或 localhost）。'
    case 'InvalidStateError':
      return mode === 'enroll'
        ? '此设备已为账户注册过 Passkey。'
        : '此设备没有可用的 Passkey。'
    case 'NotSupportedError':
      return '当前浏览器不支持所需的 Passkey 算法。'
    case 'ConstraintError':
      return '设备不满足 Passkey 条件（如未启用生物识别）。'
    default:
      return mode === 'enroll' ? '注册失败，请重试。' : '验证失败，请重试。'
  }
}

/**
 * Render a friendly Chinese message for any error thrown during a WebAuthn
 * ceremony. The mode controls verb choice ("注册" vs "验证") in the
 * generic-failure paths.
 */
export function messageForCeremonyError(err: unknown, mode: 'enroll' | 'assert'): string {
  if (err instanceof WebAuthnUserCancelled) {
    return '操作已取消或超时，请重试。'
  }
  if (err instanceof ApiRequestError) {
    return err.message
  }
  if (err instanceof DOMException) {
    return mapDOMException(err, mode)
  }
  if (err instanceof Error) {
    return mode === 'enroll' ? `注册失败：${err.message}` : `验证失败：${err.message}`
  }
  return mode === 'enroll' ? '注册失败，请重试。' : '验证失败，请重试。'
}
