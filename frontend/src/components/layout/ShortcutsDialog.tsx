import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Kbd } from '@/components/ui/kbd'
import { appKey, isMac } from '@/lib/utils'
import { useUI } from '@/stores/ui'

const k = (s: string) => `${appKey}${s}`

const GROUPS: { title: string; items: [string, string][] }[] = [
  {
    title: 'Tabs',
    items: [
      ['Quick connect / new tab', k('T')],
      ['Close tab', k('W')],
      ['Next / previous tab', `${k(']')}  ${k('[')}`],
      ['Go to Vault', k('0')],
      ['Go to tab 1–9', k('1–9')],
    ],
  },
  {
    title: 'Terminal',
    items: [
      ['Copy selection', isMac ? '⌘C' : 'Ctrl+Shift+C'],
      ['Paste', isMac ? '⌘V' : 'Ctrl+Shift+V'],
      ['Find', k('F')],
      ['Clear scrollback', k('K')],
      ['Show or hide server metrics', k('I')],
      ['Next / previous color scheme', `${k(isMac ? '⌥]' : 'Alt+]')}  ${k(isMac ? '⌥[' : 'Alt+[')}`],
      ['Larger / smaller text', `${k('=')}  ${k('-')}`],
    ],
  },
  {
    title: 'App',
    items: [
      ['Settings', k(',')],
      ['Lock Remvyn (with a master password)', k('L')],
      ['Show shortcuts', k('/')],
    ],
  },
  {
    title: 'SFTP',
    items: [
      ['Open folder / transfer file', 'Double-click'],
      ['Multi-select / range', isMac ? '⌘-click  ⇧-click' : 'Ctrl-click  Shift-click'],
      ['Transfer between panes', 'Drag and drop'],
      ['Delete selected', isMac ? '⌘⌫' : 'Delete'],
    ],
  },
]

export function ShortcutsDialog() {
  const { shortcutsOpen, setShortcutsOpen } = useUI()
  return (
    <Dialog open={shortcutsOpen} onOpenChange={setShortcutsOpen}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Keyboard shortcuts</DialogTitle>
        </DialogHeader>
        <div className="flex max-h-[60vh] flex-col gap-4 overflow-auto">
          {GROUPS.map((g) => (
            <div key={g.title}>
              <div className="mb-1 text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">{g.title}</div>
              {g.items.map(([label, keys]) => (
                <div key={label} className="flex items-center justify-between border-b border-border/50 py-1.5 text-[13px]">
                  <span>{label}</span>
                  <Kbd className="h-auto px-1.5 py-0.5">{keys}</Kbd>
                </div>
              ))}
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
