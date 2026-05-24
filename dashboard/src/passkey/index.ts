// Passkey SDK: the only API the rest of the dashboard should use to run a
// WebAuthn ceremony. Two entry points:
//
//   passkey.enroll(opts) → Promise<TResult>
//   passkey.assert(opts) → Promise<TResult>
//
// Both:
//   • show the global PasskeyPopup (centered floating modal)
//   • run begin → navigator.credentials.create|get → complete
//   • surface friendly Chinese error messages with Retry/Cancel
//   • resolve the promise with whatever the caller's complete() returns
//   • reject with PasskeyCancelled when the user explicitly cancels
//
// The server is the source of truth — every attestation/assertion is
// shipped to the caller's complete() which posts it to the relying-party
// endpoint for cryptographic verification. The SDK never trusts a
// credential locally; it just shuttles bytes between the browser and the
// caller's API.
//
// Cancellation contract (per WebAuthn L3 + FIDO UX guidance):
//   • Each ceremony attempt gets a fresh AbortController. The signal is
//     passed to navigator.credentials.create|get so the pending OS
//     authenticator prompt is dismissed when we cancel.
//   • Only one ceremony can be active at a time. Calling enroll() or
//     assert() while another is in flight aborts the prior one and
//     rejects its promise with PasskeyCancelled — concurrent calls
//     cannot leak.
//   • signal.aborted is the SDK's authoritative "did we cancel this?"
//     bit. Late errors from a cancelled webauthn promise are suppressed
//     so the caller sees PasskeyCancelled, not a stale retry prompt.
//
// Anti-phishing (per WebAuthn L3 + FIDO Alliance UX guidance):
//   • The phishing-resistance guarantee comes from the browser's
//     origin-binding + the server's challenge/origin verification, which
//     this SDK does not weaken — the server verifies; we just call.
//   • Optional nickname collection runs AFTER the server has verified the
//     credential and returned its row id, so a nickname rename failure
//     can't undo a successful registration.

import { ApiRequestError } from '@/api/client'
import {
  webauthnCreate,
  webauthnGet,
} from '@/api/webauthn'
import { passkeyState, resetPasskeyState } from './state'
import { messageForCeremonyError } from './errors'

/** Thrown when the user explicitly cancels the popup or the ceremony. */
export class PasskeyCancelled extends Error {
  constructor() {
    super('passkey ceremony cancelled')
    this.name = 'PasskeyCancelled'
  }
}

interface CommonOpts {
  /** Header text shown on the popup. */
  title?: string
  /** Secondary line under the title (e.g., what this is for). */
  subtitle?: string
}

export interface EnrollOptions<TComplete> extends CommonOpts {
  /** Server "register/begin" — returns the publicKey options JSON. */
  begin: () => Promise<unknown>
  /** Server "register/complete" — receives the attestation, returns whatever the RP responds with. */
  complete: (attestation: unknown) => Promise<TComplete>
  /**
   * Optional post-success rename hook. When provided AND extractCredentialId
   * is set, the popup shows a "name this passkey" step before resolving.
   * Empty/whitespace nickname means "skip — keep no nickname".
   */
  rename?: (credentialId: number, nickname: string) => Promise<void>
  /** Extract the credential row id from the complete() result for rename(). */
  extractCredentialId?: (r: TComplete) => number
}

export interface AssertOptions<TComplete> extends CommonOpts {
  /** Server "login/begin" — returns the requestOptions JSON. */
  begin: () => Promise<unknown>
  /** Server "login/complete" — receives the assertion. */
  complete: (assertion: unknown) => Promise<TComplete>
}

// Module-level state for cancellation. Exactly one flow at a time.
let activeAbort: AbortController | null = null
let activeReject: ((err: Error) => void) | null = null

function preemptActiveFlow(): void {
  if (activeAbort && !activeAbort.signal.aborted) {
    activeAbort.abort('replaced')
  }
  if (activeReject) {
    activeReject(new PasskeyCancelled())
  }
  activeAbort = null
  activeReject = null
}

// ----- enroll -------------------------------------------------------------

export function enroll<TComplete>(opts: EnrollOptions<TComplete>): Promise<TComplete> {
  return runCeremony('enroll', opts, async (signal) => {
    const options = await opts.begin()
    const attestation = await webauthnCreate(options as Parameters<typeof webauthnCreate>[0], signal)
    return opts.complete(attestation)
  })
}

// ----- assert -------------------------------------------------------------

