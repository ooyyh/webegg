import {useCallback, useEffect, useState} from 'react';
import type {ReactNode} from 'react';
import './App.css';
import {ChatWorkspace} from './chat/ChatWorkspace';
import {UsageCharts} from './components/UsageCharts';
import {AddProject, CopyText, GenerateToken, GetSnapshot, RemoveProject, ResolveApproval, SetAutomationMode, SetToolCallLimit} from '../wailsjs/go/main/App';
import {config, main} from '../wailsjs/go/models';

type Tab = 'chats' | 'overview' | 'projects' | 'tools' | 'connection';
type Theme = 'dark' | 'light';
type IconName = 'grid' | 'folder' | 'tools' | 'link' | 'plus' | 'trash' | 'copy' | 'key' | 'shield' | 'check' | 'alert' | 'plug' | 'arrow' | 'browser' | 'clock' | 'code' | 'terminal' | 'search' | 'git' | 'package' | 'zap' | 'activity' | 'close' | 'chevron' | 'message' | 'file';

const EMPTY_SNAPSHOT = main.Snapshot.createFrom({
    service: {running: false, url: 'http://127.0.0.1:8787/mcp'},
    extension: {connected: false}, provider: {connected: false}, activity: {}, projects: [], tokenConfigured: false,
    automationMode: 'automatic', toolCallLimit: 0, pendingApprovals: [],
    usage: {conversations: 0, archivedCount: 0, messages: 0, toolCalls: 0, tools: [], categories: [], days: []}, toolCount: 0,
});

const NAV_ITEMS: {id: Tab; label: string; icon: IconName}[] = [
    {id: 'chats', label: '对话', icon: 'message'},
    {id: 'overview', label: '概览', icon: 'grid'},
    {id: 'projects', label: '项目', icon: 'folder'},
    {id: 'tools', label: '工具与审批', icon: 'tools'},
    {id: 'connection', label: '连接', icon: 'link'},
];

const PAGE_COPY: Record<Tab, {eyebrow: string; title: string; description: string}> = {
    chats: {eyebrow: 'LOCAL CONVERSATIONS', title: '项目对话', description: '每个项目独立会话，工具与结果在本地清晰呈现。'},
    overview: {eyebrow: 'WORKSPACE', title: '本地 AI 编程工作台', description: '连接、工具与项目状态，尽在一个安静而精确的控制面。'},
    projects: {eyebrow: 'AUTHORIZED PROJECTS', title: '项目授权', description: '文件工具只能在显式授权的目录边界内工作。'},
    tools: {eyebrow: 'TOOLING & APPROVALS', title: '工具与审批', description: '真实 MCP 工具、能力分类，以及由你决定的自动化边界。'},
    connection: {eyebrow: 'CONNECTION', title: '连接 Gemini', description: '本地凭证、实时状态和浏览器协作入口。'},
};

const TOOL_GROUPS: {title: string; icon: IconName; tone: string; tools: {name: string; description: string}[]}[] = [
    {title: '项目与目录', icon: 'folder', tone: 'lime', tools: [
        {name: 'webegg_project_list', description: '发现授权项目'}, {name: 'webegg_project_info', description: '项目元数据'},
        {name: 'webegg_directory_list', description: '浏览目录结构'}, {name: 'webegg_directory_create', description: '创建单个目录'},
        {name: 'webegg_directory_delete', description: '删除空目录'},
    ]},
    {title: '文件读取', icon: 'file', tone: 'blue', tools: [
        {name: 'webegg_file_stat', description: '元数据与哈希'}, {name: 'webegg_file_read', description: '有界文本读取'},
    ]},
    {title: '文件编辑', icon: 'code', tone: 'cyan', tools: [
        {name: 'webegg_file_create', description: '创建完整文件'}, {name: 'webegg_file_write', description: '原子覆盖与备份'},
        {name: 'webegg_file_patch', description: '唯一匹配补丁'}, {name: 'webegg_file_delete', description: '备份后安全删除'},
        {name: 'webegg_file_move', description: '移动或重命名'}, {name: 'webegg_file_copy', description: '复制文本文件'},
    ]},
    {title: '搜索', icon: 'search', tone: 'violet', tools: [
        {name: 'webegg_glob', description: '递归文件模式匹配'}, {name: 'webegg_grep', description: '项目正则内容搜索'},
    ]},
    {title: 'Git', icon: 'git', tone: 'amber', tools: [
        {name: 'webegg_git_status', description: '分支与工作区状态'}, {name: 'webegg_git_diff', description: '安全只读差异'},
        {name: 'webegg_git_log', description: '最近提交记录'}, {name: 'webegg_git_branches', description: '本地分支列表'},
        {name: 'webegg_git_show', description: '单次提交统计'},
    ]},
    {title: '执行', icon: 'terminal', tone: 'lime', tools: [
        {name: 'webegg_shell_run', description: '受控短时 Shell 命令'}, {name: 'webegg_shell_start', description: '启动后台服务器'},
        {name: 'webegg_process_list', description: '后台进程状态'}, {name: 'webegg_process_logs', description: '读取进程日志'},
        {name: 'webegg_process_stop', description: '停止进程树'}, {name: 'webegg_project_script', description: 'npm / pnpm 构建测试'},
    ]},
];

