import claudeCode from '@/assets/ai-tools/claude-code.svg?raw'
import claudeDesktop from '@/assets/ai-tools/claude-desktop.svg?raw'
import codex from '@/assets/ai-tools/codex.svg?raw'
import cursor from '@/assets/ai-tools/cursor.svg?raw'
import gemini from '@/assets/ai-tools/gemini.svg?raw'
import opencode from '@/assets/ai-tools/opencode.svg?raw'
import vscode from '@/assets/ai-tools/vscode.svg?raw'
import windsurf from '@/assets/ai-tools/windsurf.svg?raw'
import { cn } from '@/lib/utils'

const icons: Record<string, string> = {
  'claude-code': claudeCode,
  'claude-desktop': claudeDesktop,
  codex,
  cursor,
  gemini,
  opencode,
  vscode,
  windsurf,
}

/** An AI tool's logo, by MCP client id; falls back to its initial. */
export function AIToolIcon({ id, name, className }: { id: string; name: string; className?: string }) {
  const svg = icons[id]
  return (
    <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background', className)} title={name}>
      {svg ? (
        <span aria-hidden className="flex size-[18px] [&>svg]:size-full" dangerouslySetInnerHTML={{ __html: svg }} />
      ) : (
        <span className="text-[12px] font-semibold text-muted-foreground">{name[0]}</span>
      )}
    </div>
  )
}
