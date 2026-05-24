// Singleton reactive state shared by the SDK functions (enroll, assert) and
// the global popup (PasskeyPopupHost). One ceremony at a time is enforced
// at the API surface — the popup is always tied to the current active flow.

import { reactive } from 'vue'

export type PasskeyMode = 'enroll' | 'assert'
export type PasskeyPhase = 'waiting' | 'naming' | 'error'

export interface PasskeyState {
  active: boolean
  mode: PasskeyMode
  phase: PasskeyPhase
  title: string
  subtitle: string
  /** Visible error message in 'error' phase. */
  error: string
  /** Two-way bound to the nickname input in 'naming' phase. */
  nickname: string
  /** Whether the popup is showing the 'submitting nickname' inline spinner. */
  saving: boolean
  /** Triggered by Retry in 'error' phase. */
  retry: () => Promise<void>
  /** Triggered by Cancel / Esc in 'error' or 'naming' phase. */
  cancel: () => void
  /** Triggered by Save in 'naming' phase. */
  submitNickname: () => Promise<void>
  /** Triggered by Skip in 'naming' phase. */
  skipNickname: () => Promise<void>
}

const noop = async () => {}
const noopSync = () => {}

export const passkeyState = reactive<PasskeyState>({
  active: false,
  mode: 'enroll',
  phase: 'waiting',
  title: '',
  subtitle: '',
  error: '',
  nickname: '',
  saving: false,
  retry: noop,
  cancel: noopSync,
  submitNickname: noop,
  skipNickname: noop,
})

/** Reset to a clean, hidden state. Called when a flow concludes. */
export function resetPasskeyState() {
  passkeyState.active = false
  passkeyState.error = ''
  passkeyState.nickname = ''
  passkeyState.saving = false
  passkeyState.retry = noop
  passkeyState.cancel = noopSync
  passkeyState.submitNickname = noop
  passkeyState.skipNickname = noop
}