function Icon({name, size = 18}: {name: IconName; size?: number}) {
    const paths: Record<IconName, ReactNode> = {
        grid: <><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></>,
        folder: <path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>,
        tools: <><path d="m14.7 6.3 3-3a4 4 0 0 1-5 5l-6.4 6.4a2.1 2.1 0 0 0 3 3l6.4-6.4a4 4 0 0 1 5-5l-3 3Z"/></>,
        link: <><path d="M10 13a5 5 0 0 0 7.5.5l2-2a5 5 0 0 0-7-7l-1.2 1.2M14 11a5 5 0 0 0-7.5-.5l-2 2a5 5 0 0 0 7 7l1.2-1.2"/></>,
        plus: <path d="M12 5v14M5 12h14"/>,
        trash: <><path d="M4 7h16M9 7V4h6v3M7 7l1 13h8l1-13M10 11v5M14 11v5"/></>,
        copy: <><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></>,
        key: <><circle cx="8" cy="15" r="4"/><path d="m11 12 8-8M15 8l2 2M17 6l2 2"/></>,
        shield: <><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/><path d="m9 12 2 2 4-4"/></>,
        check: <path d="m5 12 4 4L19 6"/>,
        alert: <><path d="M12 3 2.8 19h18.4ZM12 9v4M12 17h.01"/></>,
        plug: <><path d="m8 12 8-8M14 3l7 7M6 10l8 8M3 15l6 6M8 16l-4 4M16 8l4-4"/></>,
        arrow: <path d="M5 12h14M14 7l5 5-5 5"/>,
        browser: <><rect x="3" y="4" width="18" height="16" rx="3"/><path d="M3 9h18M7 6.5h.01M10 6.5h.01"/></>,
        clock: <><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></>,
        code: <><path d="m8 9-4 3 4 3M16 9l4 3-4 3M14 5l-4 14"/></>,
        terminal: <><rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3M13 15h4"/></>,
        search: <><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/></>,
        git: <><circle cx="6" cy="5" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="6" cy="19" r="2"/><path d="M6 7v10M8 10c4 0 4-2 8-2"/></>,
        package: <><path d="m12 3 9 5-9 5-9-5ZM3 8v9l9 5 9-5V8M12 13v9"/></>,
        zap: <path d="m13 2-9 12h7l-1 8 9-12h-7Z"/>,
        activity: <path d="M3 12h4l2-7 4 14 2-7h6"/>,
        close: <path d="m6 6 12 12M18 6 6 18"/>,
        chevron: <path d="m9 6 6 6-6 6"/>,
        message: <path d="M20 15a3 3 0 0 1-3 3H9l-5 3v-6a3 3 0 0 1-1-2V6a3 3 0 0 1 3-3h11a3 3 0 0 1 3 3Z"/>,
        file: <><path d="M6 2h8l4 4v16H6Z"/><path d="M14 2v5h5M9 12h6M9 16h6"/></>,    };
    return <svg className="icon" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>;
}

function Brand({compact = false}: {compact?: boolean}) {
    return <div className={`brand ${compact ? 'compact' : ''}`}>
        <div className="brand-symbol" aria-hidden="true"><svg viewBox="0 0 32 32" fill="none"><path d="M16 4C10.7 4 6 11.1 6 18a10 10 0 0 0 20 0C26 11.1 21.3 4 16 4Z" stroke="currentColor" strokeWidth="1.8"/><path d="M11 17c.3-3.1 1.8-6.1 4-7.8" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"/></svg></div>
        {!compact && <div><strong>WebEgg</strong><span>LOCAL AI CONTROL</span></div>}
    </div>;
}

