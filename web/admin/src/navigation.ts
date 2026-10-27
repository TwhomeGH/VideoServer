import { useEffect, useState } from 'react';

/** pages 定義可直接連結的管理分頁；值是翻譯鍵，由畫面語言決定顯示文字。 */
export const pages = {
  overview: 'page.overview', monitor: 'page.monitor', videos: 'page.videos', discovered: 'page.discovered', pools: 'page.pools', import: 'page.import', jobs: 'page.jobs', settings: 'page.settings', api: 'page.api', diagnostics: 'page.diagnostics',
} as const;

/** Page 是管理介面支援的分頁識別。 */
export type Page = keyof typeof pages;

function currentPage(): Page {
  const value = window.location.hash.slice(1);
  return Object.hasOwn(pages, value) ? value as Page : 'overview';
}

/** usePage 追蹤網址分頁，支援重新整理、書籤與瀏覽器上一頁。 */
export function usePage(): Page {
  const [page, setPage] = useState<Page>(currentPage);
  useEffect(() => {
    const navigate = () => { setPage(currentPage()); window.scrollTo(0, 0); };
    window.addEventListener('hashchange', navigate);
    return () => window.removeEventListener('hashchange', navigate);
  }, []);
  return page;
}
