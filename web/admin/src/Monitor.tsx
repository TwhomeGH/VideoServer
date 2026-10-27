import { useEffect, useState } from 'react';
import { api } from './api';
import { useI18n } from './i18n';
import type { ImportJob } from './ImportJobs';
import { JobProgress } from './JobProgress';

type MonitorData = {
  service: string; startedAt: string; sampledAt: string; uptimeSeconds: number; cpuPercent: number | null; cpuError?: string;
  logicalCPUs: number; goMaxProcs: number; heapAllocBytes: number; goReservedBytes: number; goroutines: number;
  requests: number; inFlight: number; errors5xx: number; receivedBytes: number; sentBytes: number;
  receiveBytesPerSecond: number; sendBytesPerSecond: number; requestsPerSecond: number; tasksError?: string;
  tasks: { jobs: { counts: Record<string, number>; active: ImportJob[] }; scan: {
    automatic: boolean; running: boolean; queued: boolean; intervalSeconds: number; idleMaxSeconds: number;
    nextRunAt?: string; nextRunInSeconds?: number;
    pools?: { poolId: string; name?: string; backoffSeconds: number; quiet: number; nextRunAt?: string; nextRunInSeconds: number }[];
    last: { completedAt: string; enqueued: number; failed: number; pending: number; changed: number; deleted: number; offlinePools: number } | null;
  }; backfill: {
    enabled: boolean; intervalSeconds: number; nextCheckAt?: string; nextCheckInSeconds: number;
    candidateCount: number; nextVideoId?: string; nextVideoTitle?: string;
    lastScheduledAt?: string; lastVideoId?: string;
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

/** Monitor 依工作活動每半秒或兩秒更新，背景分頁降頻；離開頁面時取消請求與輪詢。 */
export function Monitor() {
  const { t, locale } = useI18n();
  const [data, setData] = useState<MonitorData | null>(null);
  const [history, setHistory] = useState<MonitorData[]>([]);
  const [error, setError] = useState('');
  const [receivedAt, setReceivedAt] = useState<number | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      let delay = 2000;
      try {
        const result = await api<MonitorData>('monitor', { signal: controller.signal });
        if (controller.signal.aborted) return;
        setData(result); setError('');
        if (result.tasks?.jobs.counts.running || result.tasks?.jobs.counts.queued) delay = 500;
        setReceivedAt(Date.now());
        if (result.sampledAt && !result.sampledAt.startsWith('0001')) {
          setHistory(current => current.at(-1)?.sampledAt === result.sampledAt ? current : [...current, result].slice(-60));
        }
      } catch (reason) {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));
      }
      if (!controller.signal.aborted) timer = setTimeout(poll, document.hidden ? 5000 : delay);
    };
    void poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, []);
  const phaseLabel = (phase: string) => {
    const key = `jobPhase.${phase}`;
    const translated = t(key);
    return translated === key ? phase : translated;
  };
  const received = receivedAt ? new Date(receivedAt).toLocaleTimeString(locale) : '—';
  return <>
    <section><h2>{t('monitor.currentService', { service: data?.service || t('monitor.connecting') })}</h2>
      <p className="muted">{t('monitor.intro')}</p>
      {error && <p role="alert">{t('monitor.updateFailed', { error })}</p>}
      {data && <p>{t('monitor.uptime', {
        minutes: Math.floor(data.uptimeSeconds / 60),
        seconds: Math.floor(data.uptimeSeconds % 60),
        cpus: data.logicalCPUs,
        maxProcs: data.goMaxProcs,
        received,
      })}</p>}
    </section>
    {!data && !error && <p role="status">{t('monitor.loading')}</p>}
    {data && <>
      <div className="monitor-grid">
        <section><h3>{t('monitor.cpuTitle')}</h3><p className="metric-value">{data.cpuPercent === null ? data.cpuError ? t('monitor.cpuUnavailable') : t('monitor.waitingSample') : `${data.cpuPercent.toFixed(1)}%`}</p><p className="muted">{t('monitor.cpuNote')}</p>{data.cpuError && <p role="alert">{t('monitor.cpuError', { error: data.cpuError })}</p>}<Trend values={history.filter(item => item.cpuPercent !== null).map(item => item.cpuPercent!)} label={t('monitor.cpuTrend')} /></section>
        <section><h3>{t('monitor.memoryTitle')}</h3><p className="metric-value">{bytes(data.heapAllocBytes)}</p><p>{t('monitor.goReserved', { bytes: bytes(data.goReservedBytes) })}</p><p>{t('monitor.goroutines', { count: data.goroutines })}</p><Trend values={history.map(item => item.heapAllocBytes)} label={t('monitor.heapTrend')} /></section>
        <section><h3>{t('monitor.sendTitle')}</h3><p className="metric-value">{bytes(data.sendBytesPerSecond)}/s</p><p>{t('monitor.totalBytes', { bytes: bytes(data.sentBytes) })}</p><Trend values={history.map(item => item.sendBytesPerSecond)} label={t('monitor.sendTrend')} /></section>
        <section><h3>{t('monitor.receiveTitle')}</h3><p className="metric-value">{bytes(data.receiveBytesPerSecond)}/s</p><p>{t('monitor.totalBytes', { bytes: bytes(data.receivedBytes) })}</p><Trend values={history.map(item => item.receiveBytesPerSecond)} label={t('monitor.receiveTrend')} /></section>
      </div>
      <section><h2>{t('monitor.httpTitle')}</h2><p>{t('monitor.httpSummary', {
        inFlight: data.inFlight,
        rps: data.requestsPerSecond.toFixed(1),
        requests: data.requests,
        errors: data.errors5xx,
      })}</p><p className="muted">{t('monitor.httpNote')}</p></section>
      {data.tasksError && <section><p role="alert">{data.tasksError}</p></section>}
      {data.tasks ? <>
        <div className="monitor-dashboard">
          <section><div className="section-heading"><h2>{t('monitor.jobsTitle')}</h2><a href="#jobs">{t('monitor.viewJobs')}</a></div>
            <p>{t('monitor.jobCounts', {
              queued: data.tasks.jobs.counts.queued,
              running: data.tasks.jobs.counts.running,
              failed: data.tasks.jobs.counts.failed,
              completed: data.tasks.jobs.counts.completed,
            })}</p>
            {data.tasks.jobs.active.length === 0 && <p className="muted">{t('monitor.noActiveJobs')}</p>}
            <ul className="monitor-jobs">{data.tasks.jobs.active.map(job => <li key={job.id}><strong>{job.title}</strong><span>{job.source === 'reprobe' ? t('monitor.reprobe') : t('monitor.import')}{t('common.dot')}{phaseLabel(job.phase)}</span><JobProgress job={job} /></li>)}</ul>
            <p className="muted">{t('monitor.queueNote')}</p>
          </section>
          <div className="monitor-side">
            <section><div className="section-heading"><h2>{t('monitor.scanTitle')}</h2><a href="#pools">{t('monitor.managePools')}</a></div>
              <p>{data.tasks.scan.running ? t('monitor.scanRunning') : data.tasks.scan.queued ? t('monitor.scanQueued') : t('monitor.scanIdle')}{t('common.dot')}{data.tasks.scan.automatic ? t('monitor.scanEvery', { seconds: data.tasks.scan.intervalSeconds }) : t('monitor.scanDisabled')}</p>
              {data.tasks.scan.automatic && <p className="muted">{t('monitor.scanIdleMax', { seconds: data.tasks.scan.idleMaxSeconds })}</p>}
              {data.tasks.scan.automatic && !data.tasks.scan.running && <p className="muted">{t('monitor.scanNext', { seconds: data.tasks.scan.nextRunInSeconds || 0 })}</p>}
              {data.tasks.scan.last && <p className="muted">{t('monitor.scanLast', {
                time: new Date(data.tasks.scan.last.completedAt).toLocaleString(locale),
                enqueued: data.tasks.scan.last.enqueued,
                pending: data.tasks.scan.last.pending,
                changed: data.tasks.scan.last.changed || 0,
                deleted: data.tasks.scan.last.deleted || 0,
                failed: data.tasks.scan.last.failed,
                offline: data.tasks.scan.last.offlinePools,
              })}</p>}
              <ul className="monitor-scans">
                {data.tasks.scan.pools?.length
                  ? data.tasks.scan.pools.map(item => <li key={item.poolId}>
                    <strong>{item.name || item.poolId}</strong>
                    <span className="muted">{t('monitor.scanPoolNext', { seconds: item.nextRunInSeconds })}{t('common.dot')}{t('monitor.scanPoolBackoff', { seconds: item.backoffSeconds })}</span>
                  </li>)
                  : <li className="muted">{t('monitor.scanNoPools')}</li>}
              </ul>
            </section>
            <section><h2>{t('monitor.backfillTitle')}</h2>
              {data.tasks.backfill.enabled
                ? <>
                  <p>{t('monitor.backfillNext', { seconds: data.tasks.backfill.nextCheckInSeconds || 0 })}{t('common.dot')}{t('monitor.backfillCandidates', { count: data.tasks.backfill.candidateCount })}</p>
                  <p className="muted">{data.tasks.backfill.nextVideoTitle
                    ? t('monitor.backfillNextCandidate', { title: data.tasks.backfill.nextVideoTitle })
                    : t('monitor.backfillNoCandidate')}</p>
                </>
                : <p className="muted">{t('monitor.backfillDisabled')}</p>}
            </section>
          </div>
        </div>
      </> : !data.tasksError && <section><h2>{t('monitor.backgroundTitle')}</h2><p>{t('monitor.backgroundUnavailable')}</p></section>}
    </>}
  </>;
}
