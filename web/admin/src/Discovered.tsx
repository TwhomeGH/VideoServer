import { useEffect, useState } from 'react';
import { api, jsonRequest } from './api';
import { useI18n } from './i18n';
import { useLiveVersion } from './live';

type DiscoveredMedia = {
  poolId: string; poolName: string; provider: string; relativePath: string; size: number;
  modifiedAt: number; firstSeen: string; lastSeen: string;
};
type DiscoveredPage = { items: DiscoveredMedia[]; total: number; page: number; pageSize: number };

const PAGE_SIZE = 50;

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KiB`;
  if (value < 1024 * 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MiB`;
  return `${(value / 1024 / 1024 / 1024).toFixed(2)} GiB`;
}

/** Discovered 顯示 index-only 掃描找到、尚未匯入的影片。 */
export function Discovered() {
  const { t, locale } = useI18n();
  const [data, setData] = useState<DiscoveredPage | null>(null);
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Map<string, DiscoveredMedia>>(() => new Map());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [revision, setRevision] = useState(0);
  const liveJobs = useLiveVersion('jobs');
  const liveScan = useLiveVersion('scan');

  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const result = await api<DiscoveredPage>(`discovered?page=${page}&pageSize=${PAGE_SIZE}&q=${encodeURIComponent(query)}`, { signal: controller.signal });
        if (controller.signal.aborted) return;
        setData(result);
        setError('');
      } catch (reason) {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : String(reason));
      }
      if (!controller.signal.aborted) timer = setTimeout(poll, 5000);
    };
    void poll();
    return () => { controller.abort(); clearTimeout(timer); };
  }, [page, query, revision, liveJobs, liveScan]);

  const keyOf = (item: DiscoveredMedia) => `${item.poolId}\n${item.relativePath}`;
  const toggle = (item: DiscoveredMedia) => {
    if (item.provider === 'webdav') return;
    setSelected(current => {
      const next = new Map(current);
      const key = keyOf(item);
      if (next.has(key)) next.delete(key); else next.set(key, item);
      return next;
    });
  };
  const togglePage = () => {
    const visible = data?.items ?? [];
    const allSelected = visible.length > 0 && visible.every(item => selected.has(keyOf(item)));
    setSelected(current => {
      const next = new Map(current);
      for (const item of visible) {
        if (allSelected) next.delete(keyOf(item)); else if (item.provider !== 'webdav') next.set(keyOf(item), item);
      }
      return next;
    });
  };
  const importItems = async (target: DiscoveredMedia[]) => {
    if (!target.length) return;
    setBusy(true); setError(''); setNotice('');
    try {
      const items = target.filter(item => item.provider !== 'webdav').map(item => ({ poolId: item.poolId, relativePath: item.relativePath }));
      if (!items.length) return;
      const result = await api<{ imported: number; errors: string[] }>('discovered/import', jsonRequest('POST', { items }));
      setSelected(new Map());
      setNotice(t('discovered.imported', { count: result.imported }));
      if (result.errors?.length) setError(result.errors.join('；'));
      setRevision(value => value + 1);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  };
  const importSelected = () => importItems([...selected.values()]);

  const items = data?.items ?? [];
  const pageCount = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE));
  const allPageSelected = items.length > 0 && items.every(item => selected.has(keyOf(item)));
  return <section>
    <h2>{t('discovered.title')}</h2>
    <p className="muted">{t('discovered.intro')}</p>
    <div className="discovered-toolbar">
      <input value={query} onChange={event => { setQuery(event.target.value); setPage(1); }} placeholder={t('discovered.search')} aria-label={t('discovered.search')} />
      <span className="muted">{data ? t('discovered.summary', { total: data.total, selected: selected.size }) : t('discovered.loading')}</span>
    </div>
    {error && <p role="alert">{error}</p>}
    {notice && <p className="notice">{notice}</p>}
    <div className="discovered-actions">
      <button type="button" disabled={busy || items.length === 0} onClick={togglePage}>{allPageSelected ? t('discovered.clearPage') : t('discovered.selectPage')}</button>
      <button type="button" disabled={busy || selected.size === 0} onClick={() => void importSelected()}>{t('discovered.importSelected', { count: selected.size })}</button>
    </div>
    {data && items.length === 0
      ? <p className="muted">{query ? t('discovered.noMatches') : t('discovered.empty')}</p>
      : <div className="discovered-table" role="table" aria-label={t('discovered.title')}>
        <div className="discovered-head" role="row">
          <span /><span>{t('discovered.file')}</span><span>{t('discovered.pool')}</span>
          <span>{t('discovered.size')}</span><span>{t('discovered.modified')}</span><span>{t('discovered.action')}</span>
        </div>
        {items.map(item => {
          const key = keyOf(item);
          return <div className="discovered-row" role="row" key={key}>
            <input type="checkbox" checked={selected.has(key)} disabled={item.provider === 'webdav'} onChange={() => toggle(item)} aria-label={item.relativePath} />
            <span className="discovered-path" title={item.relativePath}>{item.relativePath}</span>
            <span className="muted">{item.poolName}{item.provider === 'webdav' ? ` · ${t('discovered.remoteOnly')}` : ''}</span>
            <span className="muted">{formatBytes(item.size)}</span>
            <span className="muted">{new Date(item.modifiedAt).toLocaleString(locale)}</span>
            <button type="button" disabled={busy || item.provider === 'webdav'} title={item.provider === 'webdav' ? t('discovered.remoteImportUnavailable') : undefined} onClick={() => void importItems([item])}>{t('discovered.importOne')}</button>
          </div>;
        })}
      </div>}
    {pageCount > 1 && <div className="job-pagination">
      <button type="button" disabled={page <= 1} onClick={() => { setPage(value => Math.max(1, value - 1)); setSelected(new Map()); }}>{t('importJobs.prevPage')}</button>
      <span className="muted">{t('importJobs.pageOf', { page, pages: pageCount })}</span>
      <button type="button" disabled={page >= pageCount} onClick={() => { setPage(value => value + 1); setSelected(new Map()); }}>{t('importJobs.nextPage')}</button>
    </div>}
  </section>;
}
