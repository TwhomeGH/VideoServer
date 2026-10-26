import { useEffect, useState } from 'react';

/** pages 定義可直接連結的管理分頁；hash 路由支援既有靜態服務。 */
export const pages = {
  overview: '總覽', videos: '影片庫', pools: '儲存池', import: '匯入影片', jobs: '匯入工作', settings: '設定', diagnostics: '服務資訊',
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