function ModeControl({mode, onChange, limit, onLimitChange, busy}: {mode: string; onChange: (mode: string) => void; limit: number; onLimitChange: (limit: number) => void; busy: boolean}) {
    const automatic = mode === 'automatic';
    return <div className="mode-control">
        <div className="segmented" role="group" aria-label="工具执行模式">
            <button className={automatic ? 'active' : ''} onClick={() => onChange('automatic')} disabled={busy} aria-pressed={automatic}><Icon name="zap" size={14}/>全自动</button>
            <button className={!automatic ? 'active approval' : ''} onClick={() => onChange('approval')} disabled={busy} aria-pressed={!automatic}><Icon name="shield" size={14}/>需批准</button>
        </div>
        <p>{automatic ? '所有工具自动执行，适合受信任的连续编程任务。' : '读取自动执行，高危操作由你在本地逐项批准。'}</p>
        <div className={`mode-status ${automatic ? 'auto' : 'approval'}`}><span/>{automatic ? 'AUTONOMOUS' : 'SUPERVISED'}</div>
        <div className="limit-control"><label htmlFor={`call-limit-${automatic ? 'auto' : 'approval'}`}>连续调用熔断</label><select id={`call-limit-${automatic ? 'auto' : 'approval'}`} value={limit} onChange={event => onLimitChange(Number(event.target.value))} disabled={busy}><option value={0}>不限次数</option><option value={20}>20 次</option><option value={50}>50 次</option><option value={100}>100 次</option><option value={200}>200 次</option></select></div>
    </div>;
}

function ApprovalCenter({items, onResolve, busy}: {items: main.PendingApproval[]; onResolve: (item: main.PendingApproval, approved: boolean) => void; busy: boolean}) {
    if (!items.length) return null;
    return <section className="approval-center">
        <div className="approval-title"><span className="approval-icon"><Icon name="alert"/></span><div><h2>等待你的决定</h2><p>操作尚未执行 · 批准后自动继续</p></div><span className="count-badge">{items.length}</span></div>
        <div className="approval-items">{items.map(item => <article className="approval-item" key={item.id}>
            <div><strong>{item.summary}</strong><code>{item.tool}</code></div>
            <div className="approval-buttons"><button className="btn small ghost" onClick={() => onResolve(item, false)} disabled={busy}>拒绝</button><button className="btn small primary" onClick={() => onResolve(item, true)} disabled={busy}>批准执行</button></div>
        </article>)}</div>
    </section>;
}

