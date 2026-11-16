/**
 * terminal Base64 编解码（D-01：帧 data=Base64 承载 PTY 任意原始字节——控制字符/非 UTF-8
 * 序列，不破坏 JSON 结构）。分块展开避免超长帧触发参数栈溢出。
 */

/** Uint8Array → Base64（0x8000 分块，String.fromCharCode.apply 栈限） */
export function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  const CHUNK = 0x8000
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK))
  }
  return btoa(binary)
}

/** Base64 → Uint8Array（output 帧解码后直写 xterm，xterm 原生按 UTF-8 解码字节流） */
export function base64ToBytes(b64: string): Uint8Array {
  const binary = atob(b64)
  const out = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    out[i] = binary.charCodeAt(i)
  }
  return out
}
