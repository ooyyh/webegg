import {useEffect, useRef, useState} from 'react';
import {ChatIcon, ChatIconName} from './ChatIcon';

export type ChatDropdownItem = {
    id: string;
    label: string;
    detail?: string;
};

export function ChatDropdown({icon, label, value, items, onSelect, emptyLabel, variant = 'project'}: {
    icon: ChatIconName;
    label: string;
    value: string;
    items: ChatDropdownItem[];
    onSelect: (id: string) => void;
    emptyLabel: string;
    variant?: 'project' | 'conversation';
}) {
    const [open, setOpen] = useState(false);
    const rootRef = useRef<HTMLDivElement>(null);
    const selected = items.find(item => item.id === value);

    useEffect(() => {
        if (!open) return;
        function onPointerDown(event: PointerEvent) {
            if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
        }
        function onKeyDown(event: KeyboardEvent) {
            if (event.key === 'Escape') setOpen(false);
        }
        document.addEventListener('pointerdown', onPointerDown);
        document.addEventListener('keydown', onKeyDown);
        return () => {
            document.removeEventListener('pointerdown', onPointerDown);
            document.removeEventListener('keydown', onKeyDown);
        };
    }, [open]);

    return <div className={`chat-dropdown ${variant} ${open ? 'open' : ''}`} ref={rootRef}>
        <button className="dropdown-trigger" type="button" onClick={() => setOpen(!open)} aria-haspopup="listbox" aria-expanded={open} aria-label={`切换${label}`}>
            <span className="dropdown-icon"><ChatIcon name={icon} size={14}/></span>
            <span className="dropdown-copy"><small>{label}</small><strong>{selected?.label || emptyLabel}</strong></span>
            <ChatIcon name="chevron" size={12}/>
        </button>
        {open && <div className="dropdown-menu" role="listbox" aria-label={label}>
            <div className="dropdown-menu-head"><span>{label}</span><b>{items.length}</b></div>
            <div className="dropdown-items">
                {items.map(item => <button key={item.id} type="button" role="option" aria-selected={item.id === value} className={item.id === value ? 'selected' : ''} onClick={() => {onSelect(item.id); setOpen(false);}}>
                    <span className="dropdown-item-copy"><strong>{item.label}</strong>{item.detail && <small>{item.detail}</small>}</span>
                    {item.id === value && <ChatIcon name="check" size={13}/>}
                </button>)}
                {!items.length && <div className="dropdown-empty">{emptyLabel}</div>}
            </div>
        </div>}
    </div>;
}
