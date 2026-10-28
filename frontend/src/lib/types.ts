// Mirrors backend/models. Plain interfaces so literals can be passed to Go.

export type AuthMethod = 'password' | 'key' | 'agent'

export interface Host {
  id: string
  label: string
  groupId: string
  hostname: string
  port: number
  username: string
  authMethod: AuthMethod
  keyId: string
  hasPassword: boolean
  color: string
  startupCommand: string
  createdAt: number
  updatedAt: number
  lastConnectedAt: number
}

export interface HostInput {
  host: Host
  password: string
  setPassword: boolean
}

export interface Group {
  id: string
  name: string
  createdAt: number
}

export interface Key {
  id: string
  name: string
  type: string
  bits: number
  fingerprint: string
  publicKey: string
  hasPassphrase: boolean
  createdAt: number
}

export interface KeyGenerateInput {
  name: string
  type: 'ed25519' | 'rsa' | 'ecdsa'
  bits: number
  passphrase: string
}

export interface KeyImportInput {
  name: string
  privateKey: string
  passphrase: string
}

export interface Snippet {
  id: string
  name: string
  command: string
  description: string
  createdAt: number
  updatedAt: number
}

export type ForwardType = 'local' | 'remote' | 'dynamic'

export interface ForwardRule {
  id: string
  label: string
  hostId: string
  type: ForwardType
  bindHost: string
  bindPort: number
  destHost: string
  destPort: number
  autoStart: boolean
  createdAt: number
}

export type ForwardState = 'stopped' | 'starting' | 'running' | 'error'

export interface ForwardStatus {
  ruleId: string
  state: ForwardState
  error?: string
  activeConns: number
  totalConns: number
  bytesIn: number
  bytesOut: number
  startedAt: number
}

export interface KnownHost {
  host: string
  keyType: string
  fingerprint: string
  addedAt: number
}

export interface HostKeyPrompt {
  id: string
  host: string
  keyType: string
  fingerprint: string
  changed: boolean
  oldFingerprint?: string
}

export interface ConnectOptions {
  password: string
  savePassword: boolean
  passphrase: string
  cols: number
  rows: number
  /** Server-side session to create or resume (persistent sessions). */
  persistKey?: string
  /** Replay a resumed session's scrollback into an empty terminal. */
  restore?: boolean
}

export interface SessionInfo {
  id: string
  hostId: string
  kind: 'terminal' | 'local' | 'sftp'
  title: string
  remote: string
  startedAt: number
  /** The shell runs in tmux on the server and survives disconnects. */
  persistent: boolean
  /** That tmux session already existed. */
  resumed: boolean
  persistNote?: 'tmux_missing' | 'tmux_old'
  /** The server accepted forwarding of the keychain's keys. */
  agentForwarding: boolean
  /** Other Remvyn sessions on the server nobody is attached to. */
  detached: ServerSession[] | null
}

/** A server asking to sign with a keychain key over agent forwarding. */
export interface KeyUseRequest {
  id: string
  hostId: string
  hostLabel: string
  remote: string
  keyName: string
  fingerprint: string
  /** "terminal", or the AI agent using the connection. */
  source: string
  ai: boolean
  createdAt: number
  expiresAt: number
}

/** A persistent terminal session running on a server. */
export interface ServerSession {
  key: string
  attached: boolean
  lastActive: number
  history: number
  command: string
  path: string
}

export interface SessionExit {
  id: string
  exitCode: number
  error?: string
}

export interface FileEntry {
  name: string
  path: string
  size: number
  mode: string
  modTime: number
  isDir: boolean
  isLink: boolean
  isHidden: boolean
}

export interface DirListing {
  path: string
  parent: string
  entries: FileEntry[]
}

/** Where a terminal's shell is. exact is false when only the home folder is known. */
export interface ShellDir {
  path: string
  home: string
  exact: boolean
}

/** A text file opened in the built-in editor. modTime is the version that was read. */
export interface FileContent {
  path: string
  content: string
  size: number
  modTime: number
}

/** An editor installed on this computer. */
export interface Editor {
  id: string
  name: string
}

export type ArchiveFormat = 'zip' | 'tar.gz'

