// Sudo: re-prove possession of the account's passkey to elevate the
// session for one sensitive action. Wraps an arbitrary async action — if
// the server demands sudo, runs the WebAuthn assertion ceremony through
// the passkey SDK (which shows the centered popup with retry/cancel) and
// retries the original action exactly once.
//
// The popup UI lives in the global PasskeyPopupHost; this composable
// only orchestrates the begin/complete pair for the sudo grant.

import { sudoBegin, sudoComplete, ApiRequestError } from '@/api/client'
import * as passkey from '@/passkey'

/**
 * Run an assertion ceremony to lift the sudo flag on the current session.
 * Re-throws passkey.PasskeyCancelled on cancel; throws ApiRequestError on
 * server-side failures.
 */
async function elevateSudo(): Promise<void> {
  await passkey.assert({
    begin: sudoBegin,
    complete: (assertion) => sudoComplete(assertion),
    title: '安全验证',
    subtitle: '此操作需要重新验证 Passkey',
  })
}

/**
 * Run `action`. If it throws sudo_required, prompt the user with a
 * WebAuthn assertion (popup) and retry once. Any other error propagates.
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

// Re-export from passkey so callers don't need a second import.
export const WebAuthnUserCancelled = passkey.PasskeyCancelled
