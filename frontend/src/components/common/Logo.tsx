import { cn } from '@/lib/utils'

/** The Remvyn "routed R" mark (branding/mark.svg), drawn in the current color. */
export function Logo({ className }: { className?: string }) {
  return (
    <svg viewBox="160 160 704 704" aria-hidden className={cn('size-4', className)}>
      <path
        fill="currentColor"
        fillRule="evenodd"
        d="M192 192H544C703.058 192 832 277.961 832 384C832 473.4 751 535.6 648 561L736 704H832V832H656L496 576H320V832H192ZM320 320V448H544C632.366 448 704 423.346 704 384C704 344.654 632.366 320 544 320Z"
      />
    </svg>
  )
}
