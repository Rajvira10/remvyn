// The parts of noVNC's RFB client Remvyn uses (the package ships no types).
declare module '@novnc/novnc' {
  export interface RFBCredentials {
    username?: string
    password?: string
    target?: string
  }

  export interface RFBOptions {
    shared?: boolean
    credentials?: RFBCredentials
    repeaterID?: string
    wsProtocols?: string[]
  }

  export default class RFB extends EventTarget {
    constructor(target: HTMLElement, urlOrChannel: string | WebSocket, options?: RFBOptions)
    viewOnly: boolean
    scaleViewport: boolean
    clipViewport: boolean
    resizeSession: boolean
    showDotCursor: boolean
    focusOnClick: boolean
    background: string
    qualityLevel: number
    compressionLevel: number
    readonly capabilities: { power: boolean }
    disconnect(): void
    sendCredentials(creds: RFBCredentials): void
    sendCtrlAltDel(): void
    sendKey(keysym: number, code: string | null, down?: boolean): void
    focus(options?: FocusOptions): void
    blur(): void
    clipboardPasteFrom(text: string): void
  }
}
