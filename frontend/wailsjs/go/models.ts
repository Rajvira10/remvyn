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
	
	export class AgentApproval {
	    id: string;
	    client: string;
	    hostId: string;
	    hostLabel: string;
	    remote: string;
	    tool: string;
	    title: string;
	    detail: string;
	    preview?: string;
	    createdAt: number;
	    expiresAt: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentApproval(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.client = source["client"];
	        this.hostId = source["hostId"];
	        this.hostLabel = source["hostLabel"];
	        this.remote = source["remote"];
	        this.tool = source["tool"];
	        this.title = source["title"];
	        this.detail = source["detail"];
	        this.preview = source["preview"];
	        this.createdAt = source["createdAt"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
	export class AgentGrant {
	    hostId: string;
	    until: number;
	
	    static createFrom(source: any = {}) {
	        return new AgentGrant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hostId = source["hostId"];
	        this.until = source["until"];
	    }
	}
	export class AgentLogEntry {
	    id: string;
	    at: number;
	    client: string;
	    hostId: string;
	    hostLabel: string;
	    tool: string;
	    summary: string;
	    status: string;
	    approval: string;
	    exitCode?: number;
	    durationMs: number;
	    output: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentLogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.client = source["client"];
	        this.hostId = source["hostId"];
	        this.hostLabel = source["hostLabel"];
	        this.tool = source["tool"];
	        this.summary = source["summary"];
	        this.status = source["status"];
	        this.approval = source["approval"];
	        this.exitCode = source["exitCode"];
	        this.durationMs = source["durationMs"];
	        this.output = source["output"];
	    }
	}
	export class AgentStatus {
	    enabled: boolean;
	    listening: boolean;
	    listenError?: string;
	    socketPath: string;
	    command: string[];
	    running: number;
	    grants: AgentGrant[];
	
	    static createFrom(source: any = {}) {
	        return new AgentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.listening = source["listening"];
	        this.listenError = source["listenError"];
	        this.socketPath = source["socketPath"];
	        this.command = source["command"];
	        this.running = source["running"];
	        this.grants = this.convertValues(source["grants"], AgentGrant);
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
	    persistKey: string;
	    restore: boolean;
	
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
	        this.persistKey = source["persistKey"];
	        this.restore = source["restore"];
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
	export class Editor {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new Editor(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class FileContent {
	    path: string;
	    content: string;
	    size: number;
	    modTime: number;
	
	    static createFrom(source: any = {}) {
	        return new FileContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
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
	export class KeyUseRequest {
	    id: string;
	    hostId: string;
	    hostLabel: string;
	    remote: string;
	    keyName: string;
	    fingerprint: string;
	    source: string;
	    ai: boolean;
	    createdAt: number;
	    expiresAt: number;
	
	    static createFrom(source: any = {}) {
	        return new KeyUseRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.hostId = source["hostId"];
	        this.hostLabel = source["hostLabel"];
	        this.remote = source["remote"];
	        this.keyName = source["keyName"];
	        this.fingerprint = source["fingerprint"];
	        this.source = source["source"];
	        this.ai = source["ai"];
	        this.createdAt = source["createdAt"];
	        this.expiresAt = source["expiresAt"];
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
	export class ServerSession {
	    key: string;
	    attached: boolean;
	    lastActive: number;
	    history: number;
	    command: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.attached = source["attached"];
	        this.lastActive = source["lastActive"];
	        this.history = source["history"];
	        this.command = source["command"];
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
	    persistent: boolean;
	    resumed: boolean;
	    persistNote?: string;
	    agentForwarding: boolean;
	    detached: ServerSession[];
	
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
	        this.persistent = source["persistent"];
	        this.resumed = source["resumed"];
	        this.persistNote = source["persistNote"];
	        this.agentForwarding = source["agentForwarding"];
	        this.detached = this.convertValues(source["detached"], ServerSession);
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
	    persistentSessions: boolean;
	    monitorHostIds: string[];
	    monitorIntervalSec: number;
	    metricsRetentionDays: number;
	    alertNotifications: boolean;
	    agentEnabled: boolean;
	    agentHosts: Record<string, string>;
	    forwardKeyHosts: string[];
	    forwardKeysToAI: boolean;
	    autoUpdateCheck: boolean;
	
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
	        this.persistentSessions = source["persistentSessions"];
	        this.monitorHostIds = source["monitorHostIds"];
	        this.monitorIntervalSec = source["monitorIntervalSec"];
	        this.metricsRetentionDays = source["metricsRetentionDays"];
	        this.alertNotifications = source["alertNotifications"];
	        this.agentEnabled = source["agentEnabled"];
	        this.agentHosts = source["agentHosts"];
	        this.forwardKeyHosts = source["forwardKeyHosts"];
	        this.forwardKeysToAI = source["forwardKeysToAI"];
	        this.autoUpdateCheck = source["autoUpdateCheck"];
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
	export class TOTPCode {
	    code: string;
	    expiresAt: number;
	    period: number;
	
	    static createFrom(source: any = {}) {
	        return new TOTPCode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.code = source["code"];
	        this.expiresAt = source["expiresAt"];
	        this.period = source["period"];
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
	export class VaultField {
	    name: string;
	    value: string;
	    hidden: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VaultField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.value = source["value"];
	        this.hidden = source["hidden"];
	    }
	}
	export class VaultItem {
	    id: string;
	    kind: string;
	    name: string;
	    username: string;
	    url: string;
	    favorite: boolean;
	    hasPassword: boolean;
	    hasTotp: boolean;
	    hasNotes: boolean;
	    fieldCount: number;
	    createdAt: number;
	    updatedAt: number;
	
	    static createFrom(source: any = {}) {
	        return new VaultItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.kind = source["kind"];
	        this.name = source["name"];
	        this.username = source["username"];
	        this.url = source["url"];
	        this.favorite = source["favorite"];
	        this.hasPassword = source["hasPassword"];
	        this.hasTotp = source["hasTotp"];
	        this.hasNotes = source["hasNotes"];
	        this.fieldCount = source["fieldCount"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	}
	export class VaultSecret {
	    password: string;
	    totp: string;
	    notes: string;
	    fields: VaultField[];
	
	    static createFrom(source: any = {}) {
	        return new VaultSecret(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.password = source["password"];
	        this.totp = source["totp"];
	        this.notes = source["notes"];
	        this.fields = this.convertValues(source["fields"], VaultField);
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
	export class VaultItemInput {
	    item: VaultItem;
	    secret?: VaultSecret;
	
	    static createFrom(source: any = {}) {
	        return new VaultItemInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.item = this.convertValues(source["item"], VaultItem);
	        this.secret = this.convertValues(source["secret"], VaultSecret);
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

export namespace updater {
	
	export class Release {
	    version: string;
	    name: string;
	    notes: string;
	    url: string;
	    // Go type: time
	    publishedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Release(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.name = source["name"];
	        this.notes = source["notes"];
	        this.url = source["url"];
	        this.publishedAt = this.convertValues(source["publishedAt"], null);
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
	export class Status {
	    current: string;
	    state: string;
	    latest?: Release;
	    available: boolean;
	    // Go type: time
	    checkedAt: any;
	    downloaded: number;
	    total: number;
	    error?: string;
	    manual?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.state = source["state"];
	        this.latest = this.convertValues(source["latest"], Release);
	        this.available = source["available"];
	        this.checkedAt = this.convertValues(source["checkedAt"], null);
	        this.downloaded = source["downloaded"];
	        this.total = source["total"];
	        this.error = source["error"];
	        this.manual = source["manual"];
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

