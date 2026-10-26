import type { Extension } from '@codemirror/state'
import type { StreamParser } from '@codemirror/language'

async function legacy(load: () => Promise<StreamParser<unknown>>): Promise<Extension> {
  const { StreamLanguage } = await import('@codemirror/language')
  return StreamLanguage.define(await load())
}

const SHELL_FILES = new Set(['.bashrc', '.bash_profile', '.bash_aliases', '.profile', '.zshrc', '.zprofile', 'profile'])

/** Name of the language shown in the status bar, and its syntax highlighting (loaded on demand). */
export async function languageFor(path: string): Promise<{ label: string; ext: Extension | null }> {
  const name = (path.split(/[\\/]/).pop() ?? '').toLowerCase()
  const dot = name.lastIndexOf('.')
  const ext = dot > 0 ? name.slice(dot + 1) : ''

  if (name === 'dockerfile' || name.startsWith('dockerfile.') || ext === 'dockerfile' || name === 'containerfile')
    return { label: 'Dockerfile', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/dockerfile')).dockerFile) }
  if (/(^|[\\/])nginx[\\/]/.test(path) || name === 'nginx.conf' || (ext === 'conf' && /nginx|sites-(available|enabled)/.test(path)))
    return { label: 'Nginx', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/nginx')).nginx) }
  if (name === '.env' || name.startsWith('.env.') || ['env', 'ini', 'cfg', 'conf', 'properties', 'cnf'].includes(ext) || name === '.editorconfig')
    return { label: name.startsWith('.env') || ext === 'env' ? 'Env' : 'Config', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/properties')).properties) }
  if (SHELL_FILES.has(name) || ['sh', 'bash', 'zsh', 'ksh'].includes(ext))
    return { label: 'Shell', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/shell')).shell) }

  switch (ext) {
    case 'js':
    case 'mjs':
    case 'cjs':
    case 'jsx':
      return { label: 'JavaScript', ext: (await import('@codemirror/lang-javascript')).javascript({ jsx: true }) }
    case 'ts':
    case 'mts':
    case 'cts':
      return { label: 'TypeScript', ext: (await import('@codemirror/lang-javascript')).javascript({ typescript: true }) }
    case 'tsx':
      return { label: 'TypeScript', ext: (await import('@codemirror/lang-javascript')).javascript({ typescript: true, jsx: true }) }
    case 'json':
    case 'jsonc':
    case 'lock':
    case 'webmanifest':
      return { label: 'JSON', ext: (await import('@codemirror/lang-json')).json() }
    case 'html':
    case 'htm':
    case 'vue':
    case 'svelte':
      return { label: 'HTML', ext: (await import('@codemirror/lang-html')).html() }
    case 'css':
    case 'scss':
    case 'less':
      return { label: 'CSS', ext: (await import('@codemirror/lang-css')).css() }
    case 'md':
    case 'markdown':
      return { label: 'Markdown', ext: (await import('@codemirror/lang-markdown')).markdown() }
    case 'py':
      return { label: 'Python', ext: (await import('@codemirror/lang-python')).python() }
    case 'php':
    case 'phtml':
      return { label: 'PHP', ext: (await import('@codemirror/lang-php')).php() }
    case 'yml':
    case 'yaml':
      return { label: 'YAML', ext: (await import('@codemirror/lang-yaml')).yaml() }
    case 'xml':
    case 'svg':
    case 'xsd':
    case 'xsl':
    case 'plist':
    case 'csproj':
    case 'dist':
      return { label: 'XML', ext: (await import('@codemirror/lang-xml')).xml() }
    case 'sql':
      return { label: 'SQL', ext: (await import('@codemirror/lang-sql')).sql() }
    case 'go':
      return { label: 'Go', ext: (await import('@codemirror/lang-go')).go() }
    case 'rs':
      return { label: 'Rust', ext: (await import('@codemirror/lang-rust')).rust() }
    case 'java':
      return { label: 'Java', ext: (await import('@codemirror/lang-java')).java() }
    case 'c':
    case 'h':
    case 'cc':
    case 'cpp':
    case 'cxx':
    case 'hpp':
      return { label: 'C/C++', ext: (await import('@codemirror/lang-cpp')).cpp() }
    case 'toml':
      return { label: 'TOML', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/toml')).toml) }
    case 'rb':
      return { label: 'Ruby', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/ruby')).ruby) }
    case 'lua':
      return { label: 'Lua', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/lua')).lua) }
    case 'ps1':
    case 'psm1':
      return { label: 'PowerShell', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/powershell')).powerShell) }
    case 'diff':
    case 'patch':
      return { label: 'Diff', ext: await legacy(async () => (await import('@codemirror/legacy-modes/mode/diff')).diff) }
  }
  return { label: 'Plain text', ext: null }
}
