// Typed access to the Go backend. Errors arrive as {code, message} objects
// (see ErrorFormatter in main.go).
import * as App from '../../wailsjs/go/main/App'
import { ClipboardGetText, ClipboardSetText, EventsOn } from '../../wailsjs/runtime/runtime'
import type {
  AppInfo,
  ConnectOptions,
  DirListing,
  ForwardRule,
  ForwardStatus,
  Group,
  Host,
  HostInput,
  Key,
  KeyGenerateInput,
  KeyImportInput,
  KnownHost,
  SSHConfigImport,
  SessionInfo,
  Settings,
  Snippet,
  Transfer,
  AlertEvent,
  AlertRule,
  MetricPoint,
  Metrics,
  MonitorStatus,
  UptimeCheck,
  UptimeResult,
  UptimeSummary,
  DeviceCode,
  LockStatus,
  RepoInfo,
  SyncRepos,
  SyncStatus,
  AgentApproval,
  AgentDecision,
  AgentLogEntry,
  AgentMode,
  AgentStatus,
  TOTPCode,
  VaultItem,
  VaultItemInput,
  VaultSecret,
} from './types'

/* eslint-disable @typescript-eslint/no-explicit-any */
const call = App as unknown as Record<string, (...args: any[]) => Promise<any>>

export const api = {
  appInfo: (): Promise<AppInfo> => call.GetAppInfo(),
  settings: (): Promise<Settings> => call.GetSettings(),
  updateSettings: (s: Settings): Promise<Settings> => call.UpdateSettings(s),
  clearAppData: (): Promise<Settings> => call.ClearAppData(),
  revealDataDir: (): Promise<void> => call.RevealDataDir(),

  hosts: (): Promise<Host[]> => call.ListHosts(),
  saveHost: (h: HostInput): Promise<Host> => call.SaveHost(h),
  revealHostPassword: (id: string): Promise<string> => call.RevealHostPassword(id),
  deleteHost: (id: string): Promise<void> => call.DeleteHost(id),
  groups: (): Promise<Group[]> => call.ListGroups(),
  saveGroup: (g: Group): Promise<Group> => call.SaveGroup(g),
  deleteGroup: (id: string): Promise<void> => call.DeleteGroup(id),
  importSSHConfig: (): Promise<SSHConfigImport> => call.ImportSSHConfig(),
  importSSHConfigFile: (): Promise<SSHConfigImport> => call.ImportSSHConfigFile(),

  openTerminal: (id: string, hostId: string, opts: ConnectOptions): Promise<SessionInfo> => call.OpenTerminal(id, hostId, opts),
  writeTerminal: (id: string, data: string): Promise<void> => call.WriteTerminal(id, data),
  resizeTerminal: (id: string, cols: number, rows: number): Promise<void> => call.ResizeTerminal(id, cols, rows),
  closeTerminal: (id: string): Promise<void> => call.CloseTerminal(id),
  cancelConnect: (id: string): Promise<void> => call.CancelConnect(id),
  resolveHostKey: (promptId: string, accept: boolean): Promise<boolean> => call.ResolveHostKey(promptId, accept),

  openSFTP: (id: string, hostId: string, opts: ConnectOptions): Promise<SessionInfo> => call.OpenSFTP(id, hostId, opts),
  closeSFTP: (id: string): Promise<void> => call.CloseSFTP(id),
  remoteHome: (id: string): Promise<string> => call.RemoteHome(id),
  listRemote: (id: string, dir: string): Promise<DirListing> => call.ListRemote(id, dir),
  mkdirRemote: (id: string, dir: string, name: string): Promise<void> => call.MkdirRemote(id, dir, name),
  renameRemote: (id: string, from: string, name: string): Promise<void> => call.RenameRemote(id, from, name),
  removeRemote: (id: string, paths: string[]): Promise<void> => call.RemoveRemote(id, paths),
  listLocal: (dir: string): Promise<DirListing> => call.ListLocal(dir),
  mkdirLocal: (dir: string, name: string): Promise<void> => call.MkdirLocal(dir, name),
  renameLocal: (from: string, name: string): Promise<void> => call.RenameLocal(from, name),
  removeLocal: (paths: string[]): Promise<void> => call.RemoveLocal(paths),
  upload: (id: string, localPaths: string[], remoteDir: string): Promise<string> => call.Upload(id, localPaths, remoteDir),
  download: (id: string, remotePaths: string[], localDir: string): Promise<string> => call.Download(id, remotePaths, localDir),
  transfers: (): Promise<Transfer[]> => call.ListTransfers(),
  cancelTransfer: (id: string): Promise<boolean> => call.CancelTransfer(id),
  clearTransfers: (): Promise<void> => call.ClearTransfers(),

  keys: (): Promise<Key[]> => call.ListKeys(),
  generateKey: (k: KeyGenerateInput): Promise<Key> => call.GenerateKey(k),
  importKey: (k: KeyImportInput): Promise<Key> => call.ImportKey(k),
  readKeyFile: (): Promise<string> => call.ReadKeyFile(),
  renameKey: (id: string, name: string): Promise<void> => call.RenameKey(id, name),
  deleteKey: (id: string): Promise<void> => call.DeleteKey(id),
  exportPrivateKey: (id: string): Promise<string> => call.ExportPrivateKey(id),
  installPublicKey: (hostId: string, keyId: string, opts: ConnectOptions): Promise<void> =>
    call.InstallPublicKey(hostId, keyId, opts),

  vaultItems: (): Promise<VaultItem[]> => call.ListVaultItems(),
  saveVaultItem: (input: VaultItemInput): Promise<VaultItem> => call.SaveVaultItem(input),
  setVaultItemFavorite: (id: string, favorite: boolean): Promise<void> => call.SetVaultItemFavorite(id, favorite),
  deleteVaultItem: (id: string): Promise<void> => call.DeleteVaultItem(id),
  revealVaultItem: (id: string): Promise<VaultSecret> => call.RevealVaultItem(id),
  vaultItemTotp: (id: string): Promise<TOTPCode> => call.VaultItemTOTP(id),
  /** Copies a value in the backend; secrets are cleared from the clipboard after 30 s. */
  copyVaultItemValue: (id: string, field: string): Promise<void> => call.CopyVaultItemValue(id, field),

  snippets: (): Promise<Snippet[]> => call.ListSnippets(),
  saveSnippet: (s: Snippet): Promise<Snippet> => call.SaveSnippet(s),
  deleteSnippet: (id: string): Promise<void> => call.DeleteSnippet(id),

  forwards: (): Promise<ForwardRule[]> => call.ListForwards(),
  forwardStatuses: (): Promise<ForwardStatus[]> => call.ForwardStatuses(),
  saveForward: (f: ForwardRule): Promise<ForwardRule> => call.SaveForward(f),
  deleteForward: (id: string): Promise<void> => call.DeleteForward(id),
  startForward: (id: string): Promise<void> => call.StartForward(id),
  stopForward: (id: string): Promise<void> => call.StopForward(id),

  knownHosts: (): Promise<KnownHost[]> => call.ListKnownHosts(),
  forgetKnownHost: (host: string): Promise<void> => call.ForgetKnownHost(host),

  monitorWatch: (hostId: string): Promise<MonitorStatus> => call.MonitorWatch(hostId),
  monitorUnwatch: (hostId: string): Promise<void> => call.MonitorUnwatch(hostId),
  monitorStatuses: (): Promise<MonitorStatus[]> => call.MonitorStatuses(),
  monitorLatest: (hostId: string): Promise<Metrics | null> => call.MonitorLatest(hostId),
  containerAction: (hostId: string, container: string, action: 'start' | 'stop' | 'restart'): Promise<void> =>
    call.ContainerAction(hostId, container, action),
  metricsHistory: (hostId: string, rangeMinutes: number): Promise<MetricPoint[]> => call.MetricsHistory(hostId, rangeMinutes),
  alertRules: (): Promise<AlertRule[]> => call.ListAlertRules(),
  saveAlertRule: (r: AlertRule): Promise<AlertRule> => call.SaveAlertRule(r),
  deleteAlertRule: (id: string): Promise<void> => call.DeleteAlertRule(id),
  alertEvents: (): Promise<AlertEvent[]> => call.ListAlertEvents(),
  uptime: (): Promise<UptimeSummary[]> => call.ListUptime(),
  saveUptimeCheck: (c: UptimeCheck): Promise<UptimeCheck> => call.SaveUptimeCheck(c),
  deleteUptimeCheck: (id: string): Promise<void> => call.DeleteUptimeCheck(id),
  testUptimeCheck: (c: UptimeCheck): Promise<UptimeResult> => call.TestUptimeCheck(c),
  sendTestNotification: (): Promise<void> => call.SendTestNotification(),

  lockStatus: (): Promise<LockStatus> => call.GetLockStatus(),
  unlock: (password: string): Promise<void> => call.Unlock(password),
  lock: (): Promise<void> => call.Lock(),
  setMasterPassword: (current: string, next: string): Promise<LockStatus> => call.SetMasterPassword(current, next),
  removeMasterPassword: (current: string): Promise<LockStatus> => call.RemoveMasterPassword(current),
  resetVault: (): Promise<Settings> => call.ResetVault(),

  syncStatus: (): Promise<SyncStatus> => call.GetSyncStatus(),
  syncSetApp: (clientId: string, slug: string): Promise<SyncStatus> => call.SyncSetApp(clientId, slug),
  syncStartLogin: (): Promise<DeviceCode> => call.SyncStartLogin(),
  syncFinishLogin: (): Promise<string> => call.SyncFinishLogin(),
  syncCancelLogin: (): Promise<void> => call.SyncCancelLogin(),
  syncRepos: (): Promise<SyncRepos> => call.SyncRepos(),
  syncInspect: (repo: string): Promise<RepoInfo> => call.SyncInspect(repo),
  syncConnect: (repo: string, password: string): Promise<SyncStatus> => call.SyncConnect(repo, password),
  syncDisconnect: (): Promise<SyncStatus> => call.SyncDisconnect(),
  syncNow: (): Promise<SyncStatus> => call.SyncNow(),
  syncWake: (): Promise<void> => call.SyncWake(),

  agentStatus: (): Promise<AgentStatus> => call.GetAgentStatus(),
  setAgentEnabled: (on: boolean): Promise<Settings> => call.SetAgentEnabled(on),
  setAgentHostMode: (hostId: string, mode: AgentMode | ''): Promise<Settings> => call.SetAgentHostMode(hostId, mode),
  revokeAgentGrants: (): Promise<void> => call.RevokeAgentGrants(),
  pendingAgentApprovals: (): Promise<AgentApproval[]> => call.PendingAgentApprovals(),
  resolveAgentApproval: (id: string, decision: AgentDecision): Promise<boolean> => call.ResolveAgentApproval(id, decision),
  agentLog: (limit: number): Promise<AgentLogEntry[]> => call.ListAgentLog(limit),
  clearAgentLog: (): Promise<void> => call.ClearAgentLog(),

}

