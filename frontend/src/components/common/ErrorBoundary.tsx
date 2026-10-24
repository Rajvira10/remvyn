import { AlertOctagonIcon } from 'lucide-react'
import { Component, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'

interface State {
  error: Error | null
}

/** Contains rendering failures to one area instead of blanking the window. */
export class ErrorBoundary extends Component<{ children: ReactNode; resetKey?: unknown }, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidUpdate(prev: { resetKey?: unknown }) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) this.setState({ error: null })
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
        <AlertOctagonIcon className="size-6 text-danger" />
        <div className="text-[14px] font-medium">Something went wrong in this view</div>
        <pre className="max-w-lg text-xs whitespace-pre-wrap text-muted-foreground" data-selectable>
          {this.state.error.message}
        </pre>
        <Button variant="outline" size="sm" onClick={() => this.setState({ error: null })}>
          Try again
        </Button>
      </div>
    )
  }
}
