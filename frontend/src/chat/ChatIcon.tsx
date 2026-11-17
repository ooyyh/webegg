import type {ReactNode} from 'react';

export type ChatIconName = 'message' | 'plus' | 'send' | 'archive' | 'restore' | 'tool' | 'check' | 'loader' | 'file' | 'terminal' | 'search' | 'folder' | 'code' | 'chevron' | 'sparkles' | 'alert' | 'trash' | 'grid';

export function ChatIcon({name, size = 17}: {name: ChatIconName; size?: number}) {
    const paths: Record<ChatIconName, ReactNode> = {
        message: <path d="M20 15a3 3 0 0 1-3 3H9l-5 3v-6a3 3 0 0 1-1-2V6a3 3 0 0 1 3-3h11a3 3 0 0 1 3 3Z"/>,
        plus: <path d="M12 5v14M5 12h14"/>,
        send: <path d="m4 4 17 8-17 8 3-8ZM7 12h14"/>,
        archive: <><rect x="3" y="4" width="18" height="5" rx="1"/><path d="M5 9v11h14V9M9 13h6"/></>,
        restore: <><path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5M12 7v5l3 2"/></>,
        tool: <path d="m14.7 6.3 3-3a4 4 0 0 1-5 5l-6.4 6.4a2.1 2.1 0 0 0 3 3l6.4-6.4a4 4 0 0 1 5-5l-3 3Z"/>,
        check: <path d="m5 12 4 4L19 6"/>,
        loader: <><path d="M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M18.4 5.6l-2.1 2.1M7.7 16.3l-2.1 2.1"/></>,
        file: <><path d="M6 2h8l4 4v16H6Z"/><path d="M14 2v5h5M9 12h6M9 16h6"/></>,
        terminal: <><rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3M13 15h4"/></>,
        search: <><circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/></>,
        folder: <path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>,
        code: <><path d="m8 9-4 3 4 3M16 9l4 3-4 3M14 5l-4 14"/></>,
        chevron: <path d="m6 9 6 6 6-6"/>,
        sparkles: <><path d="m12 3 1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5ZM19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8Z"/></>,
        alert: <><path d="M12 3 2.8 19h18.4ZM12 9v4M12 17h.01"/></>,
        trash: <><path d="M4 7h16M9 7V4h6v3M7 7l1 13h8l1-13M10 11v5M14 11v5"/></>,
        grid: <><rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/></>,
    };
    return <svg className="chat-icon" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{paths[name]}</svg>;
}
