import { useState } from 'react';

/** VideoFilterValues 保存已套用的查詢條件，送出表單才更新清單。 */
export type VideoFilterValues = { q: string; tag: string; category: string; poolId: string; adult: string; sort: string; pageSize: string };

/** defaultVideoFilters 回傳不篩選、每頁 24 筆的獨立查詢設定。 */
export function defaultVideoFilters(): VideoFilterValues {
  return { q: '', tag: '', category: '', poolId: '', adult: '', sort: 'newest', pageSize: '24' };
}

/** VideoFilters 提供文字與內容標記篩選；onApply 回傳條件並由父層重設頁碼。 */
export function VideoFilters({ pools, initialValues, onApply }: {
  pools: { id: string; name: string }[]; initialValues: VideoFilterValues; onApply: (values: VideoFilterValues) => void;
}) {
  const [values, setValues] = useState(initialValues);
  const patch = (field: keyof VideoFilterValues, value: string) => setValues(current => ({ ...current, [field]: value }));
  return <form className="video-filters" onSubmit={event => { event.preventDefault(); onApply(values); }}>
    <label>搜尋標題與簡介<input maxLength={200} value={values.q} onChange={event => patch('q', event.target.value)} placeholder="輸入關鍵字" /></label>
    <label>Tag<input maxLength={200} value={values.tag} onChange={event => patch('tag', event.target.value)} placeholder="完整標記名稱" /></label>
    <label>分類<input maxLength={80} value={values.category} onChange={event => patch('category', event.target.value)} placeholder="完整分類名稱" /></label>
    <label>儲存池<select value={values.poolId} onChange={event => patch('poolId', event.target.value)}><option value="">所有儲存池</option>{pools.map(pool => <option key={pool.id} value={pool.id}>{pool.name}</option>)}</select></label>
    <label>內容標記<select value={values.adult} onChange={event => patch('adult', event.target.value)}><option value="">全部</option><option value="true">已標示 18+</option><option value="false">未標示 18+</option></select></label>
    <label>排序<select value={values.sort} onChange={event => patch('sort', event.target.value)}><option value="newest">最新匯入</option><option value="oldest">最早匯入</option><option value="title">標題</option></select></label>
    <label>每頁<select value={values.pageSize} onChange={event => patch('pageSize', event.target.value)}>{[12, 24, 48, 100].map(size => <option key={size} value={size}>{size} 筆</option>)}</select></label>
    <div className="button-row"><button>套用查詢</button><button type="button" onClick={() => { const defaults = defaultVideoFilters(); setValues(defaults); onApply(defaults); }}>清除條件</button></div>
  </form>;
}