export function assert<TComplete>(opts: AssertOptions<TComplete>): Promise<TComplete> {
  return runCeremony('assert', opts, async (signal) => {
    const options = await opts.begin()
    const assertion = await webauthnGet(options as Parameters<typeof webauthnGet>[0], signal)
    return opts.complete(assertion)
  })
}

// ----- internal -----------------------------------------------------------

function runCeremony<TComplete>(
  mode: 'enroll' | 'assert',
  opts: CommonOpts & {
    rename?: (id: number, nickname: string) => Promise<void>
    extractCredentialId?: (r: TComplete) => number
  },
  attempt: (signal: AbortSignal) => Promise<TComplete>,
): Promise<TComplete> {
  // Replace any in-flight ceremony — only one OS authenticator prompt at
  // a time per the WebAuthn spec, so we proactively abort the previous one
  // before starting a new one rather than let them race.
  preemptActiveFlow()

  return new Promise<TComplete>((resolve, reject) => {
    let attemptResult: TComplete | null = null

    const settle = (result: TComplete) => {
      activeAbort = null
      activeReject = null
      resetPasskeyState()
      resolve(result)
    }
    const fail = (err: Error) => {
      // Abort any in-flight webauthn call so the OS authenticator UI
      // dismisses alongside our popup. Most authenticators honor the
      // abort signal; a few (e.g. some browser-extension implementations)
      // don't — in that case the user has to dismiss the OS prompt
      // themselves, but we've still rejected the promise correctly.
      if (activeAbort && !activeAbort.signal.aborted) {
        activeAbort.abort('cancelled')
      }
      activeAbort = null
      activeReject = null
      resetPasskeyState()
      reject(err)
    }
    activeReject = fail

    const run = async () => {
      // Fresh AbortController per attempt — a used one stays aborted
      // forever, which would short-circuit the next webauthn call.
      const controller = new AbortController()
      activeAbort = controller
      const signal = controller.signal
      passkeyState.phase = 'waiting'
      passkeyState.error = ''
      passkeyState.saving = false

      try {
        attemptResult = await attempt(signal)
        // Late completion after a Cancel — the SDK promise was already
        // rejected; discard this result rather than show stale UI.
        if (signal.aborted) return

        if (
          mode === 'enroll' &&
          opts.rename &&
          opts.extractCredentialId &&
          attemptResult !== null
        ) {
          passkeyState.phase = 'naming'
          passkeyState.nickname = ''
          return
        }
        if (attemptResult !== null) settle(attemptResult)
      } catch (err) {
        // Cancellation we initiated: SDK has already rejected; suppress
        // the stale error display.
        if (signal.aborted) return
        passkeyState.error = messageForCeremonyError(err, mode)
        passkeyState.phase = 'error'
      }
    }

    passkeyState.active = true
    passkeyState.mode = mode
    passkeyState.title = opts.title ?? (mode === 'enroll' ? '注册 Passkey' : '验证 Passkey')
    passkeyState.subtitle = opts.subtitle ?? ''
    passkeyState.error = ''
    passkeyState.nickname = ''
    passkeyState.saving = false

    passkeyState.retry = run
    passkeyState.cancel = () => {
      // Cancel from 'naming' phase means accept the credential as-is —
      // the server already minted it; rejecting the SDK promise would
      // throw away a successful registration.
      if (passkeyState.phase === 'naming' && attemptResult !== null) {
        const r = attemptResult
        settle(r)
        return
      }
      fail(new PasskeyCancelled())
    }
    passkeyState.skipNickname = async () => {
      if (attemptResult !== null) settle(attemptResult)
    }
    passkeyState.submitNickname = async () => {
      if (!opts.rename || !opts.extractCredentialId || attemptResult === null) return
      const trimmed = passkeyState.nickname.trim()
      if (!trimmed) {
        if (attemptResult !== null) settle(attemptResult)
        return
      }
      passkeyState.saving = true
      try {
        await opts.rename(opts.extractCredentialId(attemptResult), trimmed)
        if (attemptResult !== null) settle(attemptResult)
      } catch (err) {
        passkeyState.saving = false
        passkeyState.error =
          err instanceof ApiRequestError
            ? `昵称保存失败：${err.message}`
            : '昵称保存失败，可稍后在个人设置中修改。'
      }
    }

    void run()
  })
}

// Aggregate export so callers can write `import * as passkey from '@/passkey'`
// or pick `enroll`/`assert` directly.
export default { enroll, assert }
