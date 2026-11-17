import {useCallback, useEffect, useRef, useState} from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {ArchiveConversation, ClearConversations, CreateConversation, GetConversation, ListConversations, RestoreConversation, SendChatMessage, TidyConversations} from '../../wailsjs/go/main/App';
import {chat, config} from '../../wailsjs/go/models';
import {ChatIcon} from './ChatIcon';
import {ChatDropdown} from './ChatDropdown';
import {ToolCard} from './ToolCard';
import './Chat.css';

type Modal = 'delete' | 'archived' | 'clear' | null;

export function ChatWorkspace({projects, connected, providerReady, providerLastSeen, toolCount, onAddProject, theme, onThemeChange, onOpenMenu}: {projects: config.Project[]; connected: boolean; providerReady: boolean; providerLastSeen?: string; toolCount: number; onAddProject: () => void; theme: 'dark' | 'light'; onThemeChange: () => void; onOpenMenu: () => void}) {
    const [projectID, setProjectID] = useState(projects[0]?.id || '');
    const [conversationList, setConversationList] = useState<chat.List>(chat.List.createFrom({active: [], archived: []}));
    const [activeID, setActiveID] = useState('');
    const [active, setActive] = useState<chat.Conversation | null>(null);
    const [input, setInput] = useState('');
    const [error, setError] = useState('');
    const [sending, setSending] = useState(false);
    const [loading, setLoading] = useState(false);
    const [renderMode, setRenderMode] = useState(true);
    const [modal, setModal] = useState<Modal>(null);
    const inputRef = useRef<HTMLTextAreaElement>(null);
    const endRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (projectID && projects.some(project => project.id === projectID)) return;
        setProjectID(projects[0]?.id || '');
    }, [projects, projectID]);

    const refreshList = useCallback(async () => {
        if (!projectID) return chat.List.createFrom({active: [], archived: []});
        const list = await ListConversations(projectID);
        setConversationList(list);
        return list;
    }, [projectID]);

    useEffect(() => {
        let cancelled = false;
        async function initialize() {
            if (!projectID) { setConversationList(chat.List.createFrom({active: [], archived: []})); setActive(null); return; }
            setLoading(true); setError(''); setActive(null); setActiveID('');
            try {
                let list = await ListConversations(projectID);
                let initialID = list.active[0]?.id || '';
                if (!initialID) {
                    const created = await CreateConversation(projectID);
                    initialID = created.id;
                    list = await ListConversations(projectID);
                }
                if (cancelled) return;
                setConversationList(list);
                setActiveID(initialID);
            } catch (reason) { if (!cancelled) setError(String(reason)); } finally { if (!cancelled) setLoading(false); }
        }
        void initialize();
        return () => { cancelled = true; };
    }, [projectID]);

    useEffect(() => {
        if (!activeID) { setActive(null); return; }
        let cancelled = false;
        async function load() {
            try { const conversation = await GetConversation(activeID); if (!cancelled) setActive(conversation); }
            catch (reason) { if (!cancelled) setError(String(reason)); }
        }
        void load();
        const timer = window.setInterval(load, 1000);
        return () => { cancelled = true; window.clearInterval(timer); };
    }, [activeID]);

    useEffect(() => {
        endRef.current?.scrollIntoView({behavior: 'smooth', block: 'end'});
    }, [active?.messages.length, active?.messages.at(-1)?.content]);

    async function createConversation() {
        if (!projectID) return;
        setError('');
        try {
            const created = await CreateConversation(projectID);
            await refreshList();
            setActiveID(created.id);
            inputRef.current?.focus();
        } catch (reason) { setError(String(reason)); }
    }

    async function archiveActive() {
        if (!active) return;
        try {
            await ArchiveConversation(active.id);
            const list = await refreshList();
            setActiveID(list.active[0]?.id || '');
            setActive(null);
            setModal(null);
        } catch (reason) { setError(String(reason)); }
    }

    async function restoreConversation(id: string) {
        try {
            const restored = await RestoreConversation(id);
            await refreshList();
            setActiveID(restored.id);
            setModal(null);
        } catch (reason) { setError(String(reason)); }
    }

    async function tidyEmpty() {
        if (!projectID) return;
        try {
            const removed = await TidyConversations(projectID);
            const list = await refreshList();
            if (!list.active.some(item => item.id === activeID)) setActiveID(list.active[0]?.id || '');
            if (removed > 0) setError(`已清理 ${removed} 个空会话`);
        } catch (reason) { setError(String(reason)); }
    }

    async function clearAll() {
        try {
            const removed = await ClearConversations(projectID);
            setConversationList(chat.List.createFrom({active: [], archived: []}));
            setActive(null); setActiveID(''); setModal(null);
            setError(`已清空 ${removed} 条本项目会话记录`);
        } catch (reason) { setError(String(reason)); }
    }

    async function send() {
        const content = input.trim();
        if (!content || !active || sending || !connected) return;
        if (!providerReady) { setError('浏览器插件还没有领取任务：请刷新或打开 Gemini 标签页并确认插件已启用。'); return; }
        setSending(true); setError('');
        try {
            await SendChatMessage(active.id, content);
            setInput('');
            const updated = await GetConversation(active.id);
            setActive(updated);
            await refreshList();
        } catch (reason) { setError(String(reason)); } finally { setSending(false); inputRef.current?.focus(); }
    }

    if (!projects.length) return <section className="chat-empty-projects"><span><ChatIcon name="folder" size={26}/></span><h2>先授权一个项目文件夹</h2><p>每个项目都有独立的会话、工具记录和本地历史。</p><button className="chat-primary-button" onClick={onAddProject}><ChatIcon name="plus" size={15}/>添加项目</button></section>;

    const onlyInitialization = active?.messages.length === 1 && active.messages[0].role === 'assistant' && !active.messages[0].content && (active.messages[0].status === 'queued' || active.messages[0].status === 'running');
    return <div className="chat-workspace">
        <section className="local-chat-main">
            <header className="local-chat-header">
                <div className="chat-switchers">
                    <button className="chat-menu-trigger" onClick={onOpenMenu} title="打开工作区导航" aria-label="打开工作区导航"><ChatIcon name="grid" size={15}/><span>WebEgg</span></button>
                    <span className="switcher-divider"/>
                    <ChatDropdown icon="folder" label="项目" variant="project" value={projectID} items={projects.map(project => ({id: project.id, label: project.name, detail: project.path}))} onSelect={setProjectID} emptyLabel="选择项目"/>
                    <span className="switcher-divider"/>
                    <ChatDropdown icon="message" label="会话" variant="conversation" value={activeID} items={conversationList.active.map(summary => ({id: summary.id, label: summary.title, detail: relativeTime(summary.updatedAt)}))} onSelect={setActiveID} emptyLabel="没有会话"/>
                    <button className="new-conversation-icon" onClick={createConversation} title="新建会话并自动连接浏览器" aria-label="新建会话"><ChatIcon name="plus" size={15}/></button>
                    {conversationList.active.some(summary => !summary.title.startsWith('新会话')) && <button className="tidy-button" onClick={tidyEmpty}>清理空会话</button>}
                </div>
                <div className="chat-header-actions">
                    <span className={`provider-chip ${providerReady ? 'ready' : ''}`}><i/>{providerReady ? '适配器在线' : providerLastSeen ? `适配器 ${relativeTime(providerLastSeen)}` : '等待插件'}</span>
                    <button className={`render-toggle ${renderMode ? 'active' : ''}`} onClick={() => setRenderMode(!renderMode)} title="切换结构化渲染与原始 JSON"><ChatIcon name="code" size={14}/>{renderMode ? '渲染' : '原文'}</button>
                    <ThemeButton theme={theme} onChange={onThemeChange}/>
                    <button className="header-icon-button" onClick={() => setModal('archived')} title="已删除记录（可恢复）"><ChatIcon name="archive" size={15}/>{conversationList.archived.length > 0 && <b>{conversationList.archived.length}</b>}</button>
                    {active && <button className="header-icon-button danger" onClick={() => setModal('delete')} title="删除会话（可恢复）"><ChatIcon name="trash" size={15}/></button>}
                </div>
            </header>

            {!providerReady && <div className="provider-help"><ChatIcon name="alert" size={14}/><span>浏览器适配器没有心跳。请打开并刷新 Gemini 页面，确认 <strong>WebEgg Monochrome 0.9.6</strong> 已在扩展管理页启用。</span></div>}

            <div className="local-message-scroll">
                <div className="local-message-list">
                    {!active ? <div className="chat-welcome"><span><ChatIcon name="message" size={26}/></span><h2>开始一个项目会话</h2><p>点击右上角 + 新建会话，插件会自动建立网页对话并注入工具协议。</p><button className="chat-primary-button" onClick={createConversation}><ChatIcon name="plus" size={15}/>新建会话</button></div> : active.messages.length === 0 ? <div className="chat-welcome"><span><ChatIcon name="sparkles" size={27}/></span><h2>这个项目，从哪开始？</h2><p>Gemini 会通过 WebEgg 使用真实项目工具，结果直接回到这里。</p><div className="prompt-suggestions"><button onClick={() => setInput('先浏览项目结构，告诉我这个项目是做什么的。')}>分析项目结构<ChatIcon name="chevron" size={13}/></button><button onClick={() => setInput('查找项目的测试入口并运行测试，汇总结果。')}>运行项目测试<ChatIcon name="chevron" size={13}/></button><button onClick={() => setInput('检查当前 Git 状态和未提交差异，给出代码审阅建议。')}>审阅当前改动<ChatIcon name="chevron" size={13}/></button></div></div> : onlyInitialization ? <ChatBootState providerReady={providerReady} toolCount={toolCount}/> : active.messages.map(message => <MessageView key={message.id} message={message} renderMode={renderMode}/>)}
                    <div ref={endRef}/>
                </div>
            </div>

            <div className="chat-composer-wrap">
                {error && <div className="chat-error" role="alert"><ChatIcon name="alert" size={14}/><span>{error}</span><button onClick={() => setError('')} aria-label="关闭提示">×</button></div>}
                <div className={`chat-composer ${!connected ? 'disconnected' : ''}`}>
                    <textarea ref={inputRef} value={input} onChange={event => setInput(event.target.value)} onKeyDown={event => {if (event.key === 'Enter' && !event.shiftKey) {event.preventDefault(); void send();}}} placeholder={providerReady ? '给这个项目安排一个任务…' : '等待浏览器插件领取任务…'} rows={3} disabled={!active}/>
                    <div className="composer-bottom"><div className="composer-context"><span><ChatIcon name="folder" size={12}/>{projects.find(project => project.id === projectID)?.name}</span><span>{toolCount} 项工具</span><span>会话保存在本机</span></div><div className="composer-send"><span>Enter 发送 · Shift + Enter 换行</span><button onClick={send} disabled={!input.trim() || !active || sending || !connected} aria-label="发送消息">{sending ? <span className="tool-spinner"/> : <ChatIcon name="send" size={16}/>}</button></div></div>
                </div>
            </div>
        </section>

        {modal && <div className="chat-modal-backdrop" onMouseDown={event => {if (event.target === event.currentTarget) setModal(null);}}>
            <section className="chat-modal" role="dialog" aria-modal="true">
                <button className="chat-modal-close" onClick={() => setModal(null)} aria-label="关闭">×</button>
                {modal === 'delete' && <>
                    <span className="modal-emblem"><ChatIcon name="trash" size={22}/></span><h2>删除这个会话？</h2><p>消息和工具记录会移入「已删除记录」，之后可以完整复活为新会话。</p><div className="modal-actions"><button className="modal-secondary" onClick={() => setModal(null)}>取消</button><button className="modal-danger" onClick={archiveActive}>移入已删除记录</button></div>
                </>}
                {modal === 'archived' && <>
                    <span className="modal-emblem"><ChatIcon name="archive" size={22}/></span><h2>已删除记录</h2><p>归档保留完整消息和工具历史，可以复活为新会话。</p>
                    <div className="archive-modal-list">{conversationList.archived.map(summary => <div className="archive-modal-row" key={summary.id}><div><strong>{summary.title}</strong><small>{relativeTime(summary.updatedAt)}</small></div><button onClick={() => restoreConversation(summary.id)}><ChatIcon name="restore" size={13}/>复活</button></div>)}{!conversationList.archived.length && <p className="archive-none">暂无已删除记录</p>}</div>
                    <div className="modal-actions"><button className="modal-secondary" onClick={() => setModal(null)}>关闭</button>{conversationList.archived.length > 0 && <button className="modal-danger" onClick={() => setModal('clear')}>清空全部记录</button>}</div>
                </>}
                {modal === 'clear' && <>
                    <span className="modal-emblem danger"><ChatIcon name="alert" size={22}/></span><h2>清空本项目全部会话？</h2><p>当前会话、归档记录和工具历史都会被永久删除，无法恢复。项目文件不受影响。</p><div className="modal-actions"><button className="modal-secondary" onClick={() => setModal('archived')}>返回</button><button className="modal-danger" onClick={clearAll}>永久清空</button></div>
                </>}
            </section>
        </div>}
    </div>;
}

