/** ImportJob 是後端持久化匯入工作的管理檢視。 */
export type ImportJob = {
  id: string; title: string; path: string; source: string; status: string; phase: string;
  bytesProcessed: number; totalBytes: number; error: string; attempts: number; videoId: string;
};

/** ImportJobs 顯示背景階段與雜湊進度；onRetry 重新排程指定失敗工作。 */
export function ImportJobs({ jobs, busy, onRetry }: { jobs: ImportJob[]; busy: boolean; onRetry: (id: string) => void }) {
  const labels: Record<string, string> = { queued: '等待處理', probing: '檢查影片資訊', hashing: '計算 SHA-256', completed: '已完成', failed: '失敗' };
  return <section><h2>背景匯入工作</h2><p className="muted">顯示最近 100 筆工作；重新啟動後會接續未完成工作。一次處理一部影片，避免磁碟同時大量讀取。</p>
    {jobs.length === 0 && <p>目前沒有匯入工作。<a href="#import">匯入影片</a></p>}
    <div className="job-list">{jobs.map(job => <article className="job-card" key={job.id}>
      <div className="section-heading"><h3>{job.title}</h3><span>{labels[job.phase] || job.status}</span></div>
      <p className="muted">{job.source === 'scan' ? '自動掃描' : '手動匯入'} · {job.path} · 嘗試 {job.attempts} 次</p>
      {(job.status === 'running' || job.status === 'completed') && <>
        <progress max={Math.max(1, job.totalBytes)} value={job.phase === 'hashing' || job.phase === 'completed' ? job.bytesProcessed : undefined} />
        {job.phase === 'hashing' && <small>{(job.bytesProcessed / 1048576).toFixed(1)} / {(job.totalBytes / 1048576).toFixed(1)} MB</small>}
      </>}
      {job.error && <p role="alert">{job.error}</p>}
      {job.status === 'failed' && <button disabled={busy} onClick={() => onRetry(job.id)}>重試匯入</button>}
      {job.status === 'completed' && <a href="#videos">到影片庫查看</a>}
    </article>)}</div>
  </section>;
}
