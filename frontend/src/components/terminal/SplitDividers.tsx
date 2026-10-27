import { useRef, type PointerEvent as ReactPointerEvent } from 'react'
import { cn } from '@/lib/utils'
import { splitRects, useSessions, type Layout, type Rect, type SplitDir } from '@/stores/sessions'

interface Divider {
  path: string
  dir: SplitDir
  rect: Rect // the split's whole area
  ratio: number
}

/** Draggable dividers between the panes of a split tab, laid over the panes. */
export function SplitDividers({ rootKey, layout, visible }: { rootKey: string; layout: Layout; visible: boolean }) {
  const ref = useRef<HTMLDivElement>(null)
  const dividers: Divider[] = []
  splitRects(layout, { x: 0, y: 0, w: 1, h: 1 }, '', (n, rect, path) => {
    if (!('pane' in n)) dividers.push({ path, dir: n.dir, rect, ratio: n.ratio })
  })

  const drag = (d: Divider) => (e: ReactPointerEvent) => {
    e.preventDefault()
    const box = ref.current!.getBoundingClientRect()
    const onMove = (ev: PointerEvent) => {
      const ratio =
        d.dir === 'row'
          ? (ev.clientX - box.left - d.rect.x * box.width) / (d.rect.w * box.width)
          : (ev.clientY - box.top - d.rect.y * box.height) / (d.rect.h * box.height)
      useSessions.getState().setRatio(rootKey, d.path, ratio)
    }
    const onUp = () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      document.body.style.cursor = ''
    }
    document.body.style.cursor = d.dir === 'row' ? 'col-resize' : 'row-resize'
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  return (
    <div ref={ref} className={cn('pointer-events-none absolute inset-0 z-20', !visible && 'hidden')}>
      {dividers.map((d) => {
        const row = d.dir === 'row'
        const at = row ? d.rect.x + d.rect.w * d.ratio : d.rect.y + d.rect.h * d.ratio
        const style = row
          ? { left: `calc(${at * 100}% - 3px)`, top: `${d.rect.y * 100}%`, height: `${d.rect.h * 100}%`, width: 6 }
          : { top: `calc(${at * 100}% - 3px)`, left: `${d.rect.x * 100}%`, width: `${d.rect.w * 100}%`, height: 6 }
        return (
          <div
            key={d.path}
            role="separator"
            aria-orientation={row ? 'vertical' : 'horizontal'}
            onPointerDown={drag(d)}
            className={cn('group pointer-events-auto absolute flex justify-center', row ? 'cursor-col-resize' : 'cursor-row-resize flex-col')}
            style={style}
          >
            <div className={cn('bg-border transition-colors group-hover:bg-primary', row ? 'h-full w-px' : 'h-px w-full')} />
          </div>
        )
      })}
    </div>
  )
}