function ChatBootState({providerReady, toolCount}: {providerReady: boolean; toolCount: number}) {
    return <div className="chat-boot-state">
        <div className="boot-orb" aria-hidden="true"><span/><ChatIcon name="sparkles" size={25}/></div>
        <span className="boot-kicker">AUTOMATIC SESSION SETUP</span>
        <h2>正在建立网页协作会话</h2>
        <p>WebEgg 正在自动控制浏览器新建 Gemini 对话，并注入本地工具协议。</p>
        <div className="boot-steps">
            <div className={providerReady ? 'done' : 'active'}><span>{providerReady ? <ChatIcon name="check" size={12}/> : <i/>}</span>浏览器适配器{providerReady ? '已连接' : '等待连接'}</div>
            <div className={providerReady ? 'active' : ''}><span>{providerReady ? <i/> : '2'}</span>新建独立网页会话</div>
            <div><span>3</span>注入 {toolCount} 项工具协议</div>
        </div>
    </div>;
}

function ThemeButton({theme, onChange}: {theme: 'dark' | 'light'; onChange: () => void}) {
    const dark = theme === 'dark';
    return <button className="chat-theme-toggle" onClick={onChange} aria-label={dark ? '切换到日间模式' : '切换到夜间模式'} title={dark ? '切换到日间模式' : '切换到夜间模式'} aria-pressed={dark}>
        <span className="theme-track" aria-hidden="true"><i className="theme-star star-one"/><i className="theme-star star-two"/><span className="theme-thumb"><svg className="sun-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"><circle cx="12" cy="12" r="3.5"/><path d="M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M19.1 4.9l-1.4 1.4M6.3 17.7l-1.4 1.4"/></svg><svg className="moon-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M20 15.2A8.5 8.5 0 0 1 8.8 4 8.5 8.5 0 1 0 20 15.2Z"/></svg></span></span>
    </button>;
}

