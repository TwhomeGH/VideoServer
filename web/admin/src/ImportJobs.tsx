import { useEffect, useState } from 'react';
import { api } from './api';
import { useI18n } from './i18n';
import { JobProgress } from './JobProgress';

/** ImportJob 是匯入與重新讀取工作的狀態。 */
export type ImportJob = {
  id: string; title: string; path: string; source: string; status: string; phase: string;
  bytesProcessed: number; totalBytes: number; error: string; attempts: number; videoId: string;
  createdAt?: string; updatedAt?: string;
};

type ScheduleData = {
  scan?: { automatic: boolean; running: boolean; queued: boolean; nextRunInSeconds?: number };
  backfill?: { enabled: boolean; nextCheckInSeconds?: number; candidateCount: number };
};

type JobFilter = 'all' | 'active' | 'failed' | 'completed';

const PAGE_SIZE = 20;

/** ImportJobs 顯示影片工作；緊湊列、篩選與分頁避免 100 筆時過度捲動。 */
export function ImportJobs() {
  const { t, locale } = useI18n();
  const [jobs, setJobs] = useState<ImportJob[] | null>(null);
  const [schedule, setSchedule] = useState<ScheduleData | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
  const [filter, setFilter] = useState<JobFilter>('all');
  const [page, setPage] = useState(0);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      let delay = 2000;
      try {
        const result = await api<ImportJob[]>('import-jobs', { signal: controller.signal });
        if (controller.signal.aborted) return;
        setJobs(result); setError('');
        const nextSchedule = await api<ScheduleData>('schedule', { signal: controller.signal });
        if (!controller.signal.aborted) setSchedule(nextSchedule);
        if (result.some(job => job.status === 'queued' || job.status === 'running')) delay = 500;
      } catch (reason) {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));
      }
      if (!controller.signal.aborted) timer = setTimeout(poll, document.hidden ? 5000 : delay);
    };
    void poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [revision]);

  async function retry(id: string) {
    setBusy(true); setError('');
    try {
      const result = await api<ImportJob>(`import-jobs/${id}/retry`, { method: 'POST', body: '{}' });
      setJobs(current => current?.map(job => job.id === id ? result : job) || null);
      setRevision(value => value + 1);
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { setBusy(false); }
  }

  const phaseLabel = (job: ImportJob) => {
    const key = `jobPhase.${job.phase}`;
    const translated = t(key);
    return translated === key ? job.status : translated;
  };
  const sourceLabel = (source: string) => source === 'reprobe' ? t('importJobs.sourceReprobe')
    : source === 'scan' ? t('importJobs.sourceScan') : t('importJobs.sourceManual');
  const statusClass = (status: string) => status === 'running' ? 'online' : status === 'failed' ? 'failed' : status === 'queued' ? 'offline' : 'disabled';
  const matchesFilter = (job: ImportJob, value: JobFilter) => value === 'all' ||
    (value === 'active' && (job.status === 'queued' || job.status === 'running')) ||
    (value === 'failed' && job.status === 'failed') ||
    (value === 'completed' && job.status === 'completed');

  const allJobs = jobs ?? [];
  const counts = {
    all: allJobs.length,
    active: allJobs.filter(job => job.status === 'queued' || job.status === 'running').length,
    failed: allJobs.filter(job => job.status === 'failed').length,
    completed: allJobs.filter(job => job.status === 'completed').length,
  };
  const filtered = allJobs.filter(job => matchesFilter(job, filter));
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const currentPage = Math.min(page, pageCount - 1);
  const visible = filtered.slice(currentPage * PAGE_SIZE, (currentPage + 1) * PAGE_SIZE);
  const first = filtered.length === 0 ? 0 : currentPage * PAGE_SIZE + 1;
  const last = Math.min(filtered.length, (currentPage + 1) * PAGE_SIZE);
  const filters: { key: JobFilter; label: string }[] = [
    { key: 'all', label: t('importJobs.filterAll') },
    { key: 'active', label: t('importJobs.filterActive') },
    { key: 'failed', label: t('importJobs.filterFailed') },
    { key: 'completed', label: t('importJobs.filterCompleted') },
  ];
  const toggleDetails = (id: string) => {
    setExpanded(current => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  return <section><h2>{t('importJobs.title')}</h2><p className="muted">{t('importJobs.intro1')}</p>
    <p className="muted">{t('importJobs.intro2')}</p>
    {schedule && <p className="muted">
      {schedule.scan?.automatic ? t('importJobs.nextScan', { seconds: schedule.scan.nextRunInSeconds || 0 }) : t('importJobs.scanDisabled')}
      {t('common.dot')}
      {schedule.backfill?.enabled
        ? t('importJobs.nextBackfill', { seconds: schedule.backfill.nextCheckInSeconds || 0, count: schedule.backfill.candidateCount })
        : t('importJobs.backfillDisabled')}
    </p>}
    {error && <p role="alert">{t('importJobs.updateFailed', { error })}</p>}
    {jobs === null && !error && <p role="status">{t('importJobs.loading')}</p>}
    {jobs?.length === 0 && <p>{t('importJobs.empty')}{' '}<a href="#import">{t('importJobs.import')}</a></p>}
    {jobs && jobs.length > 0 && <>
      <div className="job-toolbar">
        <div className="job-filters" role="group" aria-label={t('importJobs.filterLabel')}>
          {filters.map(item => <button type="button" key={item.key} className={filter === item.key ? 'is-active' : ''}
            onClick={() => { setFilter(item.key); setPage(0); }}>
            {item.label} <span className="job-filter-count">{counts[item.key]}</span>
          </button>)}
        </div>
        <span className="muted">{t('importJobs.showing', { from: first, to: last, total: filtered.length })}</span>
      </div>
      {filtered.length === 0
        ? <p className="muted">{t('importJobs.noMatches')}</p>
        : <>
          <div className="job-table" role="table" aria-label={t('importJobs.title')}>
            <div className="job-table-head" role="row">
              <span>{t('importJobs.jobHeader')}</span>
              <span>{t('importJobs.statusHeader')}</span>
              <span>{t('importJobs.progressHeader')}</span>
              <span>{t('importJobs.attemptsHeader')}</span>
              <span>{t('importJobs.actionsHeader')}</span>
            </div>
            {visible.map(job => {
              const details = expanded.has(job.id);
              return <article className="job-row" key={job.id} role="row">
                <div className="job-main">
                  <div className="job-title"><strong title={job.title}>{job.title}</strong><span className="muted">{sourceLabel(job.source)}</span></div>
                  <span className={`state-badge job-status ${statusClass(job.status)}`}>{phaseLabel(job)}</span>
                  <div className="job-progress-cell"><JobProgress job={job} /></div>
                  <span className="job-attempts muted">{t('importJobs.attemptsShort', { count: job.attempts })}</span>
                  <div className="job-actions">
                    {job.status === 'failed' && <button type="button" disabled={busy} onClick={() => void retry(job.id)}>{t('importJobs.retry')}</button>}
                    {job.status === 'completed' && <a href="#videos">{t('importJobs.viewLibrary')}</a>}
                    <button type="button" onClick={() => toggleDetails(job.id)}>{details ? t('importJobs.hideDetails') : t('importJobs.details')}</button>
                  </div>
                </div>
                {details && <div className="job-detail">
                  <span>{t('importJobs.pathLabel')}: {job.path}</span>
                  {job.updatedAt && <span>{t('importJobs.updated', { time: new Date(job.updatedAt).toLocaleString(locale) })}</span>}
                  {job.error && <span role="alert">{job.error}</span>}
                </div>}
              </article>;
            })}
          </div>
          {pageCount > 1 && <div className="job-pagination">
            <button type="button" disabled={currentPage === 0} onClick={() => setPage(currentPage - 1)}>{t('importJobs.prevPage')}</button>
            <span className="muted">{t('importJobs.pageOf', { page: currentPage + 1, pages: pageCount })}</span>
            <button type="button" disabled={currentPage >= pageCount - 1} onClick={() => setPage(currentPage + 1)}>{t('importJobs.nextPage')}</button>
          </div>}
        </>}
    </>}
  </section>;
}
