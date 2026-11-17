import {useState} from 'react';
import {chat} from '../../wailsjs/go/models';
import {ChatIcon, ChatIconName} from './ChatIcon';

export function ToolCard({message, renderMode}: {message: chat.Message; renderMode: boolean}) {
    const [expanded, setExpanded] = useState(false);
    const [tab, setTab] = useState<'result' | 'arguments'>('result');
    const running = message.status === 'running';
    const failed = !running && message.result?.ok === false;
    const tool = message.tool || '';
    const summary = toolSummary(tool, message.arguments || {});
    const icon = toolIcon(tool);

    return <article className={`local-tool-card ${running ? 'running' : failed ? 'failed' : 'complete'}`}>
        <button className="local-tool-head" onClick={() => setExpanded(!expanded)} aria-expanded={expanded}>
            <span className="local-tool-icon">{running ? <span className="tool-spinner"/> : <ChatIcon name={icon} size={15}/>}</span>
            <span className="local-tool-name">{tool.replace('webegg_', '')}</span>
            <span className="local-tool-summary" title={summary}>{summary}</span>
            <span className="local-tool-status">{running ? '执行中' : failed ? '失败' : '已完成'}</span>
            <ChatIcon name="chevron" size={13}/>
        </button>
        {expanded && <div className="local-tool-body">
            <div className="tool-tabs" role="tablist">
                <button className={tab === 'result' ? 'active' : ''} onClick={() => setTab('result')} role="tab" aria-selected={tab === 'result'}>执行结果</button>
                <button className={tab === 'arguments' ? 'active' : ''} onClick={() => setTab('arguments')} role="tab" aria-selected={tab === 'arguments'}>调用参数</button>
                <span>WEBEGG · LOCAL MCP</span>
            </div>
            {tab === 'arguments' ? <pre className="tool-json">{formatJSON(message.arguments)}</pre> : running ? <div className="tool-waiting"><span className="tool-spinner"/>正在等待本地执行结果…</div> : <ToolResult value={message.result} renderMode={renderMode}/>}
        </div>}
    </article>;
}

function ToolResult({value, renderMode}: {value: any; renderMode: boolean}) {
    if (!renderMode) return <pre className="tool-json">{formatJSON(value)}</pre>;
    const data = value?.result ?? value;
    if (value?.ok === false) return <div className="tool-error"><ChatIcon name="alert" size={16}/><div><strong>{value?.error?.code || 'TOOL_ERROR'}</strong><p>{value?.error?.message || '工具执行失败'}</p></div></div>;
    if (data?.content && typeof data.content === 'string') return <div className="file-result"><div className="result-meta"><span>{data.path || '文件内容'}</span>{data.totalLines !== undefined && <span>{data.totalLines} 行</span>}{data.hash && <span>SHA {String(data.hash).slice(0, 10)}</span>}</div><pre className="file-content">{data.content}</pre></div>;
    if (Array.isArray(data?.entries)) return <div className="directory-result"><div className="result-meta"><span>{data.path || '项目根目录'}</span><span>{data.entries.length} 项</span></div><div className="directory-table">{data.entries.map((entry: any, index: number) => <div className="directory-row" key={`${entry.path}-${index}`}><ChatIcon name={entry.type === 'directory' ? 'folder' : 'file'} size={14}/><span>{entry.name}</span><small>{entry.type === 'directory' ? '目录' : formatBytes(entry.size)}</small></div>)}</div></div>;
    if (Array.isArray(data?.projects)) return <div className="project-result">{data.projects.map((project: any) => <div key={project.id}><ChatIcon name="folder" size={15}/><span><strong>{project.name}</strong><code>{project.id}</code></span></div>)}</div>;
    if (data?.process) return <div className="process-result"><div className="result-meta"><span>退出码 {data.process.exitCode}</span><span>{data.process.durationMs} ms</span>{data.process.timedOut && <span>已超时</span>}</div>{data.process.stdout && <pre className="terminal-output">{data.process.stdout}</pre>}{data.process.stderr && <pre className="terminal-output stderr">{data.process.stderr}</pre>}{!data.process.stdout && !data.process.stderr && <div className="muted-result">命令完成，没有标准输出。</div>}</div>;
    if (typeof data?.diff === 'string') return <pre className="diff-output">{data.diff || '没有差异'}</pre>;
    if (typeof data?.status === 'string') return <pre className="terminal-output">{data.status}</pre>;
    if (Array.isArray(data?.matches)) return <div className="matches-result">{data.matches.map((match: any, index: number) => typeof match === 'string' ? <code key={index}>{match}</code> : <div className="match-row" key={index}><span>{match.path}:{match.line}</span><code>{match.text}</code></div>)}</div>;
    if (Array.isArray(data?.commits)) return <div className="commit-result">{data.commits.map((commit: any) => <div key={commit.hash}><code>{commit.shortHash}</code><span>{commit.subject}</span><small>{commit.author}</small></div>)}</div>;
    return <pre className="tool-json">{formatJSON(data)}</pre>;
}

function toolSummary(tool: string, args: Record<string, any>): string {
    if (tool === 'webegg_project_list') return '授权项目';
    if (tool === 'webegg_directory_list') return args.path || '项目根目录';
    if (tool === 'webegg_glob' || tool === 'webegg_grep') return args.pattern || '';
    if (tool === 'webegg_shell_run') return args.command || '';
    if (tool === 'webegg_project_script') return args.script || '';
    if (tool === 'webegg_git_status') return '工作区状态';
    if (tool === 'webegg_git_diff') return args.path || (args.staged ? '暂存区差异' : '工作区差异');
    if (tool === 'webegg_git_log') return `${args.limit || 20} 条提交`;
    return args.path || '本地项目';
}
function toolIcon(tool: string): ChatIconName {
    if (tool.includes('shell') || tool.includes('script')) return 'terminal';
    if (tool.includes('grep') || tool.includes('glob')) return 'search';
    if (tool.includes('directory') || tool.includes('project')) return 'folder';
    if (tool.includes('file')) return 'file';
    return 'tool';
}
function formatJSON(value: any) { return value === undefined ? '无返回内容' : typeof value === 'string' ? value : JSON.stringify(value, null, 2); }
function formatBytes(value: number | undefined) { if (value === undefined) return ''; if (value < 1024) return `${value} B`; if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`; return `${(value / 1024 / 1024).toFixed(1)} MB`; }
