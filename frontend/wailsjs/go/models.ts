export namespace chat {
	
	export class CategoryUsage {
	    name: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new CategoryUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	    }
	}
	export class Message {
	    id: string;
	    role: string;
	    content: string;
	    status: string;
	    // Go type: time
	    createdAt: any;
	    tool?: string;
	    arguments?: Record<string, any>;
	    result?: any;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.status = source["status"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.tool = source["tool"];
	        this.arguments = source["arguments"];
	        this.result = source["result"];
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
	export class Conversation {
	    id: string;
	    projectId: string;
	    provider: string;
	    title: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	    archived?: boolean;
	    initialized: boolean;
	    messages: Message[];
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.provider = source["provider"];
	        this.title = source["title"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.archived = source["archived"];
	        this.initialized = source["initialized"];
	        this.messages = this.convertValues(source["messages"], Message);
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
	export class DayUsage {
	    date: string;
	    messages: number;
	    toolCalls: number;
	
	    static createFrom(source: any = {}) {
	        return new DayUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.date = source["date"];
	        this.messages = source["messages"];
	        this.toolCalls = source["toolCalls"];
	    }
	}
	export class Summary {
	    id: string;
	    projectId: string;
	    provider: string;
	    title: string;
	    // Go type: time
	    updatedAt: any;
	    archived: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.provider = source["provider"];
	        this.title = source["title"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.archived = source["archived"];
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
	export class List {
	    active: Summary[];
	    archived: Summary[];
	
	    static createFrom(source: any = {}) {
	        return new List(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.active = this.convertValues(source["active"], Summary);
	        this.archived = this.convertValues(source["archived"], Summary);
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
	
	
	export class ToolUsage {
	    name: string;
	    category: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new ToolUsage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.category = source["category"];
	        this.count = source["count"];
	    }
	}
	export class Usage {
	    conversations: number;
	    archivedCount: number;
	    messages: number;
	    toolCalls: number;
	    tools: ToolUsage[];
	    categories: CategoryUsage[];
	    days: DayUsage[];
	
	    static createFrom(source: any = {}) {
	        return new Usage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversations = source["conversations"];
	        this.archivedCount = source["archivedCount"];
	        this.messages = source["messages"];
	        this.toolCalls = source["toolCalls"];
	        this.tools = this.convertValues(source["tools"], ToolUsage);
	        this.categories = this.convertValues(source["categories"], CategoryUsage);
	        this.days = this.convertValues(source["days"], DayUsage);
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

export namespace config {
	
	export class Project {
	    id: string;
	    name: string;
	    path: string;
	    addedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.addedAt = source["addedAt"];
	    }
	}

}

export namespace main {
	
	export class ActivityStatus {
	    tool?: string;
	    time?: string;
	
	    static createFrom(source: any = {}) {
	        return new ActivityStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tool = source["tool"];
	        this.time = source["time"];
	    }
	}
	export class ExtensionStatus {
	    connected: boolean;
	    lastSeen?: string;
	
	    static createFrom(source: any = {}) {
	        return new ExtensionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.lastSeen = source["lastSeen"];
	    }
	}
	export class PendingApproval {
	    id: string;
	    tool: string;
	    summary: string;
	    createdAt: string;
	    expiresAt: string;
	
	    static createFrom(source: any = {}) {
	        return new PendingApproval(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.tool = source["tool"];
	        this.summary = source["summary"];
	        this.createdAt = source["createdAt"];
	        this.expiresAt = source["expiresAt"];
	    }
	}
	export class ServiceStatus {
	    running: boolean;
	    url: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.url = source["url"];
	        this.error = source["error"];
	    }
	}
	export class Snapshot {
	    service: ServiceStatus;
	    extension: ExtensionStatus;
	    provider: ExtensionStatus;
	    activity: ActivityStatus;
	    projects: config.Project[];
	    tokenConfigured: boolean;
	    automationMode: string;
	    toolCallLimit: number;
	    pendingApprovals: PendingApproval[];
	    usage: chat.Usage;
	    toolCount: number;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.service = this.convertValues(source["service"], ServiceStatus);
	        this.extension = this.convertValues(source["extension"], ExtensionStatus);
	        this.provider = this.convertValues(source["provider"], ExtensionStatus);
	        this.activity = this.convertValues(source["activity"], ActivityStatus);
	        this.projects = this.convertValues(source["projects"], config.Project);
	        this.tokenConfigured = source["tokenConfigured"];
	        this.automationMode = source["automationMode"];
	        this.toolCallLimit = source["toolCallLimit"];
	        this.pendingApprovals = this.convertValues(source["pendingApprovals"], PendingApproval);
	        this.usage = this.convertValues(source["usage"], chat.Usage);
	        this.toolCount = source["toolCount"];
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

