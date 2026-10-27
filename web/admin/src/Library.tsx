import { useEffect, useRef, useState } from 'react';
import { api } from './api';
import { useI18n } from './i18n';
import { formatDuration } from './format';
import { ScanTierBadge } from './ScanTierBadge';
import { Attribution, emptyMetadata, MetadataFields, type VideoMetadata } from './VideoMetadata';
import type { Page } from './navigation';
import { ImportJobs, type ImportJob } from './ImportJobs';
import { VideoFilters, defaultVideoFilters } from './VideoFilters';

type Pool = { id: string; name: string; path: string; online: boolean; enabled: boolean; filesystem?: string; volumeId?: string; volumeStatus?: string };
type Video = VideoMetadata & {
  id: string; title: string; poolId: string; size: number; fileHash: string;
  available: boolean; unavailableReason?: string; importedAt: string;
  description: string; category: string; adult: boolean; license: string;
  media?: { durationSeconds: number; width: number; height: number; videoCodec: string; audioCodec: string; frameRate: number };
};
type PoolScanStatus = {
  poolId: string; name?: string; tier?: string; backoffSeconds: number; quiet: number;
  lastActivityAt?: string; nextRunAt?: string; nextRunInSeconds: number;
};
type ScanStatus = {
  automatic: boolean; intervalSeconds: number; idleMaxSeconds: number; stableSeconds: number; running: boolean; queued: boolean;
  nextRunAt?: string; nextRunInSeconds?: number; pools?: PoolScanStatus[];
  last: { completedAt: string; imported: number; enqueued: number; pending: number; changed: number; deleted: number; failed: number; offlinePools: number; errors: string[] } | null;
};
type LibraryData = { nodeId: string; pools: Pool[]; videos: Video[]; scan: ScanStatus | null; jobs: ImportJob[];
  pagination: { page: number; pageSize: number; total: number; totalVideos: number } };

function mediaSummary(video: Video, t: (key: string, params?: Record<string, string | number>) => string): string {
  if (!video.media) return t('common.noTechnicalInfo');
  const media = video.media;
  const seconds = Math.floor(media.durationSeconds);
  const duration = seconds > 0 ? `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}` : t('common.unknownDuration');
  return `${media.width} × ${media.height} · ${media.videoCodec} / ${media.audioCodec || t('common.noAudio')} · ${duration}`;
}

/** unavailableVideoLabel 將後端原因轉成可理解的中文／英文提示。 */
function unavailableVideoLabel(video: Video, poolName: string | undefined, t: (key: string, params?: Record<string, string | number>) => string): string {
  switch (video.unavailableReason) {
    case 'pool_disabled': return t('library.poolDisabledVideo', { pool: poolName || t('common.notProvided') });
    case 'pool_offline': return t('library.poolOfflineVideo', { pool: poolName || t('common.notProvided') });
    case 'source_changed': return t('library.sourceChanged');
    case 'source_missing': return t('library.sourceMissing');
    default: return t('library.unavailable');
  }
}

async function request<T>(path: string, body?: object, method = 'POST'): Promise<T> {
  return api<T>(path, body ? { method, body: JSON.stringify(body) } : { method: 'GET' });
}

