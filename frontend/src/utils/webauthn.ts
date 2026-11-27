/**
 * WebAuthn 客户端助手：服务端 options JSON（[]byte 为 base64url 无填充）↔
 * navigator.credentials 所需的 ArrayBuffer，及凭据响应序列化回 base64url JSON。
 * 仅做编码转换，协议校验在服务端（go-webauthn）。
 */

function b64urlToBuf(s: string): ArrayBuffer {
  const pad = s.length % 4 === 0 ? '' : '='.repeat(4 - (s.length % 4))
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/') + pad
  const bin = atob(b64)
  const buf = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i)
  return buf.buffer
}

function bufToB64url(buf: ArrayBuffer | null): string {
  if (!buf) return ''
  const bytes = new Uint8Array(buf)
  let bin = ''
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

interface RawOptions {
  publicKey: Record<string, unknown>
}

/** register-begin options → navigator.credentials.create 入参。 */
export function prepareCreateOptions(options: unknown): PublicKeyCredentialCreationOptions {
  const pk = (options as RawOptions).publicKey
  const out = { ...pk } as Record<string, unknown>
  out.challenge = b64urlToBuf(pk.challenge as string)
  const user = pk.user as Record<string, unknown>
  out.user = { ...user, id: b64urlToBuf(user.id as string) }
  const exclude = (pk.excludeCredentials as Array<Record<string, unknown>> | undefined) ?? []
  out.excludeCredentials = exclude.map((c) => ({ ...c, id: b64urlToBuf(c.id as string) }))
  return out as unknown as PublicKeyCredentialCreationOptions
}

/** login-begin options → navigator.credentials.get 入参。 */
export function prepareGetOptions(options: unknown): PublicKeyCredentialRequestOptions {
  const pk = (options as RawOptions).publicKey
  const out = { ...pk } as Record<string, unknown>
  out.challenge = b64urlToBuf(pk.challenge as string)
  const allow = (pk.allowCredentials as Array<Record<string, unknown>> | undefined) ?? []
  out.allowCredentials = allow.map((c) => ({ ...c, id: b64urlToBuf(c.id as string) }))
  return out as unknown as PublicKeyCredentialRequestOptions
}

/** 注册凭据 → register-finish credential JSON。 */
export function serializeCreation(cred: PublicKeyCredential): unknown {
  const r = cred.response as AuthenticatorAttestationResponse
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      clientDataJSON: bufToB64url(r.clientDataJSON),
      attestationObject: bufToB64url(r.attestationObject),
      transports: typeof r.getTransports === 'function' ? r.getTransports() : []
    },
    clientExtensionResults: cred.getClientExtensionResults()
  }
}

/** 断言凭据 → login-finish / passkey-verify credential JSON。 */
export function serializeAssertion(cred: PublicKeyCredential): unknown {
  const r = cred.response as AuthenticatorAssertionResponse
  return {
    id: cred.id,
    rawId: bufToB64url(cred.rawId),
    type: cred.type,
    response: {
      clientDataJSON: bufToB64url(r.clientDataJSON),
      authenticatorData: bufToB64url(r.authenticatorData),
      signature: bufToB64url(r.signature),
      userHandle: bufToB64url(r.userHandle)
    },
    clientExtensionResults: cred.getClientExtensionResults()
  }
}

/** 浏览器是否支持 passkey。 */
export function passkeySupported(): boolean {
  return typeof window !== 'undefined' && typeof window.PublicKeyCredential !== 'undefined'
}
