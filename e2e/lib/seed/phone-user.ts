// e2e/lib/seed/phone-user.ts — register a real AUTH_METHOD_PHONE user
// off-camera so a spec can exercise the "returning member" path (the phone
// already has an account). Mints a Firebase ID token straight off the Auth
// Emulator's REST API (the same emulator the harness boots), then calls
// PhoneRegister — the same RPC the in-app phone-first flow uses. (#2492)

import { LoginService } from '../../gen/ripls/api/login_service_pb.js';
import { createTestClient } from '../connect.js';
import { emulatorVerificationCode } from '../ui/phone-register.js';

// The Auth Emulator ignores the API key for demo projects; any value works.
const EMULATOR_API_KEY = 'fake-api-key';

function emulatorHost(): string {
  return process.env.FIREBASE_AUTH_EMULATOR_HOST ?? '127.0.0.1:9099';
}

/** Drive the emulator phone-auth REST flow to a verified Firebase ID token. */
async function firebasePhoneIdToken(phone: string): Promise<string> {
  const base = `http://${emulatorHost()}/identitytoolkit.googleapis.com/v1/accounts`;

  // 1. Request a verification code (the emulator skips real reCAPTCHA).
  const sendResp = await fetch(`${base}:sendVerificationCode?key=${EMULATOR_API_KEY}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ phoneNumber: phone, recaptchaToken: 'NO_RECAPTCHA' }),
  });
  if (!sendResp.ok) {
    throw new Error(`sendVerificationCode ${sendResp.status}: ${await sendResp.text()}`);
  }
  const { sessionInfo } = (await sendResp.json()) as { sessionInfo: string };

  // 2. Read the code the emulator generated, then exchange it for an ID token.
  const code = await emulatorVerificationCode(phone);
  const signResp = await fetch(`${base}:signInWithPhoneNumber?key=${EMULATOR_API_KEY}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionInfo, code }),
  });
  if (!signResp.ok) {
    throw new Error(`signInWithPhoneNumber ${signResp.status}: ${await signResp.text()}`);
  }
  const { idToken } = (await signResp.json()) as { idToken: string };
  if (!idToken) throw new Error('emulator returned no idToken for phone sign-in');
  return idToken;
}

export interface SeededPhoneUser {
  userId: string;
  accessToken: string;
  refreshToken: string;
  name: string;
  phoneNumber: string;
}

/**
 * Register a real phone user via PhoneRegister (no invite). The returned account
 * "already exists" for [phoneNumber], so a later in-app phone-first attempt with
 * the same number must log in rather than reject.
 */
export async function registerPhoneUser(opts: {
  baseUrl: string;
  specSlug: string;
  phoneNumber: string;
  name: string;
}): Promise<SeededPhoneUser> {
  const idToken = await firebasePhoneIdToken(opts.phoneNumber);
  const client = createTestClient(LoginService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
  });
  const resp = await client.phoneRegister({
    firebaseIdToken: idToken,
    name: opts.name,
    shortCode: '',
  });
  if (!resp.user || !resp.tokens) {
    throw new Error('PhoneRegister returned no user/tokens');
  }
  return {
    userId: resp.user.id,
    accessToken: resp.tokens.accessToken,
    refreshToken: resp.tokens.refreshToken,
    name: opts.name,
    phoneNumber: opts.phoneNumber,
  };
}

/**
 * Log in an EXISTING phone user — one who registered on camera through the
 * phone-first UI — and return their session. The walkthrough reels use this to
 * act as the guest off camera after their on-camera registration (avatar,
 * chat replies), since the UI flow never exposes the minted tokens (#2686).
 */
export async function loginPhoneUser(opts: {
  baseUrl: string;
  specSlug: string;
  phoneNumber: string;
}): Promise<SeededPhoneUser> {
  const idToken = await firebasePhoneIdToken(opts.phoneNumber);
  const client = createTestClient(LoginService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
  });
  const resp = await client.phoneLogin({ firebaseIdToken: idToken });
  if (!resp.user || !resp.tokens) {
    throw new Error('PhoneLogin returned no user/tokens');
  }
  return {
    userId: resp.user.id,
    accessToken: resp.tokens.accessToken,
    refreshToken: resp.tokens.refreshToken,
    name: resp.user.name,
    phoneNumber: opts.phoneNumber,
  };
}