/** Library 依 page 顯示管理分頁，切換時保留尚未送出的表單資料。 */
export function Library({ page, onNodeId }: { page: Page; onNodeId?: (nodeId: string) => void }) {
  const { t, locale } = useI18n();
  const [data, setData] = useState<LibraryData | null>(null);
  const [error, setError] = useState('');
  const [loadError, setLoadError] = useState('');
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState('');
  const [root, setRoot] = useState('');
  const [poolId, setPoolId] = useState('');
  const [path, setPath] = useState('');
  const [draft, setDraft] = useState<VideoMetadata>(emptyMetadata);
  const [editing, setEditing] = useState<Video | null>(null);
  const [editDraft, setEditDraft] = useState<VideoMetadata>(emptyMetadata);
  const [selected, setSelected] = useState<Video | null>(null);
  const [filters, setFilters] = useState(defaultVideoFilters);
  const [videoPage, setVideoPage] = useState(1);
  const refreshVersion = useRef(0);
  const queryString = page === 'monitor' || page === 'api' || page === 'jobs' ? '' : page === 'videos'
    ? new URLSearchParams({ ...filters, page: String(videoPage) }).toString() : 'pageSize=1';
  const playerDialog = useRef<HTMLDialogElement>(null);
  const editDialog = useRef<HTMLDialogElement>(null);

  async function refresh(signal?: AbortSignal) {
    const version = ++refreshVersion.current;
    const result = await api<LibraryData>(`library?${queryString}`, { signal });
    if (signal?.aborted || version !== refreshVersion.current) return;
    result.videos = result.videos.map(video => ({ ...emptyMetadata(), ...video }));
    setData(result);
    onNodeId?.(result.nodeId);
    setLoadError('');
    setSelected(current => current ? result.videos.find(video => video.id === current.id) || current : null);
    setPoolId(current => current || result.pools[0]?.id || '');
  }

  useEffect(() => {
    if (!queryString) return;
    let active = true;
    const controller = new AbortController();
    setData(null);
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (active) {
        try { await refresh(controller.signal); }
        catch (reason) { if (active) setLoadError(reason instanceof Error ? reason.message : String(reason)); }
        if (active) timer = setTimeout(poll, 5000);
      }
    };
    void poll();
    return () => { active = false; controller.abort(); ++refreshVersion.current; clearTimeout(timer); };
  }, [queryString]);

  useEffect(() => {
    if (page !== 'videos') { setSelected(null); setEditing(null); }
  }, [page]);

  useEffect(() => {
    const dialog = playerDialog.current;
    if (!dialog) return;
    if (selected && !dialog.open) dialog.showModal();
    else if (!selected && dialog.open) dialog.close();
  }, [selected]);

  useEffect(() => {
    const dialog = editDialog.current;
    if (!dialog) return;
    if (editing && !dialog.open) dialog.showModal();
    else if (!editing && dialog.open) dialog.close();
  }, [editing]);

  async function submit(action: () => Promise<void>) {
    setBusy(true); setError('');
    try { await action(); await refresh(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { setBusy(false); }
  }

  return <>
    {(error || loadError) && <section><p role="alert">{error || loadError}</p><button disabled={busy} onClick={() => void submit(async () => {})}>{t('common.retry')}</button></section>}
    {page === 'overview' && <>
      <div className="summary-grid">
        <a href="#videos"><span>{t('overview.videoTotal')}</span><strong>{data?.pagination.totalVideos ?? '—'}</strong></a>
        <a href="#pools"><span>{t('overview.onlinePools')}</span><strong>{data ? `${data.pools.filter(pool => pool.enabled && pool.online).length} / ${data.pools.filter(pool => pool.enabled).length}` : '—'}</strong></a>
        <a href="#jobs"><span>{t('overview.recentJobs')}</span><strong>{data?.jobs.length ?? '—'}</strong></a>
      </div>
      <section><h2>{t('overview.startTitle')}</h2><p>{t('overview.startIntro')}</p><div className="quick-links"><a href="#pools">{t('overview.managePools')}</a><a href="#import">{t('overview.import')}</a><a href="#videos">{t('overview.browse')}</a></div></section>
    </>}
    {page === 'videos' && <section><div className="section-heading"><div><h2>{t('library.importedTitle')}</h2><p className="muted">{data ? t('library.filterSummary', { matching: data.pagination.total, total: data.pagination.totalVideos }) : t('library.reading')}</p></div>
      <button disabled={busy} onClick={() => void submit(async () => {})}>{t('library.refresh')}</button></div>
      <VideoFilters pools={data?.pools || []} initialValues={filters} onApply={values => { setSelected(null); setEditing(null); setFilters(values); setVideoPage(1); }} />
      {data?.videos.length === 0 && <div className="empty-state">{data.pagination.totalVideos === 0 ? <>{t('library.empty')}{' '}<a href="#import">{t('library.importFirst')}</a></> : t('library.noMatches')}</div>}
      <div className="video-grid">{data?.videos.map(video => {
        const pool = data.pools.find(item => item.id === video.poolId);
        const availability = video.available ? t('library.available') : unavailableVideoLabel(video, pool?.name, t);
        return <article key={video.id} className="video-card">
        <div className="video-cover" aria-hidden="true">▶</div><h3>{video.title}</h3>
        <p>{video.category || t('library.unclassified')}{video.adult ? `${t('common.dot')}${t('common.adultTag')}` : ''}</p>
        {video.description && <p className="description">{video.description}</p>}
        <p>{video.license ? t('common.license', { license: video.license }) : t('common.licenseMissing')}</p>
        <Attribution video={video} />
        <p>{t('library.size', { size: (video.size / 1024 / 1024).toFixed(1), availability })}</p>
        <p className="muted">{mediaSummary(video, t)}</p>
        <button disabled={!video.available} title={!video.available ? availability : undefined} onClick={() => setSelected(video)}>{t('library.play')}</button>
        <button disabled={busy} onClick={() => { setEditing(video); setEditDraft({ ...video }); }}>{t('library.edit')}</button>
        <button disabled={busy || !video.available || data.jobs.some(job => job.source === 'reprobe' && job.videoId === video.id && (job.status === 'queued' || job.status === 'running'))} title={!video.available ? availability : undefined} onClick={() => void submit(async () => {
          await request<ImportJob>(`videos/${video.id}/probe`, {});
          window.location.hash = 'jobs';
        })}>{t('library.reprobe')}</button>
      </article>;
      })}</div>
      {data && <nav className="pagination" aria-label={t('library.paginationAria')}>
        <button disabled={data.pagination.page <= 1} onClick={() => setVideoPage(data.pagination.page - 1)}>{t('library.prev')}</button>
        <span>{t('library.pageOf', { page: data.pagination.page, pages: Math.max(1, Math.ceil(data.pagination.total / data.pagination.pageSize)) })}</span>
        <button disabled={data.pagination.page * data.pagination.pageSize >= data.pagination.total} onClick={() => setVideoPage(data.pagination.page + 1)}>{t('library.next')}</button>
      </nav>}
    </section>}
      {page === 'pools' && <div className="split-layout">
        <section><h2>{t('library.poolsTitle')}</h2><p className="muted">{t('library.poolsIntro')}</p>
          {data?.scan && <>
            <p>{data.scan.automatic ? t('library.scanAutoOn', { duration: formatDuration(data.scan.intervalSeconds, t) }) : t('library.scanAutoOff')}</p>
            <p className="muted">{t('library.scanStable', { duration: formatDuration(data.scan.stableSeconds, t) })}</p>
            <p className="muted">{t('library.scanBackoff', { hot: formatDuration(data.scan.intervalSeconds, t), dormant: formatDuration(data.scan.idleMaxSeconds || 0, t) })}</p>
            {data.scan.automatic && !data.scan.running && <p className="muted">{t('library.scanNext', { duration: formatDuration(data.scan.nextRunInSeconds || 0, t) })}</p>}
          </>}
          <div className="button-row">
            {data?.scan && <button disabled={busy || data.scan.running || data.scan.queued} onClick={() => void submit(async () => { await request('library/scan', {}); })}>{data.scan.running ? t('library.scanRunning') : data.scan.queued ? t('library.scanQueued') : t('library.scanNow')}</button>}
            <button disabled={busy} onClick={() => void submit(async () => { await request('pools/refresh', {}); })}>{t('library.recheck')}</button>
          </div>
          {data?.scan?.last && <p className="muted">{t('library.scanLast', {
            time: new Date(data.scan.last.completedAt).toLocaleString(locale),
            enqueued: data.scan.last.enqueued || 0,
            pending: data.scan.last.pending,
            changed: data.scan.last.changed || 0,
            deleted: data.scan.last.deleted || 0,
            failed: data.scan.last.failed,
            offline: data.scan.last.offlinePools,
          })}{t('common.dot')}<a href="#jobs">{t('library.viewJobs')}</a></p>}
          {data?.scan?.last?.errors.length ? <details><summary>{t('library.scanErrors')}</summary><ul>{data.scan.last!.errors.map((message, index) => <li key={index}>{message}</li>)}</ul></details> : null}
          {data?.pools.length === 0 && <p>{t('library.noPools')}</p>}
          <ul className="pool-list">{data?.pools.map(pool => {
            const status = !pool.enabled ? t('library.disabled') : pool.online ? t('library.online') : t('library.offline');
            const statusClass = !pool.enabled ? 'disabled' : pool.online ? 'online' : 'offline';
            const scanInfo = data.scan?.pools?.find(item => item.poolId === pool.id);
            const scanLabel = !pool.enabled ? t('library.poolScanDisabled')
              : !pool.online ? t('library.poolScanOffline')
                : scanInfo ? t('library.poolScanNext', { duration: formatDuration(scanInfo.nextRunInSeconds, t), interval: formatDuration(scanInfo.backoffSeconds, t) })
                  : t('library.poolScanWaiting');
            return <li key={pool.id} className="pool-card">
              <div className="pool-head">
                <strong>{pool.name}</strong>
                <span className={`state-badge ${statusClass}`}>{status}</span>
              </div>
              <dl className="pool-meta">
                <div><dt>{t('library.poolPathLabel')}</dt><dd><code>{pool.path}</code></dd></div>
                <div><dt>{t('library.filesystem')}</dt><dd>{pool.filesystem || t('common.notProvided')}</dd></div>
                <div><dt>{t('library.volumeGuid')}</dt><dd>{pool.volumeId
                  ? <code className="volume-id" title={pool.volumeId}>{pool.volumeId}</code>
                  : pool.volumeStatus === 'unsupported' ? t('library.volumeNotApplicable')
                    : pool.volumeStatus === 'unbound' ? t('library.volumeUnbound') : t('common.notProvided')}</dd></div>
                <div><dt>{t('library.poolScanLabel')}</dt><dd>{scanInfo && <><ScanTierBadge tier={scanInfo.tier} /> </>}{scanLabel}</dd></div>
              </dl>
              <div className="pool-actions">
                {pool.volumeId && <span className="muted">{t('library.volumeTracked')}</span>}
                <button type="button" disabled={busy || data.scan?.running || data.scan?.queued || !pool.enabled || !pool.online}
                  onClick={() => void submit(async () => { await request(`pools/${pool.id}/scan`, {}); })}>{t('library.scanPool')}</button>
                <button type="button" disabled={busy} onClick={() => void submit(async () => {
                  await request<Pool>(`pools/${pool.id}`, { enabled: !pool.enabled }, 'PATCH');
                })}>{pool.enabled ? t('library.disablePool') : t('library.enablePool')}</button>
              </div>
            </li>;
          })}</ul>
          <p className="muted scan-tier-legend">{t('scanTier.legend')}</p>
        </section>
        <section><h2>{t('library.addPoolTitle')}</h2><p className="muted">{t('library.addPoolIntro')}</p>
          <form onSubmit={event => { event.preventDefault(); void submit(async () => {
            const pool = await request<Pool>('pools', { name, path: root });
            setPoolId(pool.id); setName(''); setRoot('');
          }); }}>
            <label>{t('library.poolName')}<input required value={name} onChange={event => setName(event.target.value)} placeholder={t('library.poolNamePlaceholder')} /></label>
            <label>{t('library.poolPath')}<input required value={root} onChange={event => setRoot(event.target.value)} placeholder={t('library.poolPathPlaceholder')} /></label>
            <button disabled={busy}>{t('library.registerPool')}</button>
          </form>
        </section>
      </div>}
      {page === 'import' && <form className="import-layout" onSubmit={event => { event.preventDefault(); void submit(async () => {
        await request<ImportJob>('videos/import', { poolId, path, ...draft });
        setPath(''); setDraft(emptyMetadata());
        window.location.hash = 'jobs';
      }); }}>
        <section className="import-source form-stack"><h2>{t('library.importTitle')}</h2><p className="muted">{t('library.importIntro')}</p>
          {data?.pools.length === 0 && <p>{t('library.noPoolsImport')}{' '}<a href="#pools">{t('library.registerFirst')}</a></p>}
          <label>{t('library.pool')}<select required value={poolId} onChange={event => setPoolId(event.target.value)}><option value="">{t('library.selectPool')}</option>{data?.pools.map(pool => <option key={pool.id} value={pool.id} disabled={!pool.enabled || !pool.online}>{pool.name}{!pool.enabled ? t('library.disabledSuffix') : pool.online ? '' : t('library.offlineSuffix')}</option>)}</select></label>
          <label>{t('library.relativePath')}<input required value={path} onChange={event => setPath(event.target.value)} placeholder={t('library.relativePathPlaceholder')} /></label>
        </section>
        <section className="import-metadata form-stack"><h2>{t('library.metadataTitle')}</h2><p className="muted">{t('library.metadataIntro')}</p>
          <MetadataFields value={draft} onChange={setDraft} />
          <button disabled={busy || !poolId}>{busy ? t('library.adding') : t('library.addToQueue')}</button>
          {busy && <p role="status">{t('library.processing')}</p>}
        </section>
      </form>}
    {page === 'jobs' && <ImportJobs />}
    <dialog ref={playerDialog} className="modal" aria-label={t('library.playerAria')} onClose={() => setSelected(null)} onClick={event => { if (event.target === event.currentTarget) setSelected(null); }}>
      {selected && <><div className="modal-heading"><h2>{selected.title}</h2><button onClick={() => setSelected(null)}>{t('library.closePlayer')}</button></div>
        <video key={selected.id} controls autoPlay preload="metadata" src={`/api/v1/videos/${selected.id}/content`} />
        <p>{selected.category || t('library.unclassified')}{t('common.dot')}{selected.adult ? t('library.adultContent') : t('library.notAdult')}{t('common.dot')}{selected.license ? t('common.license', { license: selected.license }) : t('common.licenseMissing')}</p>
        {selected.description && <p className="description">{selected.description}</p>}
        <Attribution video={selected} />
        <p className="muted">{t('library.playerNote')}</p></>}
    </dialog>
    <dialog ref={editDialog} className="modal edit-modal" aria-label={t('library.editAria')} onClose={() => setEditing(null)} onClick={event => { if (event.target === event.currentTarget) setEditing(null); }}>
      {editing && <><div className="modal-heading"><h2>{t('library.editAria')}</h2><button disabled={busy} onClick={() => setEditing(null)}>{t('library.cancelEdit')}</button></div>
        <form onSubmit={event => { event.preventDefault(); void submit(async () => {
          await request(`videos/${editing.id}/metadata`, {
            title: editDraft.title, description: editDraft.description, category: editDraft.category,
            adult: editDraft.adult, license: editDraft.license, tags: editDraft.tags, creators: editDraft.creators,
            workType: editDraft.workType, sourceUrl: editDraft.sourceUrl, sourcePlatform: editDraft.sourcePlatform,
            sourcePending: editDraft.sourcePending, repostNote: editDraft.repostNote, language: editDraft.language,
          }, 'PUT');
          setEditing(null);
        }); }}><MetadataFields value={editDraft} onChange={setEditDraft} editing /><button disabled={busy}>{t('library.saveData')}</button></form></>}
    </dialog>
  </>;
}
