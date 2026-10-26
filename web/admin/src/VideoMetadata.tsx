/** Creator 描述作品的原作者，與匯入影片的管理者身分分開。 */
export type Creator = { name: string; url: string };

/** VideoMetadata 是匯入與編輯共用的完整描述資料。 */
export type VideoMetadata = {
  title: string; description: string; category: string; adult: boolean; license: string;
  tags: string[]; creators: Creator[]; workType: 'unknown' | 'original' | 'repost';
  sourceUrl: string; sourcePlatform: string; sourcePending: boolean; repostNote: string; language: string;
};

/** emptyMetadata 回傳獨立的表單預設值，不預先聲明作品為原創。 */
export function emptyMetadata(): VideoMetadata {
  return { title: '', description: '', category: '', adult: false, license: '', tags: [], creators: [],
    workType: 'unknown', sourceUrl: '', sourcePlatform: '', sourcePending: false, repostNote: '', language: '' };
}

/** MetadataFields 共用匯入與編輯欄位；value 為表單值，onChange 回傳完整新值。 */
export function MetadataFields({ value, onChange, editing = false }: {
  value: VideoMetadata; onChange: (value: VideoMetadata) => void; editing?: boolean;
}) {
  const patch = (change: Partial<VideoMetadata>) => onChange({ ...value, ...change });
  const updateCreator = (index: number, change: Partial<Creator>) => patch({
    creators: value.creators.map((creator, current) => current === index ? { ...creator, ...change } : creator),
  });
  return <>
    <label>影片標題{editing ? '' : '（選填）'}<input required={editing} value={value.title} onChange={event => patch({ title: event.target.value })} placeholder="預設使用檔案名稱" /></label>
    <label>內容簡介（選填）<textarea rows={4} maxLength={10000} value={value.description} onChange={event => patch({ description: event.target.value })} /></label>
    <label>分類（選填）<input maxLength={80} value={value.category} onChange={event => patch({ category: event.target.value })} placeholder="例如：教育、科技、生活" /></label>
    <label>Tag（選填）<input value={value.tags.join(',')} onChange={event => patch({ tags: event.target.value.split(/[,，]/) })} placeholder="以逗號分隔，例如 Go,後端開發,入門" /></label>
    <small className="muted">最多 20 個，每個 40 字；儲存時自動移除空白與重複標記。</small>
    <label>影片語言（選填）<input maxLength={80} value={value.language} onChange={event => patch({ language: event.target.value })} placeholder="例如：繁體中文、English" /></label>
    <label>18+ 內容標記<select value={value.adult ? 'yes' : 'no'} onChange={event => patch({ adult: event.target.value === 'yes' })}><option value="no">未標示 18+</option><option value="yes">包含 18+ 內容</option></select></label>
    <fieldset><legend>作品與來源</legend>
      <label>作品類型<select value={value.workType} onChange={event => patch({ workType: event.target.value as VideoMetadata['workType'] })}><option value="unknown">未確認</option><option value="original">原創</option><option value="repost">轉載</option></select></label>
      <p className="muted">原作者與匯入者分開記錄。轉載需填原作者，並提供作品來源或標示待補。</p>
      {value.creators.map((creator, index) => <div className="creator-row" key={index}>
        <label>原作者 {index + 1}<input required maxLength={120} value={creator.name} onChange={event => updateCreator(index, { name: event.target.value })} /></label>
        <label>作者頁面／頻道（選填）<input type="url" value={creator.url} onChange={event => updateCreator(index, { url: event.target.value })} placeholder="https://…" /></label>
        <button type="button" onClick={() => patch({ creators: value.creators.filter((_, current) => current !== index) })}>移除此作者</button>
      </div>)}
      <button type="button" disabled={value.creators.length >= 20} onClick={() => patch({ creators: [...value.creators, { name: '', url: '' }] })}>新增原作者</button>
      {value.workType === 'repost' && <>
        <label>來源平台（選填）<input maxLength={80} value={value.sourcePlatform} onChange={event => patch({ sourcePlatform: event.target.value })} placeholder="例如：B站、YouTube" /></label>
        <label>原始作品連結<input type="url" required={!value.sourcePending} value={value.sourceUrl} onChange={event => patch({ sourceUrl: event.target.value, sourcePending: event.target.value ? false : value.sourcePending })} placeholder="原始影片的發布頁網址" /></label>
        <label className="checkbox-label"><input type="checkbox" checked={value.sourcePending} disabled={Boolean(value.sourceUrl)} onChange={event => patch({ sourcePending: event.target.checked })} />來源待補（尚未找到原始作品連結）</label>
        <label>轉載說明（選填）<textarea rows={3} maxLength={2000} value={value.repostNote} onChange={event => patch({ repostNote: event.target.value })} placeholder="例如：翻譯、剪輯或取得授權的情況" /></label>
      </>}
    </fieldset>
    <label>授權資訊（選填）<input maxLength={500} value={value.license} onChange={event => patch({ license: event.target.value })} placeholder="例如：保留所有權利、CC BY 4.0" /></label>
    <p className="muted">註明轉載來源不代表取得授權。授權留空表示未提供。18+ 目前僅為標記。</p>
  </>;
}

/** Attribution 顯示作品聲明與原作者連結；所有連結均由後端驗證 HTTP(S)。 */
export function Attribution({ video }: { video: VideoMetadata }) {
  return <div className="attribution">
    <p>{video.workType === 'repost' ? '轉載自' : video.workType === 'original' ? '原創 · 作者' : '作品類型未確認 · 作者'}{' '}
      {video.creators.length ? video.creators.map((creator, index) => <span key={index}>{index > 0 && '、'}{creator.url ? <a href={creator.url} target="_blank" rel="noopener noreferrer">{creator.name}</a> : creator.name}</span>) : '未提供'}</p>
    {video.workType === 'repost' && <p>{video.sourcePlatform && `${video.sourcePlatform} · `}{video.sourceUrl ? <a href={video.sourceUrl} target="_blank" rel="noopener noreferrer">查看原始作品</a> : '來源待補'}</p>}
    {video.workType === 'repost' && video.repostNote && <p className="description">{video.repostNote}</p>}
    {video.tags.length > 0 && <div className="tags">{video.tags.map(tag => <span key={tag}>#{tag}</span>)}</div>}
    {video.language && <p>語言：{video.language}</p>}
  </div>;
}