/** Result of uploading a file edited in an external editor. */
export interface EditSync {
  sessionId: string
  path: string
  name: string
  conflict: boolean
  error: string
}

export type TransferState = 'queued' | 'running' | 'done' | 'failed' | 'cancelled'

export interface Transfer {
  id: string
  sessionId: string
  direction: 'upload' | 'download'
  source: string
  dest: string
  name: string
  state: TransferState
  error?: string
  bytesTotal: number
  bytesDone: number
  filesTotal: number
  filesDone: number
  startedAt: number
  finishedAt: number
}

export type Theme = 'dark' | 'light' | 'system'

export interface Settings {
  theme: Theme
  terminalFontSize: number
  terminalFontFamily: string
  terminalTheme: string
  autoLockMinutes: number
  cursorStyle: 'block' | 'bar' | 'underline'
  cursorBlink: boolean
  scrollback: number
  copyOnSelect: boolean
  persistentSessions: boolean
  keepaliveSeconds: number
  connectTimeout: number
  showHiddenFiles: boolean
  confirmOnClose: boolean
  commandSuggestions: boolean
  monitorHostIds: string[]
  monitorIntervalSec: number
  metricsRetentionDays: number
  alertNotifications: boolean
  agentEnabled: boolean
  /** host ID → 'ask' | 'auto'; missing = no agent access */
  agentHosts: Record<string, AgentMode>
  /** Hosts whose sessions may use the keychain's keys (agent forwarding). */
  forwardKeyHosts: string[]
  /** Also forward keys on AI agents' connections to those hosts. */
  forwardKeysToAI: boolean
  /** Look for a new release at startup and every few hours. */
  autoUpdateCheck: boolean
}

export type UpdateState = 'idle' | 'checking' | 'uptodate' | 'available' | 'downloading' | 'installing' | 'restarting' | 'error'

export interface Release {
  version: string
  name: string
  notes: string
  url: string
  publishedAt: string
}

export interface UpdateStatus {
  current: string
  state: UpdateState
  latest?: Release
  /** latest is newer than current */
  available: boolean
  checkedAt: string
  downloaded: number
  total: number
  error?: string
  /** Why this copy can't update itself; the UI links to the release instead. */
  manual?: string
}

export type AgentMode = 'ask' | 'auto'
export type AgentDecision = 'deny' | 'once' | 'session'

export interface AgentApproval {
  id: string
  client: string
  hostId: string
  hostLabel: string
  remote: string
  tool: string
  title: string
  detail: string
  preview?: string
  createdAt: number
  expiresAt: number
}

export interface AgentLogEntry {
  id: string
  at: number
  client: string
  hostId: string
  hostLabel: string
  tool: string
  summary: string
  status: 'ok' | 'failed' | 'error' | 'denied'
  approval: '' | 'auto' | 'once' | 'session'
  exitCode?: number
  durationMs: number
  output: string
}

export interface AgentGrant {
  hostId: string
  until: number
}

export interface AgentStatus {
  enabled: boolean
  listening: boolean
  listenError?: string
  socketPath: string
  command: string[]
  running: number
  grants: AgentGrant[]
}

export interface SSHConfigImport {
  imported: number
  skipped: number
  keys: number
  warnings: string[]
  path: string
}

export interface AppInfo {
  version: string
  dataDir: string
  platform: string
  homeDir: string
  hasAgent: boolean
  initError?: string
}


// ---------------------------------------------------------------- monitoring (mirrors backend/models/monitor.go)

export interface DiskUsage {
  mount: string
  fs: string
  total: number
  used: number
  usedPct: number
}

export interface NetIface {
  name: string
  rxRate: number
  txRate: number
  rxTotal: number
  txTotal: number
}

export interface Process {
  pid: number
  user: string
  command: string
  cpuPct: number
  memPct: number
}

export interface Container {
  id: string
  name: string
  image: string
  state: string
  status: string
  ports: string
  cpuPct: number
  memPct: number
  memUsage: string
}

export interface DockerInfo {
  available: boolean
  error: string
  containers: Container[]
  updatedAt: number
}

export interface Service {
  name: string
  active: string
  sub: string
  description: string
}

