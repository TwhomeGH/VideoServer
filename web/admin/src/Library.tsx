import { useEffect, useRef, useState } from 'react';
import { Attribution, emptyMetadata, MetadataFields, type VideoMetadata } from './VideoMetadata';
import type { Page } from './navigation';

type Pool = { id: string; name: string; path: string; online: boolean };
type Video = VideoMetadata & {
  id: string; title: string; poolId: string; size: number; fileHash: string;
  available: boolean; importedAt: string;
  description: string; category: string; adult: boolean; license: string;
};
type ScanStatus = {
  automatic: boolean; intervalSeconds: number; stableSeconds: number; running: boolean; queued: boolean;
  last: { completedAt: string; imported: number; pending: number; failed: number; offlinePools: number; errors: string[] } | null;
};
type LibraryData = { nodeId: string; pools: Pool[]; videos: Video[]; scan: ScanStatus | null };

async function request<T>(path: string, body?: object, method = 'POST'): Promise<T> {
  const response = await fetch(`/api/v1/${path}`, body ? {
    method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
  } : undefined);
  const result = await response.json();
  if (!response.ok) throw new Error(result.error || `服務回應 ${response.status}`);
  return result as T;
}

/** Library 依 page 顯示管理分頁，切換時保留尚未送出的表單資料。 */
export function Library({ page, onNodeId }: { page: Page; onNodeId?: (nodeId: string) => void }) {
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
  const playerDialog = useRef<HTMLDialogElement>(null);
  const editDialog = useRef<HTMLDialogElement>(null);

  async function refresh() {
    const result = await request<LibraryData>('library');
    result.videos = result.videos.map(video => ({ ...emptyMetadata(), ...video }));
    setData(result);
    onNodeId?.(result.nodeId);
    setLoadError('');
    setSelected(current => current ? result.videos.find(video => video.id === current.id) || null : null);
    setPoolId(current => current || result.pools[0]?.id || '');
  }

  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (active) {
        try { await refresh(); }
        catch (reason) { if (active) setLoadError(reason instanceof Error ? reason.message : String(reason)); }
        if (active) timer = setTimeout(poll, 5000);
      }
    };
    void poll();
    return () => { active = false; clearTimeout(timer); };
  }, []);

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
    {(error || loadError) && <section><p role="alert">{error || loadError}</p><button disabled={busy} onClick={() => void submit(async () => {})}>重試</button></section>}
    {page === 'overview' && <>
      <div className="summary-grid">
        <a href="#videos"><span>影片總數</span><strong>{data?.videos.length ?? '—'}</strong></a>
        <a href="#pools"><span>在線儲存池</span><strong>{data ? `${data.pools.filter(pool => pool.online).length} / ${data.pools.length}` : '—'}</strong></a>
        <a href="#videos"><span>可用影片來源</span><strong>{data?.videos.filter(video => video.available).length ?? '—'}</strong></a>
      </div>
      <section><h2>開始管理</h2><p>先登錄影片目錄，再匯入目錄內的檔案。</p><div className="quick-links"><a href="#pools">管理儲存池</a><a href="#import">匯入影片</a><a href="#videos">瀏覽影片庫</a></div></section>
    </>}
    {page === 'videos' && <section><div className="section-heading"><div><h2>已匯入影片</h2><p className="muted">{data ? `${data.videos.length} 部影片` : '正在讀取影片庫…'}</p></div>
      <button disabled={busy} onClick={() => void submit(async () => {})}>重新整理</button></div>
      {data?.videos.length === 0 && <div className="empty-state">影片庫還是空的。<a href="#import">匯入第一部影片</a></div>}
      <div className="video-grid">{data?.videos.map(video => <article key={video.id} className="video-card">
        <div className="video-cover" aria-hidden="true">▶</div><h3>{video.title}</h3>
        <p>{video.category || '未分類'}{video.adult ? ' · 18+' : ''}</p>
        {video.description && <p className="description">{video.description}</p>}
        <p>授權：{video.license || '未提供'}</p>
        <Attribution video={video} />
        <p>{(video.size / 1024 / 1024).toFixed(1)} MB · {video.available ? '來源可用' : '離線或已變更'}</p>
        <button disabled={!video.available} onClick={() => setSelected(video)}>播放影片</button>
        <button disabled={busy} onClick={() => { setEditing(video); setEditDraft({ ...video }); }}>編輯資料</button>
      </article>)}</div>
    </section>}
      {page === 'pools' && <div className="split-layout">
        <section><h2>儲存目錄與狀態</h2><p className="muted">登錄伺服器上的既有目錄，原始檔案維持原位。</p>
          {data?.scan && <p>{data.scan.automatic ? `自動檢查已開啟，每 ${data.scan.intervalSeconds} 秒掃描一次` : '自動檢查已停用，可手動掃描'}；新增檔案需連續觀察穩定至少 {data.scan.stableSeconds} 秒才匯入。</p>}
          <div className="button-row">
            {data?.scan && <button disabled={busy || data.scan.running || data.scan.queued} onClick={() => void submit(async () => { await request('library/scan', {}); })}>{data.scan.running ? '正在掃描…' : data.scan.queued ? '掃描已排程' : '立即掃描新增影片'}</button>}
            <button disabled={busy} onClick={() => void submit(async () => {})}>重新檢查狀態</button>
          </div>
          {data?.scan?.last && <p className="muted">上次完成：{new Date(data.scan.last.completedAt).toLocaleString()} · 新增 {data.scan.last.imported} · 等待穩定 {data.scan.last.pending} · 失敗 {data.scan.last.failed} · 離線池 {data.scan.last.offlinePools}</p>}
          {data?.scan?.last?.errors.length ? <details><summary>掃描錯誤</summary><ul>{data.scan.last!.errors.map((message, index) => <li key={index}>{message}</li>)}</ul></details> : null}
          {data?.pools.length === 0 && <p>尚未註冊儲存池，請先新增影片所在的目錄。</p>}
          <ul className="pool-list">{data?.pools.map(pool => <li key={pool.id}><strong>{pool.name}</strong><span className={pool.online ? 'badge' : ''}>{pool.online ? '在線' : '離線'}</span><code>{pool.path}</code></li>)}</ul>
        </section>
        <section><h2>新增儲存池</h2><p className="muted">註冊新的影片目錄，之後可透過掃描或手動匯入加入影片。</p>
          <form onSubmit={event => { event.preventDefault(); void submit(async () => {
            const pool = await request<Pool>('pools', { name, path: root });
            setPoolId(pool.id); setName(''); setRoot('');
          }); }}>
            <label>儲存池名稱<input required value={name} onChange={event => setName(event.target.value)} placeholder="例如：主影片庫" /></label>
            <label>伺服器絕對目錄<input required value={root} onChange={event => setRoot(event.target.value)} placeholder="例如：D:\Videos 或 /srv/videos" /></label>
            <button disabled={busy}>註冊儲存池</button>
          </form>
        </section>
      </div>}
      {page === 'import' && <form className="import-layout" onSubmit={event => { event.preventDefault(); void submit(async () => {
        await request('videos/import', { poolId, path, ...draft });
        setPath(''); setDraft(emptyMetadata());
      }); }}>
        <section className="import-source form-stack"><h2>匯入既有影片</h2><p className="muted">建立 SHA-256 索引，不複製檔案。大型影片計算雜湊需要一些時間。</p>
          {data?.pools.length === 0 && <p>尚無儲存池。<a href="#pools">先註冊影片目錄</a></p>}
          <label>儲存池<select required value={poolId} onChange={event => setPoolId(event.target.value)}><option value="">選擇儲存池</option>{data?.pools.map(pool => <option key={pool.id} value={pool.id} disabled={!pool.online}>{pool.name}{pool.online ? '' : '（離線）'}</option>)}</select></label>
          <label>池內相對路徑<input required value={path} onChange={event => setPath(event.target.value)} placeholder="例如：旅行/第一天.mp4" /></label>
        </section>
        <section className="import-metadata form-stack"><h2>影片資料</h2><p className="muted">自動匯入使用檔名作為標題，作品類型保留「未確認」；其他描述可稍後在影片庫補填。</p>
          <MetadataFields value={draft} onChange={setDraft} />
          <button disabled={busy || !poolId}>{busy ? '處理中…' : '匯入影片'}</button>
          {busy && <p role="status">正在處理，請等待完成。</p>}
        </section>
      </form>}
    <dialog ref={playerDialog} className="modal" aria-label="影片播放器" onClose={() => setSelected(null)} onClick={event => { if (event.target === event.currentTarget) setSelected(null); }}>
      {selected && <><div className="modal-heading"><h2>{selected.title}</h2><button onClick={() => setSelected(null)}>關閉播放器</button></div>
        <video key={selected.id} controls autoPlay preload="metadata" src={`/api/v1/videos/${selected.id}/content`} />
        <p>{selected.category || '未分類'} · {selected.adult ? '18+ 內容' : '未標示 18+'} · 授權：{selected.license || '未提供'}</p>
        {selected.description && <p className="description">{selected.description}</p>}
        <Attribution video={selected} />
        <p className="muted">此階段直接播放原始檔。是否能播放取決於瀏覽器支援的編碼，尚未提供自動轉碼。</p></>}
    </dialog>
    <dialog ref={editDialog} className="modal edit-modal" aria-label="編輯影片資料" onClose={() => setEditing(null)} onClick={event => { if (event.target === event.currentTarget) setEditing(null); }}>
      {editing && <><div className="modal-heading"><h2>編輯影片資料</h2><button disabled={busy} onClick={() => setEditing(null)}>取消編輯</button></div>
        <form onSubmit={event => { event.preventDefault(); void submit(async () => {
          await request(`videos/${editing.id}/metadata`, {
            title: editDraft.title, description: editDraft.description, category: editDraft.category,
            adult: editDraft.adult, license: editDraft.license, tags: editDraft.tags, creators: editDraft.creators,
            workType: editDraft.workType, sourceUrl: editDraft.sourceUrl, sourcePlatform: editDraft.sourcePlatform,
            sourcePending: editDraft.sourcePending, repostNote: editDraft.repostNote, language: editDraft.language,
          }, 'PUT');
          setEditing(null);
        }); }}><MetadataFields value={editDraft} onChange={setEditDraft} editing /><button disabled={busy}>儲存資料</button></form></>}
    </dialog>
  </>;
}
