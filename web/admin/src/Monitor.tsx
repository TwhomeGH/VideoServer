import { useEffect, useState } from 'react';
import { api } from './api';
import type { ImportJob } from './ImportJobs';

type MonitorData = {
  service: string; startedAt: string; sampledAt: string; uptimeSeconds: number; cpuPercent: number | null; cpuError?: string;
  logicalCPUs: number; goMaxProcs: number; heapAllocBytes: number; goReservedBytes: number; goroutines: number;
  requests: number; inFlight: number; errors5xx: number; receivedBytes: number; sentBytes: number;
  receiveBytesPerSecond: number; sendBytesPerSecond: number; requestsPerSecond: number; tasksError?: string;
  tasks: { jobs: { counts: Record<string, number>; active: ImportJob[] }; scan: {
    automatic: boolean; running: boolean; queued: boolean; intervalSeconds: number;
    last: { completedAt: string; enqueued: number; failed: number; pending: number; offlinePools: number } | null;
  } } | null;
};

function bytes(value: number): string {
  if (value < 1024) return `${value.toFixed(0)} B`;
  if (value < 1048576) return `${(value / 1024).toFixed(1)} KiB`;
  return `${(value / 1048576).toFixed(1)} MiB`;
}

/** Trend 畫出此頁開啟後最近 60 個樣本；不儲存歷史數據。 */
function Trend({ values, label }: { values: number[]; label: string }) {
  const max = Math.max(1, ...values);
  const points = values.map((value, index) => `${index * 240 / Math.max(1, values.length - 1)},${48 - value / max * 44}`).join(' ');
  return <svg className="monitor-trend" viewBox="0 0 240 52" role="img" aria-label={label}>
    <line x1="0" y1="49" x2="240" y2="49" stroke="var(--line)" />
    <polyline points={points} fill="none" stroke="var(--accent)" strokeWidth="2" />
  </svg>;
}

