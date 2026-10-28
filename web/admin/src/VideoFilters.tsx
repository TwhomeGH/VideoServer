import { useState } from 'react';
import { useI18n } from './i18n';

/** VideoFilterValues 保存已套用的查詢條件，送出表單才更新清單。 */
export type VideoFilterValues = { q: string; tag: string; category: string; poolId: string; adult: string; sort: string; pageSize: string };

/** defaultVideoFilters 回傳不篩選、每頁 24 筆的獨立查詢設定。 */
export function defaultVideoFilters(): VideoFilterValues {
  return { q: '', tag: '', category: '', poolId: '', adult: '', sort: 'newest', pageSize: '24' };
}

/** VideoFilters 提供文字與內容標記篩選；onApply 回傳條件並由父層重設頁碼。 */
export function VideoFilters({ pools, categories, initialValues, onApply }: {
  pools: { id: string; name: string; enabled?: boolean }[]; categories: { id: string; name: string }[]; initialValues: VideoFilterValues; onApply: (values: VideoFilterValues) => void;
}) {
  const { t } = useI18n();
  const [values, setValues] = useState(initialValues);
  const patch = (field: keyof VideoFilterValues, value: string) => setValues(current => ({ ...current, [field]: value }));
  return <form className="video-filters" onSubmit={event => { event.preventDefault(); onApply(values); }}>
    <label>{t('filters.search')}<input maxLength={200} value={values.q} onChange={event => patch('q', event.target.value)} placeholder={t('filters.searchPlaceholder')} /></label>
    <label>{t('filters.tag')}<input maxLength={200} value={values.tag} onChange={event => patch('tag', event.target.value)} placeholder={t('filters.tagPlaceholder')} /></label>
    <label>{t('filters.category')}<select value={values.category} onChange={event => patch('category', event.target.value)}><option value="">{t('filters.allCategories')}</option>{categories.map(category => <option key={category.id} value={category.name}>{category.name}</option>)}</select></label>
    <label>{t('filters.pool')}<select value={values.poolId} onChange={event => patch('poolId', event.target.value)}><option value="">{t('filters.allPools')}</option>{pools.map(pool => <option key={pool.id} value={pool.id}>{pool.name}{pool.enabled === false ? t('library.disabledSuffix') : ''}</option>)}</select></label>
    <label>{t('filters.adult')}<select value={values.adult} onChange={event => patch('adult', event.target.value)}><option value="">{t('filters.all')}</option><option value="true">{t('filters.adultYes')}</option><option value="false">{t('filters.adultNo')}</option></select></label>
    <label>{t('filters.sort')}<select value={values.sort} onChange={event => patch('sort', event.target.value)}><option value="newest">{t('filters.newest')}</option><option value="oldest">{t('filters.oldest')}</option><option value="title">{t('filters.title')}</option></select></label>
    <label>{t('filters.pageSize')}<select value={values.pageSize} onChange={event => patch('pageSize', event.target.value)}>{[12, 24, 48, 100].map(size => <option key={size} value={size}>{t('filters.items', { count: size })}</option>)}</select></label>
    <div className="button-row"><button>{t('filters.apply')}</button><button type="button" onClick={() => { const defaults = defaultVideoFilters(); setValues(defaults); onApply(defaults); }}>{t('filters.clear')}</button></div>
  </form>;
}
