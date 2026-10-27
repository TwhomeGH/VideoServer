import { useEffect, useState } from 'react';
import { api } from './api';
import { useI18n } from './i18n';
import { JobProgress } from './JobProgress';

/** ImportJob 是後端持久化匯入工作的管理檢視。 */
export type ImportJob = {
  id: string; title: string; path: string; source: string; status: string; phase: string;
  bytesProcessed: number; totalBytes: number; error: string; attempts: number; videoId: string;
};

type ScheduleData = {
  scan?: { automatic: boolean; running: boolean; queued: boolean; nextRunInSeconds?: number };
  backfill?: { enabled: boolean; nextCheckInSeconds?: number; candidateCount: number };
};

/** ImportJobs 獨立讀取工作狀態；有活動時每半秒更新，不重新載入影片庫或清空畫面。 */
export function ImportJobs() {
  const { t } = useI18n();
  const [jobs, setJobs] = useState<ImportJob[] | null>(null);
  const [schedule, setSchedule] = useState<ScheduleData | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [revision, setRevision] = useState(0);
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
    <div className="job-list">{jobs?.map(job => <article className="job-card" key={job.id}>
      <div className="section-heading"><h3>{job.title}</h3><span>{phaseLabel(job)}</span></div>
      <p className="muted">{t('importJobs.meta', { source: sourceLabel(job.source), path: job.path, attempts: job.attempts })}</p>
      <JobProgress job={job} />
      {job.error && <p role="alert">{job.error}</p>}
      {job.status === 'failed' && <button disabled={busy} onClick={() => void retry(job.id)}>{t('importJobs.retry')}</button>}
      {job.status === 'completed' && <a href="#videos">{t('importJobs.viewLibrary')}</a>}
    </article>)}</div>
  </section>;
}