function MessageView({message, renderMode}: {message: chat.Message; renderMode: boolean}) {
    if (message.role === 'tool') return <ToolCard message={message} renderMode={renderMode}/>;
    if (message.role === 'user') return <article className="local-user-message"><div>{message.content}</div><span>你</span></article>;
    const waiting = !message.content && (message.status === 'queued' || message.status === 'running');
    return <article className={`local-assistant-message ${message.status === 'error' ? 'error' : ''}`}>
        <span className="assistant-avatar"><ChatIcon name="sparkles" size={15}/></span>
        <div className="assistant-content"><div className="assistant-meta"><strong>Gemini</strong><span>{message.status === 'queued' ? '排队中' : message.status === 'running' ? '正在回复' : message.status === 'error' ? '失败' : '已完成'}</span></div>
            {waiting ? <div className="assistant-thinking"><i/><i/><i/><span>{message.status === 'queued' ? '等待浏览器插件领取任务' : '正在生成回复'}</span></div> : message.content && (renderMode ? <div className="markdown-body"><ReactMarkdown remarkPlugins={[remarkGfm]}>{message.content}</ReactMarkdown>{message.status === 'running' && <span className="stream-cursor"/>}</div> : <pre className="raw-assistant">{message.content}</pre>)}
        </div>
    </article>;
}

function relativeTime(value: any): string {
    const date = new Date(value);
    const diff = Date.now() - date.getTime();
    if (diff < 60_000) return '刚刚';
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`;
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`;
    return date.toLocaleDateString('zh-CN', {month: '2-digit', day: '2-digit'});
}
