// Password generation with the platform CSPRNG and unbiased sampling.

const LOWER = 'abcdefghijkmnopqrstuvwxyz' // no l
const UPPER = 'ABCDEFGHJKLMNPQRSTUVWXYZ' // no I, O
const DIGITS = '23456789' // no 0, 1
const SYMBOLS = '!@#$%^&*-_=+?'

export interface GenOptions {
  length: number
  digits: boolean
  symbols: boolean
}

export const defaultGen: GenOptions = { length: 20, digits: true, symbols: true }

/** Uniform random integer in [0, n). */
function randomInt(n: number): number {
  const limit = Math.floor(0x1_0000_0000 / n) * n
  const buf = new Uint32Array(1)
  for (;;) {
    crypto.getRandomValues(buf)
    if (buf[0] < limit) return buf[0] % n
  }
}

export function generatePassword(o: GenOptions): string {
  const sets = [LOWER, UPPER]
  if (o.digits) sets.push(DIGITS)
  if (o.symbols) sets.push(SYMBOLS)
  const all = sets.join('')
  const length = Math.max(8, Math.min(128, o.length))
  for (;;) {
    let out = ''
    for (let i = 0; i < length; i++) out += all[randomInt(all.length)]
    // Every chosen character set appears at least once.
    if (sets.every((s) => [...out].some((c) => s.includes(c)))) return out
  }
}

/** Rough strength from length and the character classes used. */
export function passwordStrength(pw: string): { bits: number; label: string; level: 0 | 1 | 2 | 3 } {
  if (!pw) return { bits: 0, label: '', level: 0 }
  let pool = 0
  if (/[a-z]/.test(pw)) pool += 26
  if (/[A-Z]/.test(pw)) pool += 26
  if (/[0-9]/.test(pw)) pool += 10
  if (/[^a-zA-Z0-9]/.test(pw)) pool += 33
  const bits = Math.round(pw.length * Math.log2(Math.max(pool, 2)))
  if (bits < 45) return { bits, label: 'Weak', level: 0 }
  if (bits < 70) return { bits, label: 'Fair', level: 1 }
  if (bits < 100) return { bits, label: 'Strong', level: 2 }
  return { bits, label: 'Very strong', level: 3 }
}
