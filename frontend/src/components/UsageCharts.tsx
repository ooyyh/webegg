import {chat} from '../../wailsjs/go/models';

export function UsageCharts({usage}: {usage: chat.Usage}) {
    const days = usage.days?.length ? usage.days : Array.from({length: 7}, (_, index) => chat.DayUsage.createFrom({date: dateOffset(index - 6), messages: 0, toolCalls: 0}));
    const maxValue = Math.max(1, ...days.map(day => Math.max(day.messages, day.toolCalls)));
    const categories = usage.categories?.length ? usage.categories : [];
    const maxCategory = Math.max(1, ...categories.map(item => item.count));
    const width = 620;
    const height = 170;
    const chartBottom = 136;
    const chartTop = 18;
    const slot = width / days.length;
    const barWidth = Math.min(18, slot * 0.24);

    return <section className="usage-section">
        <div className="usage-summary"><div><span className="eyebrow">LOCAL ANALYTICS</span><h2>工作区数据</h2><p>来自本机会话、消息与真实工具调用历史</p></div><div className="usage-kpis"><div><span>会话</span><strong>{usage.conversations}</strong></div><div><span>消息</span><strong>{usage.messages}</strong></div><div><span>工具调用</span><strong>{usage.toolCalls}</strong></div><div><span>归档</span><strong>{usage.archivedCount}</strong></div></div></div>
        <div className="usage-grid">
            <article className="usage-chart-card">
                <div className="usage-chart-head"><div><h3>最近 7 天活动</h3><p>消息与工具调用趋势</p></div><div className="chart-legend"><span className="messages">消息</span><span className="tools">工具调用</span></div></div>
                <svg className="activity-chart" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="最近七天消息和工具调用柱状图">
                    {[0, 0.25, 0.5, 0.75, 1].map(ratio => <line key={ratio} x1="0" x2={width} y1={chartTop + (chartBottom - chartTop) * ratio} y2={chartTop + (chartBottom - chartTop) * ratio} className="chart-gridline"/>)}
                    {days.map((day, index) => {
                        const center = slot * index + slot / 2;
                        const messageHeight = (day.messages / maxValue) * (chartBottom - chartTop);
                        const toolHeight = (day.toolCalls / maxValue) * (chartBottom - chartTop);
                        return <g key={day.date}>
                            <rect className="bar-message" x={center - barWidth - 2} y={chartBottom - messageHeight} width={barWidth} height={messageHeight} rx="3"/>
                            <rect className="bar-tool" x={center + 2} y={chartBottom - toolHeight} width={barWidth} height={toolHeight} rx="3"/>
                            <text className="chart-label" x={center} y={chartBottom + 19} textAnchor="middle">{formatDay(day.date)}</text>
                            {(day.messages > 0 || day.toolCalls > 0) && <text className="chart-value" x={center} y={Math.min(chartBottom - Math.max(messageHeight, toolHeight) - 6, chartBottom - 6)} textAnchor="middle">{Math.max(day.messages, day.toolCalls)}</text>}
                        </g>;
                    })}
                </svg>
            </article>
            <article className="usage-chart-card distribution-card">
                <div className="usage-chart-head"><div><h3>工具能力分布</h3><p>已注册 {usage.tools?.length || 0} 种被调用工具</p></div></div>
                {categories.length ? <div className="distribution-list">{categories.map((item, index) => <div className="distribution-row" key={item.name}><div><span>{item.name}</span><b>{item.count}</b></div><div className="distribution-track"><i style={{width: `${Math.max(6, (item.count / maxCategory) * 100)}%`, opacity: 1 - index * 0.12}}/></div></div>)}</div> : <div className="chart-empty"><span/><strong>还没有工具调用数据</strong><p>开始一次项目任务后，这里会显示真实能力分布。</p></div>}
                <div className="category-legend">{categories.slice(0, 6).map(item => <span key={item.name}>{item.name} <b>{item.count}</b></span>)}</div>
            </article>
        </div>
    </section>;
}

function formatDay(date: string) {
    const value = new Date(`${date}T00:00:00`);
    return `${value.getMonth() + 1}/${value.getDate()}`;
}
function dateOffset(offset: number) {
    const date = new Date();
    date.setDate(date.getDate() + offset);
    return date.toISOString().slice(0, 10);
}
