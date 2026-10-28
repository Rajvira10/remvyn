// Fuzzy matching and shell quoting for the terminal's path picker.

const BOUNDARY = '/_-. '

/**
 * Scores one query term against s (a relative path), or -1 when the letters
 * of term don't all appear in order. Matches in the file name, at the start
 * of words and in runs score higher.
 */
function termScore(term: string, s: string, lower: string): number {
  const slash = lower.lastIndexOf('/')
  const name = lower.slice(slash + 1)
  // Whole-name and in-name matches beat letters scattered over the path.
  if (name === term) return 10000
  let bonus = 0
  if (name.startsWith(term)) bonus = 3000
  else if (name.includes(term)) bonus = 2000
  else if (lower.includes(term)) bonus = 1000
  // Terms without a slash are tried against the name first.
  const from = !term.includes('/') && subsequenceAt(term, lower, slash + 1) ? slash + 1 : 0
  let at = from
  let prev = -2
  let score = from > 0 ? 500 : 0
  for (const ch of term) {
    const i = lower.indexOf(ch, at)
    if (i < 0) return -1
    if (i === prev + 1) score += 8
    if (i === 0 || BOUNDARY.includes(lower[i - 1]) || (s[i] !== lower[i] && s[i - 1] === lower[i - 1])) score += 12
    prev = i
    at = i + 1
  }
  return bonus + score
}

function subsequenceAt(term: string, s: string, from: number): boolean {
  let i = 0
  for (let j = from; j < s.length && i < term.length; j++) if (s[j] === term[i]) i++
  return i === term.length
}

/**
 * Scores a path against a query of space-separated terms, all of which must
 * match; -1 means no match. Shorter names and paths win ties.
 */
export function pathScore(query: string, path: string): number {
  const lower = path.toLowerCase()
  let total = 0
  for (const term of query.toLowerCase().split(/\s+/)) {
    if (!term) continue
    const s = termScore(term, path, lower)
    if (s < 0) return -1
    total += s
  }
  const name = path.length - path.lastIndexOf('/') - 1
  return total - path.length - 3 * name - 5 * (path.split('/').length - 1)
}

/** The positions in path to highlight for query (first occurrences, in order). */
export function matchPositions(query: string, path: string): Set<number> {
  const lower = path.toLowerCase()
  const out = new Set<number>()
  for (const term of query.toLowerCase().split(/\s+/)) {
    if (!term) continue
    const slash = lower.lastIndexOf('/')
    const sub = lower.indexOf(term, slash + 1)
    if (sub >= 0) {
      for (let i = 0; i < term.length; i++) out.add(sub + i)
      continue
    }
    let at = !term.includes('/') && subsequenceAt(term, lower, slash + 1) ? slash + 1 : 0
    for (const ch of term) {
      const i = lower.indexOf(ch, at)
      if (i < 0) break
      out.add(i)
      at = i + 1
    }
  }
  return out
}

// Characters no shell treats specially ("=" only after the start, for zsh).
const PLAIN = /^[\w@%+:,./-][\w@%+=:,./-]*$/
const PLAIN_PS = /^[\w.:\\/-]+$/

/** Quotes a path as one shell word, leaving plain paths and a leading ~/ alone. */
export function shellQuote(path: string, powershell = false): string {
  if (powershell) return PLAIN_PS.test(path) ? path : `'${path.replace(/'/g, "''")}'`
  if (path.startsWith('~/')) return path.length > 2 ? '~/' + shellQuote(path.slice(2)) : path
  return PLAIN.test(path) ? path : `'${path.replace(/'/g, `'\\''`)}'`
}
