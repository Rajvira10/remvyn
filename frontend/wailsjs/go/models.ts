export namespace core {
	
	export class SyncRepos {
	    repos: vaultsync.Repo[];
	    installed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SyncRepos(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repos = this.convertValues(source["repos"], vaultsync.Repo);
	        this.installed = source["installed"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {
	
	export class AppInfo {
	    version: string;
	    dataDir: string;
	    platform: string;
	    homeDir: string;
	    hasAgent: boolean;
	    initError?: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.dataDir = source["dataDir"];
	        this.platform = source["platform"];
	        this.homeDir = source["homeDir"];
	        this.hasAgent = source["hasAgent"];
	        this.initError = source["initError"];
	    }
	}

}

export namespace models {
	
	export class AlertEvent {
	    id: number;
	    ruleId: string;
	    ruleName: string;
	    metric: string;
	    subject: string;
	    subjectId: string;
	    value: number;
	    threshold: number;
	    message: string;
	    startedAt: number;
	    resolvedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new AlertEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.ruleId = source["ruleId"];
	        this.ruleName = source["ruleName"];
	        this.metric = source["metric"];
	        this.subject = source["subject"];
	        this.subjectId = source["subjectId"];
	        this.value = source["value"];
	        this.threshold = source["threshold"];
	        this.message = source["message"];
	        this.startedAt = source["startedAt"];
	        this.resolvedAt = source["resolvedAt"];
	    }
	}
	export class AlertRule {
	    id: string;
	    name: string;
	    metric: string;
	    hostId: string;
	    checkId: string;
	    op: string;
	    threshold: number;
	    durationSec: number;
	    enabled: boolean;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new AlertRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.metric = source["metric"];
	        this.hostId = source["hostId"];
	        this.checkId = source["checkId"];
	        this.op = source["op"];
	        this.threshold = source["threshold"];
	        this.durationSec = source["durationSec"];
	        this.enabled = source["enabled"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ConnectOptions {
	    password: string;
	    savePassword: boolean;
	    passphrase: string;
	    cols: number;
	    rows: number;
	
	    static createFrom(source: any = {}) {
	        return new ConnectOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.password = source["password"];
	        this.savePassword = source["savePassword"];
	        this.passphrase = source["passphrase"];
	        this.cols = source["cols"];
	        this.rows = source["rows"];
	    }
	}
	export class Container {
	    id: string;
	    name: string;
	    image: string;
	    state: string;
	    status: string;
	    ports: string;
	    cpuPct: number;
	    memPct: number;
	    memUsage: string;
	
	    static createFrom(source: any = {}) {
	        return new Container(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.image = source["image"];
	        this.state = source["state"];
	        this.status = source["status"];
	        this.ports = source["ports"];
	        this.cpuPct = source["cpuPct"];
	        this.memPct = source["memPct"];
	        this.memUsage = source["memUsage"];
	    }
	}
	export class FileEntry {
	    name: string;
	    path: string;
	    size: number;
	    mode: string;
	    modTime: number;
	    isDir: boolean;
	    isLink: boolean;
	    isHidden: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.mode = source["mode"];
	        this.modTime = source["modTime"];
	        this.isDir = source["isDir"];
	        this.isLink = source["isLink"];
	        this.isHidden = source["isHidden"];
	    }
	}
	export class DirListing {
	    path: string;
	    parent: string;
	    entries: FileEntry[];
	
	    static createFrom(source: any = {}) {
	        return new DirListing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.parent = source["parent"];
	        this.entries = this.convertValues(source["entries"], FileEntry);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DiskUsage {
	    mount: string;
	    fs: string;
	    total: number;
	    used: number;
	    usedPct: number;
	
	    static createFrom(source: any = {}) {
	        return new DiskUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mount = source["mount"];
	        this.fs = source["fs"];
	        this.total = source["total"];
	        this.used = source["used"];
	        this.usedPct = source["usedPct"];
	    }
	}
	export class DockerInfo {
	    available: boolean;
	    error: string;
	    containers: Container[];
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new DockerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.error = source["error"];
	        this.containers = this.convertValues(source["containers"], Container);
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ForwardRule {
	    id: string;
	    label: string;
	    hostId: string;
	    type: string;
	    bindHost: string;
	    bindPort: number;
	    destHost: string;
	    destPort: number;
	    autoStart: boolean;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ForwardRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.hostId = source["hostId"];
	        this.type = source["type"];
	        this.bindHost = source["bindHost"];
	        this.bindPort = source["bindPort"];
	        this.destHost = source["destHost"];
	        this.destPort = source["destPort"];
	        this.autoStart = source["autoStart"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class ForwardStatus {
	    ruleId: string;
	    state: string;
	    error?: string;
	    activeConns: number;
	    totalConns: number;
	    bytesIn: number;
	    bytesOut: number;
	    startedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ForwardStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ruleId = source["ruleId"];
	        this.state = source["state"];
	        this.error = source["error"];
	        this.activeConns = source["activeConns"];
	        this.totalConns = source["totalConns"];
	        this.bytesIn = source["bytesIn"];
	        this.bytesOut = source["bytesOut"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class Group {
	    id: string;
	    name: string;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class Host {
	    id: string;
	    label: string;
	    groupId: string;
	    hostname: string;
	    port: number;
	    username: string;
	    authMethod: string;
	    keyId: string;
	    hasPassword: boolean;
	    color: string;
	    startupCommand: string;
	    createdAt: number;
	    updatedAt: number;
	    lastConnectedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Host(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.groupId = source["groupId"];
	        this.hostname = source["hostname"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.authMethod = source["authMethod"];
	        this.keyId = source["keyId"];
	        this.hasPassword = source["hasPassword"];
	        this.color = source["color"];
	        this.startupCommand = source["startupCommand"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.lastConnectedAt = source["lastConnectedAt"];
	    }
	}
	export class HostInput {
	    host: Host;
	    password: string;
	    setPassword: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = this.convertValues(source["host"], Host);
	        this.password = source["password"];
	        this.setPassword = source["setPassword"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Key {
	    id: string;
	    name: string;
	    type: string;
	    bits: number;
	    fingerprint: string;
	    publicKey: string;
	    hasPassphrase: boolean;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Key(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.bits = source["bits"];
	        this.fingerprint = source["fingerprint"];
	        this.publicKey = source["publicKey"];
	        this.hasPassphrase = source["hasPassphrase"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class KeyGenerateInput {
	    name: string;
	    type: string;
	    bits: number;
	    passphrase: string;
	
	    static createFrom(source: any = {}) {
	        return new KeyGenerateInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.bits = source["bits"];
	        this.passphrase = source["passphrase"];
	    }
	}
	export class KeyImportInput {
	    name: string;
	    privateKey: string;
	    passphrase: string;
	
	    static createFrom(source: any = {}) {
	        return new KeyImportInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.privateKey = source["privateKey"];
	        this.passphrase = source["passphrase"];
	    }
	}
	export class KnownHost {
	    host: string;
	    keyType: string;
	    fingerprint: string;
	    addedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new KnownHost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.keyType = source["keyType"];
	        this.fingerprint = source["fingerprint"];
	        this.addedAt = source["addedAt"];
	    }
	}
	export class LockStatus {
	    protected: boolean;
	    locked: boolean;
	    minLength: number;
	    syncConnected: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LockStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protected = source["protected"];
	        this.locked = source["locked"];
	        this.minLength = source["minLength"];
	        this.syncConnected = source["syncConnected"];
	    }
	}
	export class MetricPoint {
	    at: number;
	    cpu: number;
	    cpuMax: number;
	    mem: number;
	    swap: number;
	    disk: number;
	    load1: number;
	    rx: number;
	    tx: number;
	
	    static createFrom(source: any = {}) {
	        return new MetricPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.at = source["at"];
	        this.cpu = source["cpu"];
	        this.cpuMax = source["cpuMax"];
	        this.mem = source["mem"];
	        this.swap = source["swap"];
	        this.disk = source["disk"];
	        this.load1 = source["load1"];
	        this.rx = source["rx"];
	        this.tx = source["tx"];
	    }
	}
	export class Service {
	    name: string;
	    active: string;
	    sub: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new Service(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.active = source["active"];
	        this.sub = source["sub"];
	        this.description = source["description"];
	    }
	}
	export class ServicesInfo {
	    available: boolean;
	    error: string;
	    running: number;
	    failed: number;
	    total: number;
	    items: Service[];
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new ServicesInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.error = source["error"];
	        this.running = source["running"];
	        this.failed = source["failed"];
	        this.total = source["total"];
	        this.items = this.convertValues(source["items"], Service);
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Process {
	    pid: number;
	    user: string;
	    command: string;
	    cpuPct: number;
	    memPct: number;
	
	    static createFrom(source: any = {}) {
	        return new Process(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.user = source["user"];
	        this.command = source["command"];
	        this.cpuPct = source["cpuPct"];
	        this.memPct = source["memPct"];
	    }
	}
	export class NetIface {
	    name: string;
	    rxRate: number;
	    txRate: number;
	    rxTotal: number;
	    txTotal: number;
	
	    static createFrom(source: any = {}) {
	        return new NetIface(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.rxRate = source["rxRate"];
	        this.txRate = source["txRate"];
	        this.rxTotal = source["rxTotal"];
	        this.txTotal = source["txTotal"];
	    }
	}
	export class Metrics {
	    hostId: string;
	    at: number;
	    os: string;
	    kernel: string;
	    hostname: string;
	    uptimeSec: number;
	    cpuCount: number;
	    cpuPct: number;
	    load1: number;
	    load5: number;
	    load15: number;
	    memTotal: number;
	    memUsed: number;
	    memAvailable: number;
	    memPct: number;
	    swapTotal: number;
	    swapUsed: number;
	    swapPct: number;
	    disks: DiskUsage[];
	    diskPct: number;
	    net: NetIface[];
	    rxRate: number;
	    txRate: number;
	    processes: Process[];
	    docker: DockerInfo;
	    services: ServicesInfo;
	
	    static createFrom(source: any = {}) {
	        return new Metrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostId = source["hostId"];
	        this.at = source["at"];
	        this.os = source["os"];
	        this.kernel = source["kernel"];
	        this.hostname = source["hostname"];
	        this.uptimeSec = source["uptimeSec"];
	        this.cpuCount = source["cpuCount"];
	        this.cpuPct = source["cpuPct"];
	        this.load1 = source["load1"];
	        this.load5 = source["load5"];
	        this.load15 = source["load15"];
	        this.memTotal = source["memTotal"];
	        this.memUsed = source["memUsed"];
	        this.memAvailable = source["memAvailable"];
	        this.memPct = source["memPct"];
	        this.swapTotal = source["swapTotal"];
	        this.swapUsed = source["swapUsed"];
	        this.swapPct = source["swapPct"];
	        this.disks = this.convertValues(source["disks"], DiskUsage);
	        this.diskPct = source["diskPct"];
	        this.net = this.convertValues(source["net"], NetIface);
	        this.rxRate = source["rxRate"];
	        this.txRate = source["txRate"];
	        this.processes = this.convertValues(source["processes"], Process);
	        this.docker = this.convertValues(source["docker"], DockerInfo);
	        this.services = this.convertValues(source["services"], ServicesInfo);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MonitorStatus {
	    hostId: string;
	    state: string;
	    error: string;
	    source: string;
	    background: boolean;
	    intervalSec: number;
	    lastAt: number;
	
	    static createFrom(source: any = {}) {
	        return new MonitorStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostId = source["hostId"];
	        this.state = source["state"];
	        this.error = source["error"];
	        this.source = source["source"];
	        this.background = source["background"];
	        this.intervalSec = source["intervalSec"];
	        this.lastAt = source["lastAt"];
	    }
	}
	
	
	export class SSHConfigImport {
	    imported: number;
	    skipped: number;
	    keys: number;
	    warnings: string[];
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new SSHConfigImport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.skipped = source["skipped"];
	        this.keys = source["keys"];
	        this.warnings = source["warnings"];
	        this.path = source["path"];
	    }
	}
	
	
	export class SessionInfo {
	    id: string;
	    hostId: string;
	    kind: string;
	    title: string;
	    remote: string;
	    startedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.hostId = source["hostId"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.remote = source["remote"];
	        this.startedAt = source["startedAt"];
	    }
	}
	export class Settings {
	    theme: string;
	    terminalFontSize: number;
	    terminalFontFamily: string;
	    terminalTheme: string;
	    autoLockMinutes: number;
	    cursorStyle: string;
	    cursorBlink: boolean;
	    scrollback: number;
	    copyOnSelect: boolean;
	    keepaliveSeconds: number;
	    connectTimeout: number;
	    showHiddenFiles: boolean;
	    confirmOnClose: boolean;
	    proEnabled: boolean;
	    monitorHostIds: string[];
	    monitorIntervalSec: number;
	    metricsRetentionDays: number;
	    alertNotifications: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.terminalFontSize = source["terminalFontSize"];
	        this.terminalFontFamily = source["terminalFontFamily"];
	        this.terminalTheme = source["terminalTheme"];
	        this.autoLockMinutes = source["autoLockMinutes"];
	        this.cursorStyle = source["cursorStyle"];
	        this.cursorBlink = source["cursorBlink"];
	        this.scrollback = source["scrollback"];
	        this.copyOnSelect = source["copyOnSelect"];
	        this.keepaliveSeconds = source["keepaliveSeconds"];
	        this.connectTimeout = source["connectTimeout"];
	        this.showHiddenFiles = source["showHiddenFiles"];
	        this.confirmOnClose = source["confirmOnClose"];
	        this.proEnabled = source["proEnabled"];
	        this.monitorHostIds = source["monitorHostIds"];
	        this.monitorIntervalSec = source["monitorIntervalSec"];
	        this.metricsRetentionDays = source["metricsRetentionDays"];
	        this.alertNotifications = source["alertNotifications"];
	    }
	}
	export class Snippet {
	    id: string;
	    name: string;
	    command: string;
	    description: string;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Snippet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.command = source["command"];
	        this.description = source["description"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class Transfer {
	    id: string;
	    sessionId: string;
	    direction: string;
	    source: string;
	    dest: string;
	    name: string;
	    state: string;
	    error?: string;
	    bytesTotal: number;
	    bytesDone: number;
	    filesTotal: number;
	    filesDone: number;
	    startedAt: number;
	    finishedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new Transfer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.sessionId = source["sessionId"];
	        this.direction = source["direction"];
	        this.source = source["source"];
	        this.dest = source["dest"];
	        this.name = source["name"];
	        this.state = source["state"];
	        this.error = source["error"];
	        this.bytesTotal = source["bytesTotal"];
	        this.bytesDone = source["bytesDone"];
	        this.filesTotal = source["filesTotal"];
	        this.filesDone = source["filesDone"];
	        this.startedAt = source["startedAt"];
	        this.finishedAt = source["finishedAt"];
	    }
	}
	export class UptimeCheck {
	    id: string;
	    name: string;
	    kind: string;
	    target: string;
	    intervalSec: number;
	    timeoutSec: number;
	    enabled: boolean;
	    createdAt: number;
	
	    static createFrom(source: any = {}) {
	        return new UptimeCheck(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.target = source["target"];
	        this.intervalSec = source["intervalSec"];
	        this.timeoutSec = source["timeoutSec"];
	        this.enabled = source["enabled"];
	        this.createdAt = source["createdAt"];
	    }
	}
	export class UptimeResult {
	    checkId: string;
	    at: number;
	    ok: boolean;
	    latencyMs: number;
	    statusCode: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new UptimeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.checkId = source["checkId"];
	        this.at = source["at"];
	        this.ok = source["ok"];
	        this.latencyMs = source["latencyMs"];
	        this.statusCode = source["statusCode"];
	        this.error = source["error"];
	    }
	}
	export class UptimeSummary {
	    check: UptimeCheck;
	    up?: boolean;
	    lastAt: number;
	    latencyMs: number;
	    lastError: string;
	    uptime24h: number;
	    uptime7d: number;
	    uptime30d: number;
	    recent: UptimeResult[];
	    downSince: number;
	
	    static createFrom(source: any = {}) {
	        return new UptimeSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.check = this.convertValues(source["check"], UptimeCheck);
	        this.up = source["up"];
	        this.lastAt = source["lastAt"];
	        this.latencyMs = source["latencyMs"];
	        this.lastError = source["lastError"];
	        this.uptime24h = source["uptime24h"];
	        this.uptime7d = source["uptime7d"];
	        this.uptime30d = source["uptime30d"];
	        this.recent = this.convertValues(source["recent"], UptimeResult);
	        this.downSince = source["downSince"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace vaultsync {
	
	export class DeviceCode {
	    userCode: string;
	    verificationUri: string;
	    expiresIn: number;
	    interval: number;
	
	    static createFrom(source: any = {}) {
	        return new DeviceCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.userCode = source["userCode"];
	        this.verificationUri = source["verificationUri"];
	        this.expiresIn = source["expiresIn"];
	        this.interval = source["interval"];
	    }
	}
	export class Repo {
	    fullName: string;
	    private: boolean;
	    defaultBranch: string;
	    htmlUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new Repo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fullName = source["fullName"];
	        this.private = source["private"];
	        this.defaultBranch = source["defaultBranch"];
	        this.htmlUrl = source["htmlUrl"];
	    }
	}
	export class RepoInfo {
	    fullName: string;
	    private: boolean;
	    defaultBranch: string;
	    htmlUrl: string;
	    hasVault: boolean;
	    empty: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RepoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fullName = source["fullName"];
	        this.private = source["private"];
	        this.defaultBranch = source["defaultBranch"];
	        this.htmlUrl = source["htmlUrl"];
	        this.hasVault = source["hasVault"];
	        this.empty = source["empty"];
	    }
	}
	export class Status {
	    appConfigured: boolean;
	    clientId: string;
	    appSlug: string;
	    builtInApp: boolean;
	    connected: boolean;
	    repo: string;
	    repoUrl: string;
	    login: string;
	    state: string;
	    lastSyncAt: number;
	    error: string;
	    errorCode: string;
	    installUrl: string;
	    newRepoUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appConfigured = source["appConfigured"];
	        this.clientId = source["clientId"];
	        this.appSlug = source["appSlug"];
	        this.builtInApp = source["builtInApp"];
	        this.connected = source["connected"];
	        this.repo = source["repo"];
	        this.repoUrl = source["repoUrl"];
	        this.login = source["login"];
	        this.state = source["state"];
	        this.lastSyncAt = source["lastSyncAt"];
	        this.error = source["error"];
	        this.errorCode = source["errorCode"];
	        this.installUrl = source["installUrl"];
	        this.newRepoUrl = source["newRepoUrl"];
	    }
	}

}