function App() {
    const [theme, setTheme] = useState<Theme>(() => {
        try { return localStorage.getItem('webegg-theme') === 'light' ? 'light' : 'dark'; } catch { return 'dark'; }
    });
    const [snapshot, setSnapshot] = useState<main.Snapshot>(EMPTY_SNAPSHOT);
    const [activeTab, setActiveTab] = useState<Tab>('chats');
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');
    const [notice, setNotice] = useState('');
    const [newToken, setNewToken] = useState('');
    const [navOpen, setNavOpen] = useState(false);
    const [confirmDialog, setConfirmDialog] = useState<{title: string; message: string; confirmLabel: string; danger?: boolean; onConfirm: () => void} | null>(null);

    const refresh = useCallback(async (clearError = true) => {
        try {
            const current = await GetSnapshot();
            setSnapshot(current);
            if (clearError) setError('');
        } catch (reason) {
            setError(String(reason));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        document.documentElement.style.colorScheme = theme;
        try { localStorage.setItem('webegg-theme', theme); } catch { /* WebView storage may be unavailable in private mode. */ }
    }, [theme]);

    useEffect(() => {
        void refresh();
        const timer = window.setInterval(() => void refresh(false), 3000);
        return () => window.clearInterval(timer);
    }, [refresh]);

    async function run(action: string, task: () => Promise<void>) {
        setBusy(action); setError(''); setNotice('');
        try { await task(); } catch (reason) { setError(String(reason)); } finally { setBusy(''); }
    }
    function addProject() {
        void run('project', async () => {
            const project = await AddProject() as config.Project | null;
            await refresh();
            if (project) setNotice(`已授权项目「${project.name}」`);
        });
    }
    function removeProject(project: config.Project) {
        setConfirmDialog({title: `移除「${project.name}」？`, message: '只会移除项目的授权记录，不会删除任何本地文件。', confirmLabel: '移除授权', danger: true, onConfirm: () => {
            void run(`remove:${project.id}`, async () => { await RemoveProject(project.id); await refresh(); setNotice(`已移除项目「${project.name}」`); });
        }});
    }
    function generateToken() {
        if (snapshot.tokenConfigured) {
            setConfirmDialog({title: '重新生成连接令牌？', message: '旧令牌会立即失效，所有已经使用旧令牌的连接都需要重新配置。', confirmLabel: '重新生成', danger: true, onConfirm: issueToken});
            return;
        }
        issueToken();
    }
    function issueToken() {
        void run('token', async () => { const token = await GenerateToken(); setNewToken(token); await refresh(); setNotice('新令牌已生成'); });
    }
    function copy(value: string, message: string) { void run('copy', async () => { await CopyText(value); setNotice(message); }); }
    function changeAutomationMode(mode: string) {
        if (mode === snapshot.automationMode) return;
        if (mode === 'automatic') {
            setConfirmDialog({title: '开启全自动执行？', message: '文件修改、删除和 Shell 命令将不再逐次请求本地批准。请仅在信任当前任务和项目时使用。', confirmLabel: '开启全自动', danger: true, onConfirm: () => applyAutomationMode(mode)});
            return;
        }
        applyAutomationMode(mode);
    }
    function applyAutomationMode(mode: string) {
        void run('mode', async () => { await SetAutomationMode(mode); await refresh(); setNotice(mode === 'automatic' ? '已切换为全自动执行' : '已切换为本地批准模式'); });
    }
    function changeToolCallLimit(limit: number) {
        void run('call-limit', async () => { await SetToolCallLimit(limit); await refresh(); setNotice(limit === 0 ? '已取消连续调用次数限制' : `连续调用熔断已设为 ${limit} 次`); });
    }
    function resolveApproval(request: main.PendingApproval, approved: boolean) {
        void run(`approval:${request.id}`, async () => { await ResolveApproval(request.id, approved); await refresh(); setNotice(approved ? '已批准，插件将自动继续' : '已拒绝这次操作'); });
    }

    const serviceRunning = snapshot.service.running;
    const connected = snapshot.extension?.connected ?? false;
    const providerReady = snapshot.provider?.connected ?? false;
    const lastSeen = snapshot.extension?.lastSeen;
    const activity = snapshot.activity?.tool;
    const activityTime = snapshot.activity?.time;
    const pageCopy = PAGE_COPY[activeTab];

    return <div className="app-layout" data-theme={theme} data-view={activeTab === 'chats' ? 'chat' : 'page'}>
        {activeTab !== 'chats' && <aside className="sidebar">
            <div className="sidebar-brand"><Brand/></div>
            <nav className="main-nav" aria-label="主导航">
                <span className="nav-label">控制面板</span>
                {NAV_ITEMS.map(item => <button key={item.id} className={activeTab === item.id ? 'active' : ''} onClick={() => setActiveTab(item.id)} title={item.label} aria-current={activeTab === item.id ? 'page' : undefined} aria-label={item.label}>
                    <Icon name={item.icon}/><span>{item.label}</span>
                    {item.id === 'tools' && snapshot.pendingApprovals.length > 0 && <b>{snapshot.pendingApprovals.length}</b>}
                    {activeTab === item.id && <i/>}
                </button>)}
            </nav>
            <div className="sidebar-spacer"/>
            <button className="sidebar-mode" onClick={() => setActiveTab('tools')}>
                <span className={`mode-orb ${snapshot.automationMode}`}><Icon name={snapshot.automationMode === 'automatic' ? 'zap' : 'shield'} size={15}/></span>
                <span><strong>{snapshot.automationMode === 'automatic' ? '全自动执行' : '本地批准模式'}</strong><small>点击调整执行策略</small></span><Icon name="chevron" size={14}/>
            </button>
            <div className="sidebar-footer"><span>WebEgg v0.9.8</span><span>WINDOWS</span></div>
        </aside>}

        <div className={`app-body ${activeTab === 'chats' ? 'chat-full' : ''}`}>
            {activeTab !== 'chats' && <header className="topbar">
                <div className="breadcrumb"><span>WEBEGG</span><Icon name="chevron" size={12}/><strong>{pageCopy.eyebrow}</strong></div>
                <div className="topbar-status">
                    <ThemeToggle theme={theme} onChange={() => setTheme(theme === 'dark' ? 'light' : 'dark')}/>
                    <span className={`status-chip ${serviceRunning ? 'online' : 'offline'}`}><i/>{serviceRunning ? 'MCP 运行中' : 'MCP 离线'}</span>
                    <span className={`status-chip ${connected ? 'online' : ''}`}><i/>{connected ? 'Gemini 已连接' : '等待 Gemini'}</span>
                </div>
            </header>}

            <main className={`page-content ${activeTab === 'chats' ? 'chat-mode' : ''}`}>
                {activeTab !== 'chats' && <div className="page-heading"><div><span className="eyebrow">{pageCopy.eyebrow}</span><h1>{pageCopy.title}</h1><p>{pageCopy.description}</p></div>
                    {activeTab === 'projects' && <button className="btn primary" onClick={addProject} disabled={Boolean(busy)}><Icon name="plus" size={16}/>添加项目</button>}
                    {activeTab === 'overview' && <button className="btn secondary" onClick={() => setActiveTab('connection')}>连接设置<Icon name="arrow" size={15}/></button>}
                </div>}

                {(error || snapshot.service.error) && <div className="system-alert" role="alert"><Icon name="alert"/><span>{error || snapshot.service.error}</span></div>}
                {notice && <div className="toast" role="status"><span><Icon name="check" size={15}/>{notice}</span><button aria-label="关闭通知" onClick={() => setNotice('')}><Icon name="close" size={14}/></button></div>}

                <div className={`tab-page ${activeTab === 'chats' ? 'chat-tab-page' : ''}`} key={activeTab}>
                    {activeTab === 'chats' && <ChatWorkspace theme={theme} onThemeChange={() => setTheme(theme === 'dark' ? 'light' : 'dark')} onOpenMenu={() => setNavOpen(true)} projects={snapshot.projects} connected={connected} providerReady={providerReady} providerLastSeen={snapshot.provider?.lastSeen} toolCount={snapshot.toolCount} onAddProject={addProject}/>}
                    {activeTab === 'overview' && <>
                        <section className="hero-panel">
                            <div className="hero-copy"><span className="hero-label"><i/>LOCAL WORKSPACE</span><h2>让模型真正进入<br/><em>你的开发工作流。</em></h2><p>项目、文件、搜索、Git 与执行工具，<br/>通过本地 MCP 安全协作。</p><button className="btn primary" onClick={() => setActiveTab('connection')}>开始协作<Icon name="arrow" size={16}/></button></div>
                            <div className="hero-visual" aria-hidden="true"><div className="orbit orbit-one"/><div className="orbit orbit-two"/><div className="orbit orbit-three"/><div className="hero-core"><Brand compact/></div><span className="orbit-node node-one"><Icon name="code" size={17}/></span><span className="orbit-node node-two"><Icon name="git" size={17}/></span><span className="orbit-node node-three"><Icon name="terminal" size={17}/></span></div>
                            <div className="hero-foot"><span><i className={serviceRunning ? 'ok' : ''}/>{serviceRunning ? '本地服务正常' : '本地服务离线'}</span><span><i className={connected ? 'ok' : ''}/>{connected ? '浏览器已连接' : '等待浏览器连接'}</span><span><i className="ok"/>{snapshot.toolCount} 项工具已注册</span></div>
                        </section>

                        <section className="metric-grid">
                            <Metric icon="plug" label="本地 MCP" value={serviceRunning ? '运行正常' : '服务离线'} detail="127.0.0.1:8787" active={serviceRunning}/>
                            <Metric icon="browser" label="Gemini 连接器" value={connected ? '连接成功' : lastSeen ? '最近已连接' : '等待连接'} detail={lastSeen ? `最近 ${formatTime(lastSeen)}` : '尚未建立连接'} active={connected}/>
                            <Metric icon="folder" label="授权项目" value={`${snapshot.projects.length} 个`} detail="本地目录边界"/>
                            <Metric icon="activity" label="最近工具调用" value={activity ? activity.replace('webegg_', '') : '暂无调用'} detail={activityTime ? formatTime(activityTime) : '等待第一次工具协作'} active={Boolean(activity)}/>
                        </section>

                        <UsageCharts usage={snapshot.usage}/>

                        <ApprovalCenter items={snapshot.pendingApprovals} onResolve={resolveApproval} busy={Boolean(busy)}/>

                        <div className="overview-grid">
                            <section className="panel activity-panel">
                                <PanelHeader icon="activity" title="协作动态" subtitle="实时反映本地工具活动" action={<span className="live-label"><i/>LIVE</span>}/>
                                {activity ? <div className="current-activity"><div className="activity-pulse"><Icon name="zap" size={22}/></div><div><span>最近执行</span><h3>{activity}</h3><p>{activityTime ? formatTime(activityTime) : ''} · 通过 Gemini 连接器</p></div></div> : <div className="workflow-steps">
                                    <WorkflowStep number="01" title="建立本地连接" text="启动 WebEgg 并在插件中保存令牌" done={connected}/>
                                    <WorkflowStep number="02" title="自动连接网页" text="新建会话后自动注入工具协议" done={providerReady}/>
                                    <WorkflowStep number="03" title="开始工具优先协作" text="模型会主动读取、搜索、修改并验证"/>
                                </div>}
                                <div className="activity-footer"><Icon name="shield" size={14}/>工具输出在协议中被标记为数据，而非高优先级指令</div>
                            </section>
                            <section className="panel strategy-panel"><PanelHeader icon="zap" title="执行策略" subtitle="自动化程度由你控制"/><ModeControl mode={snapshot.automationMode} onChange={changeAutomationMode} limit={snapshot.toolCallLimit} onLimitChange={changeToolCallLimit} busy={Boolean(busy)}/><div className="security-note"><Icon name="alert" size={15}/><span>Shell 不是操作系统沙箱。处理陌生项目时建议使用需批准模式。</span></div></section>
                        </div>

                        <section className="panel recent-panel"><PanelHeader icon="folder" title="最近项目" subtitle="快速访问你的本地工作区" action={<button className="text-action" onClick={() => setActiveTab('projects')}>全部项目<Icon name="arrow" size={14}/></button>}/>
                            {snapshot.projects.length ? <div className="recent-projects">{snapshot.projects.slice(0, 3).map(project => <button className="recent-project" key={project.id} onClick={() => setActiveTab('projects')}><span className="file-icon"><Icon name="folder"/></span><span><strong>{project.name}</strong><small>{project.path}</small></span><Icon name="chevron" size={15}/></button>)}</div> : <EmptyState compact title="还没有授权项目" text="添加目录后，Gemini 才能开始本地编程协作。" action="添加第一个项目" onAction={addProject}/>}
                        </section>
                    </>}

                    {activeTab === 'projects' && <section className="panel projects-workspace">
                        <PanelHeader icon="folder" title="授权目录" subtitle={`${snapshot.projects.length} 个项目 · 物理路径仅用于本地执行`} action={<span className="soft-badge"><Icon name="shield" size={13}/>路径沙箱</span>}/>
                        {loading ? <div className="loading-state"><span/>正在读取工作区…</div> : snapshot.projects.length ? <div className="project-table"><div className="project-table-head"><span>项目</span><span>授权时间</span><span>项目标识</span><span/></div>{snapshot.projects.map(project => <div className="project-table-row" key={project.id}><div className="project-name"><span className="file-icon"><Icon name="folder"/></span><div><strong>{project.name}</strong><small title={project.path}>{project.path}</small></div></div><span className="project-date">{formatDate(project.addedAt)}</span><code>{project.id}</code><button className="icon-btn danger" aria-label={`移除 ${project.name}`} onClick={() => removeProject(project)} disabled={Boolean(busy)}><Icon name="trash" size={16}/></button></div>)}</div> : <EmptyState title="从第一个项目开始" text="选择一个本地目录。文件工具会限制在目录边界内，移除授权不会删除任何文件。" action="选择项目目录" onAction={addProject}/>}
                        <div className="workspace-note"><Icon name="shield" size={15}/><span>敏感文件、路径穿越和符号链接默认被拦截。Shell 不属于文件沙箱，请根据项目信任程度选择执行模式。</span></div>
                    </section>}

                    {activeTab === 'tools' && <>
                        <div className="tools-top-grid"><section className="panel strategy-panel"><PanelHeader icon="zap" title="自动化边界" subtitle="切换后立即生效并持久保存"/><ModeControl mode={snapshot.automationMode} onChange={changeAutomationMode} limit={snapshot.toolCallLimit} onLimitChange={changeToolCallLimit} busy={Boolean(busy)}/></section><section className="panel tool-activity-card"><PanelHeader icon="activity" title="实时调用" subtitle="最近一次 MCP tools/call"/><div className="tool-activity-value"><span className="activity-pulse"><Icon name="code" size={20}/></span><div><strong>{activity || '等待工具调用'}</strong><small>{activityTime ? formatTime(activityTime) : '发送协作协议后，模型会主动调用项目列表'}</small></div></div></section></div>
                        <ApprovalCenter items={snapshot.pendingApprovals} onResolve={resolveApproval} busy={Boolean(busy)}/>
                        <div className="tool-catalog">{TOOL_GROUPS.map(group => <section className={`panel tool-group ${group.tone}`} key={group.title}><div className="tool-group-title"><span><Icon name={group.icon}/></span><div><h2>{group.title}</h2><p>{group.tools.length} 项工具</p></div></div><div className="tool-list">{group.tools.map(tool => <div className="tool-item" key={tool.name}><Icon name="check" size={13}/><div><code>{tool.name}</code><span>{tool.description}</span></div></div>)}</div></section>)}</div>
                    </>}

                    {activeTab === 'connection' && <div className="connection-layout">
                        <div className="connection-main">
                            <section className="panel connection-status-card"><div className={`connection-orb ${connected ? 'connected' : ''}`}><Icon name="browser" size={26}/></div><div><span>GEMINI CONNECTOR</span><h2>{connected ? '连接成功' : lastSeen ? '最近已连接' : '等待连接'}</h2><p>{lastSeen ? `最近一次成功联系：${formatTime(lastSeen)}` : '在浏览器插件中保存配置后，这里会自动更新。'}</p></div><span className={`status-chip ${connected ? 'online' : ''}`}><i/>{connected ? 'ONLINE' : 'STANDBY'}</span></section>
                            <section className="panel credentials-panel"><PanelHeader icon="key" title="本地凭证" subtitle="仅用于浏览器插件与本机 MCP 之间的认证"/>
                                <label className="field-label">MCP 端点</label><div className="copy-field"><Icon name="link" size={16}/><code>{snapshot.service.url}</code><button className="icon-btn" aria-label="复制 MCP 地址" onClick={() => copyText(snapshot.service.url, 'MCP 地址已复制')} disabled={Boolean(busy)}><Icon name="copy" size={16}/></button></div>
                                <div className="token-row"><div><label className="field-label">连接令牌</label><span>{snapshot.tokenConfigured ? '已安全配置 · 仅保存 SHA-256 哈希' : '尚未生成连接令牌'}</span></div><button className="btn secondary" onClick={generateToken} disabled={Boolean(busy)}><Icon name="key" size={15}/>{snapshot.tokenConfigured ? '重新生成' : '生成令牌'}</button></div>
                                <div className="credential-foot"><Icon name="shield" size={14}/>明文令牌只在生成时显示一次，不会写入桌面配置或 Gemini 页面。</div>
                            </section>
                        </div>
                        <aside className="connection-side"><section className="panel setup-panel"><PanelHeader icon="browser" title="启动协作" subtitle="保持顺序，避免旧脚本继续生效"/><div className="setup-steps"><SetupStep n="1" title="保持桌面端运行" done={serviceRunning}/><SetupStep n="2" title="重新加载扩展并刷新 Gemini" done={connected}/><SetupStep n="3" title="新建会话并等待自动注入"/><SetupStep n="4" title="直接在本地对话发送任务"/></div><div className="protocol-card"><span><Icon name="zap" size={17}/></span><div><strong>新建会话自动注入</strong><p>本地创建会话后，浏览器适配器会自动新建网页对话并注入 <code>webegg_project_list</code> 等真实工具协议。</p></div></div></section><section className="panel privacy-panel"><Icon name="shield"/><div><strong>本地边界</strong><p>服务只监听 127.0.0.1。插件的 Service Worker 持有令牌，Gemini 内容脚本不会读取它。</p></div></section></aside>
                    </div>}
                </div>
            </main>
        </div>

        {navOpen && <div className="nav-sheet-layer" onMouseDown={event => {if (event.target === event.currentTarget) setNavOpen(false);}}>
            <nav className="nav-sheet" aria-label="工作区导航">
                <div className="nav-sheet-head"><Brand/><button onClick={() => setNavOpen(false)} aria-label="关闭导航"><Icon name="close"/></button></div>
                <span className="nav-sheet-label">工作区</span>
                {NAV_ITEMS.map(item => <button key={item.id} className={activeTab === item.id ? 'active' : ''} onClick={() => {setActiveTab(item.id); setNavOpen(false);}}><Icon name={item.icon}/><span>{item.label}</span>{item.id === 'tools' && snapshot.pendingApprovals.length > 0 && <b>{snapshot.pendingApprovals.length}</b>}</button>)}
                <div className="nav-sheet-foot"><span className={`mode-orb ${snapshot.automationMode}`}><Icon name={snapshot.automationMode === 'automatic' ? 'zap' : 'shield'} size={14}/></span><span>{snapshot.automationMode === 'automatic' ? '全自动执行' : '本地批准模式'}</span><ThemeToggle theme={theme} onChange={() => setTheme(theme === 'dark' ? 'light' : 'dark')}/></div>
            </nav>
        </div>}

        {confirmDialog && <div className="confirm-backdrop" onMouseDown={event => {if (event.target === event.currentTarget) setConfirmDialog(null);}}>
            <section className="confirm-modal" role="alertdialog" aria-modal="true" aria-labelledby="confirm-title" aria-describedby="confirm-message">
                <span className={`confirm-emblem ${confirmDialog.danger ? 'danger' : ''}`}><Icon name={confirmDialog.danger ? 'alert' : 'shield'} size={23}/></span>
                <h2 id="confirm-title">{confirmDialog.title}</h2>
                <p id="confirm-message">{confirmDialog.message}</p>
                <div className="confirm-actions"><button className="confirm-cancel" onClick={() => setConfirmDialog(null)}>取消</button><button className={`confirm-submit ${confirmDialog.danger ? 'danger' : ''}`} onClick={() => {const action = confirmDialog.onConfirm; setConfirmDialog(null); action();}}>{confirmDialog.confirmLabel}</button></div>
            </section>
        </div>}

        {newToken && <div className="modal-backdrop" role="presentation" onMouseDown={event => {if (event.target === event.currentTarget) setNewToken('');}}><section className="token-modal" role="dialog" aria-modal="true" aria-labelledby="token-modal-title"><button className="modal-close" aria-label="关闭" onClick={() => setNewToken('')}><Icon name="close"/></button><span className="modal-icon"><Icon name="key" size={24}/></span><span className="eyebrow">ONE-TIME CREDENTIAL</span><h2 id="token-modal-title">复制你的新令牌</h2><p>这是唯一一次完整显示。将它粘贴到 WebEgg 浏览器插件中。</p><div className="modal-token"><code>{newToken}</code></div><button className="btn primary full" onClick={() => copyText(newToken, '连接令牌已复制')}><Icon name="copy" size={16}/>复制连接令牌</button><button className="btn ghost full" onClick={() => setNewToken('')}>我已妥善保存</button></section></div>}
    </div>;

    function copyText(value: string, message: string) { copy(value, message); }
}

function ThemeToggle({theme, onChange}: {theme: Theme; onChange: () => void}) {
    const dark = theme === 'dark';
    return <button className="theme-toggle" onClick={onChange} aria-label={dark ? '切换到日间模式' : '切换到夜间模式'} title={dark ? '切换到日间模式' : '切换到夜间模式'} aria-pressed={dark}>
        <span className="theme-track" aria-hidden="true">
            <i className="theme-star star-one"/><i className="theme-star star-two"/><i className="theme-star star-three"/>
            <span className="theme-thumb">
                <svg className="sun-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"><circle cx="12" cy="12" r="3.5"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M19.1 4.9l-1.4 1.4M6.3 17.7l-1.4 1.4"/></svg>
                <svg className="moon-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M20 15.2A8.5 8.5 0 0 1 8.8 4 8.5 8.5 0 1 0 20 15.2Z"/></svg>
            </span>
        </span>
    </button>;
}

function Metric({icon, label, value, detail, active = false}: {icon: IconName; label: string; value: string; detail: string; active?: boolean}) {
    return <article className="metric-card"><div className={`metric-icon ${active ? 'active' : ''}`}><Icon name={icon}/></div><div className="metric-copy"><span>{label}</span><strong title={value}>{value}</strong><small>{detail}</small></div><i className={active ? 'active' : ''}/></article>;
}
function PanelHeader({icon, title, subtitle, action}: {icon: IconName; title: string; subtitle: string; action?: ReactNode}) {
    return <div className="panel-header"><div className="panel-title-icon"><Icon name={icon} size={17}/></div><div><h2>{title}</h2><p>{subtitle}</p></div>{action && <div className="panel-action">{action}</div>}</div>;
}
function WorkflowStep({number, title, text, done = false}: {number: string; title: string; text: string; done?: boolean}) {
    return <div className={`workflow-step ${done ? 'done' : ''}`}><span>{done ? <Icon name="check" size={14}/> : number}</span><div><strong>{title}</strong><p>{text}</p></div></div>;
}
function SetupStep({n, title, done = false}: {n: string; title: string; done?: boolean}) {
    return <div className={`setup-step ${done ? 'done' : ''}`}><span>{done ? <Icon name="check" size={13}/> : n}</span><p>{title}</p></div>;
}
function EmptyState({title, text, action, onAction, compact = false}: {title: string; text: string; action: string; onAction: () => void; compact?: boolean}) {
    return <div className={`empty-state ${compact ? 'compact' : ''}`}><span className="empty-icon"><Icon name="folder" size={24}/></span><h3>{title}</h3><p>{text}</p><button className="text-action" onClick={onAction}>{action}<Icon name="arrow" size={14}/></button></div>;
}
function formatTime(value: string) { return new Date(value).toLocaleString('zh-CN', {month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit'}); }
function formatDate(value: string) { return new Date(value).toLocaleDateString('zh-CN', {year: 'numeric', month: '2-digit', day: '2-digit'}); }

export default App;
