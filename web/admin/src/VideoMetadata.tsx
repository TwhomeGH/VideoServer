import { useI18n } from './i18n';

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
  const { t } = useI18n();
  const patch = (change: Partial<VideoMetadata>) => onChange({ ...value, ...change });
  const updateCreator = (index: number, change: Partial<Creator>) => patch({
    creators: value.creators.map((creator, current) => current === index ? { ...creator, ...change } : creator),
  });
  return <>
    <label>{t('metadata.title')}{editing ? '' : t('metadata.optional')}<input required={editing} value={value.title} onChange={event => patch({ title: event.target.value })} placeholder={t('metadata.titlePlaceholder')} /></label>
    <label>{t('metadata.description')}<textarea rows={4} maxLength={10000} value={value.description} onChange={event => patch({ description: event.target.value })} /></label>
    <label>{t('metadata.category')}<input maxLength={80} value={value.category} onChange={event => patch({ category: event.target.value })} placeholder={t('metadata.categoryPlaceholder')} /></label>
    <label>{t('metadata.tags')}<input value={value.tags.join(',')} onChange={event => patch({ tags: event.target.value.split(/[,，]/) })} placeholder={t('metadata.tagsPlaceholder')} /></label>
    <small className="muted">{t('metadata.tagsHint')}</small>
    <label>{t('metadata.language')}<input maxLength={80} value={value.language} onChange={event => patch({ language: event.target.value })} placeholder={t('metadata.languagePlaceholder')} /></label>
    <label>{t('metadata.adult')}<select value={value.adult ? 'yes' : 'no'} onChange={event => patch({ adult: event.target.value === 'yes' })}><option value="no">{t('metadata.adultNo')}</option><option value="yes">{t('metadata.adultYes')}</option></select></label>
    <fieldset><legend>{t('metadata.workSource')}</legend>
      <label>{t('metadata.workType')}<select value={value.workType} onChange={event => patch({ workType: event.target.value as VideoMetadata['workType'] })}><option value="unknown">{t('metadata.unknown')}</option><option value="original">{t('metadata.original')}</option><option value="repost">{t('metadata.repost')}</option></select></label>
      <p className="muted">{t('metadata.creatorNote')}</p>
      {value.creators.map((creator, index) => <div className="creator-row" key={index}>
        <label>{t('metadata.creator', { index: index + 1 })}<input required maxLength={120} value={creator.name} onChange={event => updateCreator(index, { name: event.target.value })} /></label>
        <label>{t('metadata.creatorUrl')}<input type="url" value={creator.url} onChange={event => updateCreator(index, { url: event.target.value })} placeholder="https://…" /></label>
        <button type="button" onClick={() => patch({ creators: value.creators.filter((_, current) => current !== index) })}>{t('metadata.removeCreator')}</button>
      </div>)}
      <button type="button" disabled={value.creators.length >= 20} onClick={() => patch({ creators: [...value.creators, { name: '', url: '' }] })}>{t('metadata.addCreator')}</button>
      {value.workType === 'repost' && <>
        <label>{t('metadata.sourcePlatform')}<input maxLength={80} value={value.sourcePlatform} onChange={event => patch({ sourcePlatform: event.target.value })} placeholder={t('metadata.sourcePlatformPlaceholder')} /></label>
        <label>{t('metadata.sourceUrl')}<input type="url" required={!value.sourcePending} value={value.sourceUrl} onChange={event => patch({ sourceUrl: event.target.value, sourcePending: event.target.value ? false : value.sourcePending })} placeholder={t('metadata.sourceUrlPlaceholder')} /></label>
        <label className="checkbox-label"><input type="checkbox" checked={value.sourcePending} disabled={Boolean(value.sourceUrl)} onChange={event => patch({ sourcePending: event.target.checked })} />{t('metadata.sourcePending')}</label>
        <label>{t('metadata.repostNote')}<textarea rows={3} maxLength={2000} value={value.repostNote} onChange={event => patch({ repostNote: event.target.value })} placeholder={t('metadata.repostNotePlaceholder')} /></label>
      </>}
    </fieldset>
    <label>{t('metadata.license')}<input maxLength={500} value={value.license} onChange={event => patch({ license: event.target.value })} placeholder={t('metadata.licensePlaceholder')} /></label>
    <p className="muted">{t('metadata.legalNote')}</p>
  </>;
}

/** Attribution 顯示作品聲明與原作者連結；所有連結均由後端驗證 HTTP(S)。 */
export function Attribution({ video }: { video: VideoMetadata }) {
  const { t } = useI18n();
  return <div className="attribution">
    <p>{video.workType === 'repost' ? t('attribution.repost') : video.workType === 'original' ? t('attribution.original') : t('attribution.unknown')}{' '}
      {video.creators.length ? video.creators.map((creator, index) => <span key={index}>{index > 0 && t('common.listSeparator')}{creator.url ? <a href={creator.url} target="_blank" rel="noopener noreferrer">{creator.name}</a> : creator.name}</span>) : t('attribution.notProvided')}</p>
    {video.workType === 'repost' && <p>{video.sourcePlatform && `${video.sourcePlatform}${t('common.dot')}`}{video.sourceUrl ? <a href={video.sourceUrl} target="_blank" rel="noopener noreferrer">{t('attribution.viewOriginal')}</a> : t('attribution.sourcePending')}</p>}
    {video.workType === 'repost' && video.repostNote && <p className="description">{video.repostNote}</p>}
    {video.tags.length > 0 && <div className="tags">{video.tags.map(tag => <span key={tag}>#{tag}</span>)}</div>}
    {video.language && <p>{t('attribution.language', { language: video.language })}</p>}
  </div>;
}
