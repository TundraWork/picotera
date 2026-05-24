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
// endpoint for verification. The SDK never trusts a credential locally;
// it just shuttles bytes between the browser and the caller's API.
//
// Anti-phishing notes (per WebAuthn L3 + FIDO Alliance UX guidance):
//   • The popup never accepts user input that could be replayed (no PIN
//     fields here; PIN entry is the authenticator's own UI).
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
  WebAuthnUserCancelled,
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

// ----- enroll -------------------------------------------------------------

export function enroll<TComplete>(opts: EnrollOptions<TComplete>): Promise<TComplete> {
  return runCeremony('enroll', opts, async () => {
    const options = await opts.begin()
    const attestation = await webauthnCreate(options as Parameters<typeof webauthnCreate>[0])
    return opts.complete(attestation)
  })
}

// ----- assert -------------------------------------------------------------

export function assert<TComplete>(opts: AssertOptions<TComplete>): Promise<TComplete> {
  return runCeremony('assert', opts, async () => {
    const options = await opts.begin()
    const assertion = await webauthnGet(options as Parameters<typeof webauthnGet>[0])
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
  attempt: () => Promise<TComplete>,
): Promise<TComplete> {
  return new Promise<TComplete>((resolve, reject) => {
    let attemptResult: TComplete | null = null

    const settle = (result: TComplete) => {
      resetPasskeyState()
      resolve(result)
    }
    const fail = (err: Error) => {
      resetPasskeyState()
      reject(err)
    }

    const run = async () => {
      passkeyState.phase = 'waiting'
      passkeyState.error = ''
      passkeyState.saving = false
      try {
        attemptResult = await attempt()
        // Decide between resolving immediately or branching to the
        // optional naming step.
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
        if (err instanceof WebAuthnUserCancelled) {
          // Browser-side cancel — keep the popup but show the friendly
          // message; the user can Retry. Choosing Cancel from there
          // rejects with PasskeyCancelled.
          passkeyState.error = messageForCeremonyError(err, mode)
        } else {
          passkeyState.error = messageForCeremonyError(err, mode)
        }
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
      // From error: reject with PasskeyCancelled
      // From naming: nickname-step cancel means accept the credential as-is
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
        // Treat empty as skip — same effect as the Skip button.
        if (attemptResult !== null) settle(attemptResult)
        return
      }
      passkeyState.saving = true
      try {
        await opts.rename(opts.extractCredentialId(attemptResult), trimmed)
        if (attemptResult !== null) settle(attemptResult)
      } catch (err) {
        // Credential was created; only the rename failed. Resolve with
        // the success result but flash the rename-specific error.
        passkeyState.saving = false
        passkeyState.error =
          err instanceof ApiRequestError
            ? `昵称保存失败：${err.message}`
            : '昵称保存失败，可稍后在个人设置中修改。'
        // Stay on the naming view so the user can either retry the
        // nickname or Cancel-to-accept.
      }
    }

    void run()
  })
}

// Aggregate export so callers can write `import * as passkey from '@/passkey'`
// or pick `enroll`/`assert` directly.
export default { enroll, assert }