export const events = {
  termData: (id: string) => `term:data:${id}`,
  termExit: (id: string) => `term:exit:${id}`,
  HOSTKEY_PROMPT: 'hostkey:prompt',
  HOSTKEY_RESOLVED: 'hostkey:resolved',
  FORWARD_STATUS: 'forward:status',
  TRANSFER: 'transfer:update',
  SFTP_CLOSED: 'sftp:closed',
  MONITOR_SAMPLE: 'monitor:sample',
  MONITOR_STATUS: 'monitor:status',
  ALERT: 'alert:event',
  UPTIME_RESULT: 'uptime:result',
  LOCK: 'lock:status',
  SYNC_STATUS: 'sync:status',
  VAULT_CHANGED: 'vault:changed',
  AGENT_APPROVAL: 'agent:approval',
  AGENT_APPROVAL_DONE: 'agent:approval-done',
  AGENT_ACTIVITY: 'agent:activity',
  AGENT_STATUS: 'agent:status',
} as const

export function onEvent<T>(name: string, cb: (data: T) => void): () => void {
  return EventsOn(name, cb as (...data: unknown[]) => void)
}

export const emptyConnect = (): ConnectOptions => ({ password: '', savePassword: false, passphrase: '', cols: 0, rows: 0 })

export async function copyText(text: string): Promise<boolean> {
  try {
    return await ClipboardSetText(text)
  } catch {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      return false
    }
  }
}

export async function readClipboard(): Promise<string> {
  try {
    return await ClipboardGetText()
  } catch {
    try {
      return await navigator.clipboard.readText()
    } catch {
      return ''
    }
  }
}

interface BridgeError {
  code: string
  message: string
}

/** Parses the JSON {code, message} produced by formatError in app.go. */
function parseBridgeError(err: unknown): BridgeError | null {
  const raw = err instanceof Error ? err.message : typeof err === 'string' ? err : null
  if (!raw || !raw.startsWith('{')) return null
  try {
    const v = JSON.parse(raw) as Partial<BridgeError>
    if (typeof v.message === 'string') return { code: v.code ?? '', message: v.message }
  } catch {
    /* not structured */
  }
  return null
}

/** Machine-readable error code from the backend ('' when none). */
export function errorCode(err: unknown): string {
  return parseBridgeError(err)?.code ?? ''
}

export function errorMessage(err: unknown): string {
  const b = parseBridgeError(err)
  if (b) return b.message
  if (err instanceof Error) return err.message
  if (typeof err === 'string') return err
  return 'Unexpected error'
}
