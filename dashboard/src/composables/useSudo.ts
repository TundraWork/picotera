// Sudo: re-prove possession of the account's passkey to elevate the
// session for one sensitive action. Returns a runWithSudo() helper that
// wraps an arbitrary async call — if it throws sudo_required, run the
// WebAuthn assertion and retry the action once.

import { sudoBegin, sudoComplete, ApiRequestError } from '@/api/client'
import { webauthnGet, WebAuthnUserCancelled } from '@/api/webauthn'

/**
 * Run a fresh WebAuthn assertion to lift the sudo flag on the current
 * session. Re-throws WebAuthnUserCancelled on cancel; throws
 * ApiRequestError on server-side failures.
 */
async function elevateSudo(): Promise<void> {
  const options = await sudoBegin()
  const assertion = await webauthnGet(options as Parameters<typeof webauthnGet>[0])
  await sudoComplete(assertion)
}

/**
 * Run `action`. If it throws sudo_required, prompt the user with a
 * WebAuthn assertion and retry once. Any other error propagates.
 *
 * Usage:
 *   await runWithSudo(() => pairApprove(code))
 */
export async function runWithSudo<T>(action: () => Promise<T>): Promise<T> {
  try {
    return await action()
  } catch (e: unknown) {
    if (e instanceof ApiRequestError && e.code === 'sudo_required') {
      await elevateSudo()
      return action()
    }
    throw e
  }
}

export { WebAuthnUserCancelled }
