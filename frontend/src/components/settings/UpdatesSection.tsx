import { RefreshCwIcon } from 'lucide-react'
import { toast } from 'sonner'
import { Field, Section } from '@/components/settings/Section'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import type { Settings } from '@/lib/types'
import { relativeTime } from '@/lib/utils'
import { useSettings } from '@/stores/settings'
import { useUpdate } from '@/stores/update'

export function UpdatesSection({ form, apply }: { form: Settings; apply: (patch: Partial<Settings>) => Promise<void> }) {
  const info = useSettings((s) => s.info)
  const { status, check, setDialogOpen } = useUpdate()
  const checking = status?.state === 'checking'
  const checkedAt = status?.checkedAt && !status.checkedAt.startsWith('0001') ? Date.parse(status.checkedAt) : 0

  const checkNow = async () => {
    const s = await check()
    if (!s) return
    if (s.available) setDialogOpen(true)
    else if (s.state === 'uptodate') toast.success(`Remvyn ${s.current} is the latest version`)
  }

  return (
    <Section title="Updates" description="New versions come from the Remvyn releases on GitHub.">
      <Field
        label={`Remvyn ${info?.version ?? ''}`}
        hint={
          <>
            {info?.platform}
            {checkedAt > 0 && <> · checked {relativeTime(checkedAt)}</>}
          </>
        }
      >
        <div className="flex items-center gap-2">
          {status?.available && (
            <Button size="sm" onClick={() => setDialogOpen(true)}>
              Update to {status.latest?.version}
            </Button>
          )}
          <Button variant="outline" size="sm" disabled={checking} onClick={() => void checkNow()}>
            <RefreshCwIcon className={checking ? 'animate-spin' : undefined} /> {checking ? 'Checking…' : 'Check for updates'}
          </Button>
        </div>
      </Field>
      <Field label="Check for updates automatically" hint="At startup and every 6 hours. Nothing is installed until you choose to update.">
        <Switch checked={form.autoUpdateCheck} onCheckedChange={(autoUpdateCheck) => void apply({ autoUpdateCheck })} />
      </Field>
    </Section>
  )
}