/** Monitor 每兩秒讀取目前服務的工作與資源；離開頁面時取消請求與輪詢。 */
export function Monitor() {
  const [data, setData] = useState<MonitorData | null>(null);
  const [history, setHistory] = useState<MonitorData[]>([]);
  const [error, setError] = useState('');
  const [receivedAt, setReceivedAt] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const result = await api<MonitorData>('monitor', { signal: controller.signal });
        if (controller.signal.aborted) return;
        setData(result); setError('');
        setReceivedAt(new Date().toLocaleTimeString());
        if (result.sampledAt && !result.sampledAt.startsWith('0001')) {
          setHistory(current => current.at(-1)?.sampledAt === result.sampledAt ? current : [...current, result].slice(-60));
        }
      } catch (reason) {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));
      }
      if (!controller.signal.aborted) timer = setTimeout(poll, 2000);
    };
    void poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, []);
  const phase: Record<string, string> = { queued: '等待處理', probing: '讀取影片資訊', hashing: '計算 SHA-256' };
  return <>
    <section><h2>目前服務：{data?.service || '連線中'}</h2>
      <p className="muted">每兩秒更新；顯示目前 DVN 服務程序，Docker 中不代表宿主機總使用率。CPU 不含 FFprobe 子程序，記憶體為 Go runtime，不是完整程序或容器用量。</p>
      {error && <p role="alert">更新失敗：{error}。下方保留最後成功資料，正在重試。</p>}
      {data && <p>已運行 {Math.floor(data.uptimeSeconds / 60)} 分 {Math.floor(data.uptimeSeconds % 60)} 秒 · 邏輯 CPU {data.logicalCPUs} · Go 平行上限 {data.goMaxProcs} · 最近收到資料 {receivedAt}</p>}
    </section>
    {!data && !error && <p role="status">正在取得監看資料…</p>}
    {data && <>
      <div className="monitor-grid">
        <section><h3>服務 CPU</h3><p className="metric-value">{data.cpuPercent === null ? data.cpuError ? '無法取得' : '等待樣本' : `${data.cpuPercent.toFixed(1)}%`}</p><p className="muted">單核心滿載為 100%，多核心可超過 100%。</p>{data.cpuError && <p role="alert">無法取得 CPU 樣本：{data.cpuError}</p>}<Trend values={history.filter(item => item.cpuPercent !== null).map(item => item.cpuPercent!)} label="CPU 使用率趨勢" /></section>
        <section><h3>Go 記憶體</h3><p className="metric-value">{bytes(data.heapAllocBytes)}</p><p>Go 保留：{bytes(data.goReservedBytes)}</p><p>Goroutine：{data.goroutines}</p><Trend values={history.map(item => item.heapAllocBytes)} label="Go 堆積記憶體趨勢" /></section>
        <section><h3>HTTP 傳送</h3><p className="metric-value">{bytes(data.sendBytesPerSecond)}/s</p><p>累計：{bytes(data.sentBytes)}</p><Trend values={history.map(item => item.sendBytesPerSecond)} label="HTTP 傳送速率趨勢" /></section>
        <section><h3>HTTP 接收</h3><p className="metric-value">{bytes(data.receiveBytesPerSecond)}/s</p><p>累計：{bytes(data.receivedBytes)}</p><Trend values={history.map(item => item.receiveBytesPerSecond)} label="HTTP 接收速率趨勢" /></section>
      </div>
      <section><h2>HTTP 工作負載</h2><p>處理中 {data.inFlight} · 每秒完成 {data.requestsPerSecond.toFixed(1)} · 累計完成 {data.requests} · 5xx 回應 {data.errors5xx}</p><p className="muted">只計算此服務實際讀寫的 HTTP 本文，包含播放、健康檢查與監看請求。不是所有網卡流量；計數在服務重啟後歸零。</p></section>
      {data.tasksError && <section><p role="alert">{data.tasksError}</p></section>}
      {data.tasks ? <>
        <section><div className="section-heading"><h2>影片工作</h2><a href="#jobs">查看工作與重試</a></div>
          <p>等待 {data.tasks.jobs.counts.queued} · 執行 {data.tasks.jobs.counts.running} · 失敗 {data.tasks.jobs.counts.failed} · 完成 {data.tasks.jobs.counts.completed}</p>
          {data.tasks.jobs.active.length === 0 && <p className="muted">目前沒有執行或等待中的影片工作。</p>}
          <ul className="monitor-jobs">{data.tasks.jobs.active.map(job => <li key={job.id}><strong>{job.title}</strong><span>{job.source === 'reprobe' ? '重新讀取資訊' : '匯入影片'} · {phase[job.phase] || job.phase}</span>{job.phase === 'hashing' && <><progress max={Math.max(1, job.totalBytes)} value={job.bytesProcessed} /><small>{bytes(job.bytesProcessed)} / {bytes(job.totalBytes)}</small></>}</li>)}</ul>
          <p className="muted">全佇列統計，優先列出執行中及最早等待的 10 筆工作。</p>
        </section>
        <section><h2>儲存池自動掃描</h2><p>{data.tasks.scan.running ? '正在掃描儲存池' : data.tasks.scan.queued ? '已排程，等待掃描' : '目前沒有執行掃描'} · {data.tasks.scan.automatic ? `每 ${data.tasks.scan.intervalSeconds} 秒自動檢查` : '自動掃描停用'}</p>
          {data.tasks.scan.last && <p className="muted">上次完成 {new Date(data.tasks.scan.last.completedAt).toLocaleString()} · 加入工作 {data.tasks.scan.last.enqueued} · 待穩定 {data.tasks.scan.last.pending} · 失敗 {data.tasks.scan.last.failed} · 離線池 {data.tasks.scan.last.offlinePools}</p>}
          <a href="#pools">管理儲存池與掃描</a>
        </section>
      </> : !data.tasksError && <section><h2>背景工作</h2><p>目前服務尚未實作派工或聯邦同步，暫無可顯示的背景工作。</p></section>}
    </>}
  </>;
}