export interface ServicesInfo {
  available: boolean
  error: string
  running: number
  failed: number
  total: number
  items: Service[]
  updatedAt: number
}

export interface Metrics {
  hostId: string
  at: number
  os: string
  kernel: string
  hostname: string
  uptimeSec: number
  cpuCount: number
  cpuPct: number
  load1: number
  load5: number
  load15: number
  memTotal: number
  memUsed: number
  memAvailable: number
  memPct: number
  swapTotal: number
  swapUsed: number
  swapPct: number
  disks: DiskUsage[]
  diskPct: number
  net: NetIface[]
  rxRate: number
  txRate: number
  processes: Process[]
  docker: DockerInfo
  services: ServicesInfo
}

export interface MonitorStatus {
  hostId: string
  state: 'connecting' | 'live' | 'error' | 'stopped'
  error: string
  source: 'session' | 'direct' | ''
  background: boolean
  intervalSec: number
  lastAt: number
}

export interface MetricPoint {
  at: number
  cpu: number
  cpuMax: number
  mem: number
  swap: number
  disk: number
  load1: number
  rx: number
  tx: number
}

export type AlertMetric =
  | 'cpu'
  | 'memory'
  | 'swap'
  | 'disk'
  | 'load'
  | 'services_failed'
  | 'containers_stopped'
  | 'host_unreachable'
  | 'uptime_down'

export interface AlertRule {
  id: string
  name: string
  metric: AlertMetric
  hostId: string
  checkId: string
  op: '>' | '<'
  threshold: number
  durationSec: number
  enabled: boolean
  createdAt: number
}

export interface AlertEvent {
  id: number
  ruleId: string
  ruleName: string
  metric: AlertMetric
  subject: string
  subjectId: string
  value: number
  threshold: number
  message: string
  startedAt: number
  resolvedAt: number
}

export interface UptimeCheck {
  id: string
  name: string
  kind: 'http' | 'tcp'
  target: string
  intervalSec: number
  timeoutSec: number
  enabled: boolean
  createdAt: number
}

export interface UptimeResult {
  checkId: string
  at: number
  ok: boolean
  latencyMs: number
  statusCode: number
  error: string
}

export interface UptimeSummary {
  check: UptimeCheck
  up: boolean | null
  lastAt: number
  latencyMs: number
  lastError: string
  uptime24h: number
  uptime7d: number
  uptime30d: number
  recent: UptimeResult[]
  downSince: number
}

export interface LockStatus {
  protected: boolean
  locked: boolean
  minLength: number
  syncConnected: boolean
}

export interface SyncStatus {
  appConfigured: boolean
  clientId: string
  appSlug: string
  builtInApp: boolean
  connected: boolean
  repo: string
  repoUrl: string
  login: string
  state: 'off' | 'idle' | 'syncing' | 'error' | 'locked'
  lastSyncAt: number
  error: string
  errorCode: string
  installUrl: string
  newRepoUrl: string
}

export interface DeviceCode {
  userCode: string
  verificationUri: string
  expiresIn: number
  interval: number
}

export interface SyncRepo {
  fullName: string
  private: boolean
  defaultBranch: string
  htmlUrl: string
}

export interface SyncRepos {
  repos: SyncRepo[]
  installed: boolean
}

export interface RepoInfo extends SyncRepo {
  hasVault: boolean
  empty: boolean
}

export type VaultItemKind = 'login' | 'token' | 'note'

/** A password vault item without its secrets. */
export interface VaultItem {
  id: string
  kind: VaultItemKind
  name: string
  username: string
  url: string
  favorite: boolean
  hasPassword: boolean
  hasTotp: boolean
  hasNotes: boolean
  fieldCount: number
  createdAt: number
  updatedAt: number
}

export interface VaultField {
  name: string
  value: string
  hidden: boolean
}

export interface VaultSecret {
  password: string
  totp: string
  notes: string
  fields: VaultField[]
}

export interface VaultItemInput {
  item: VaultItem
  /** null keeps the saved secret. */
  secret: VaultSecret | null
}

export interface TOTPCode {
  code: string
  expiresAt: number
  period: number
}
