import { useI18n } from './i18n';
import type { ImportJob } from './ImportJobs';

/** JobProgress 平順呈現已收到的實際位元組進度；未知長度的 FFprobe 階段不估算百分比。 */
export function JobProgress({ job }: { job: ImportJob }) {
  const { t } = useI18n();
  if (job.status !== 'running' && job.status !== 'completed') return null;
  const determinate = job.status === 'completed' || (job.phase === 'hashing' && job.totalBytes > 0);
  const percent = job.status === 'completed' ? 100 : Math.max(0, Math.min(100, job.bytesProcessed / Math.max(1, job.totalBytes) * 100));
  const label = determinate ? `${percent.toFixed(1)}%` : t('jobProgress.unknown');
  return <div className="job-progress">
    <div className={`job-progress-track${determinate ? '' : ' is-indeterminate'}${job.status === 'running' ? ' is-active' : ''}`}
      role="progressbar" aria-label={job.source === 'reprobe' ? t('jobProgress.reprobeAria') : t('jobProgress.importAria')}
      aria-valuemin={0} aria-valuemax={100} aria-valuenow={determinate ? percent : undefined} aria-valuetext={label}>
      <span key={determinate ? 'measured' : 'checking'} className="job-progress-fill" style={determinate ? { transform: `scaleX(${percent / 100})` } : undefined} />
    </div>
    {job.phase === 'hashing' && <small>{t('jobProgress.hashing', {
      percent: percent.toFixed(1),
      processed: (job.bytesProcessed / 1048576).toFixed(1),
      total: (job.totalBytes / 1048576).toFixed(1),
    })}</small>}
    {job.phase === 'probing' && <small>{t('jobProgress.probing')}</small>}
  </div>;
}
