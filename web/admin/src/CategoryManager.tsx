import { useState } from 'react';
import { api } from './api';
import { useI18n } from './i18n';

export type Category = { id: string; name: string; videoCount: number };

/** CategoryManager 管理分類名稱；影片仍以 videos.category 保存名稱。 */
export function CategoryManager({ categories, onChanged }: { categories: Category[]; onChanged: () => void }) {
  const { t } = useI18n();
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const run = async (action: () => Promise<unknown>) => {
    setBusy(true);
    setError('');
    try {
      await action();
      onChanged();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  };

  const rename = (category: Category, next: string) => {
    if (next.trim() === category.name) return;
    void run(() => api(`categories/${category.id}`, { method: 'PATCH', body: JSON.stringify({ name: next }) }));
  };

  return <div className="category-manager">
    <p className="muted">{t('categories.intro')}</p>
    <form className="button-row" onSubmit={event => { event.preventDefault(); void run(async () => {
      await api('categories', { method: 'POST', body: JSON.stringify({ name }) });
      setName('');
    }); }}>
      <input required maxLength={80} value={name} onChange={event => setName(event.target.value)} placeholder={t('categories.namePlaceholder')} />
      <button disabled={busy}>{t('categories.add')}</button>
    </form>
    {error && <p role="alert">{error}</p>}
    <ul className="category-list">
      {categories.map(category => <CategoryRow key={category.id} category={category} busy={busy} onRename={next => rename(category, next)} onDelete={() => {
        if (window.confirm(t('categories.deleteConfirm', { name: category.name }))) {
          void run(() => api(`categories/${category.id}?confirm=true`, { method: 'DELETE' }));
        }
      }} />)}
    </ul>
    {categories.length === 0 && <p className="muted">{t('categories.empty')}</p>}
  </div>;
}

function CategoryRow({ category, busy, onRename, onDelete }: { category: Category; busy: boolean; onRename: (name: string) => void; onDelete: () => void }) {
  const { t } = useI18n();
  const [name, setName] = useState(category.name);
  return <li>
    <input maxLength={80} value={name} onChange={event => setName(event.target.value)} aria-label={t('categories.renameAria', { name: category.name })} />
    <span className="muted">{t('categories.count', { count: category.videoCount })}</span>
    <button type="button" disabled={busy || name.trim() === category.name} onClick={() => onRename(name)}>{t('categories.rename')}</button>
    <button type="button" disabled={busy} onClick={onDelete}>{t('categories.delete')}</button>
  </li>;
}
