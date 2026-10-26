import { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';
import { api } from './api';
import { Library } from './Library';
import { Login } from './Login';
import { Settings } from './Settings';
import { Monitor } from './Monitor';
import { pages, usePage, type Page } from './navigation';

type Theme = 'system' | 'light' | 'dark';
type RuntimeConfig = {
  source: string;
  network?: { stateFile: string; rememberEnabled: boolean; previousPort: number; activePort: number; saved: boolean; warning?: string };
  effective: { host: string; preferredPort: number; fallbackPorts: number[] | null; dataDirectory: string; webDirectory: string; autoScan: boolean; scanIntervalSeconds: number; fileStableSeconds: number };
};
type Status = {
  status: string;
  listenAddress: string;
  build: {
    service: string; version: string; commit: string; buildTime: string;
    channel: string; dirty: boolean; protocolVersion: number;
    imageBuildTime: string; imageSourceDigest: string;
  };
};

function initialTheme(): Theme {
  try {
    const value = localStorage.getItem('dvn-theme');
    if (value === 'light' || value === 'dark') return value;
  } catch { /* 無儲存權限時仍可使用系統外觀。 */ }
  return 'system';
}

/** formatTime 將建置時間轉為瀏覽器本地時間；無法解析時保留原始值。 */
function formatTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-TW', { hour12: false });
}

function App() {
  const page = usePage();
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [runtimeConfig, setRuntimeConfig] = useState<RuntimeConfig | null>(null);
  const [configError, setConfigError] = useState('');
  const [nodeId, setNodeId] = useState('');

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try { localStorage.setItem('dvn-theme', theme); } catch { /* 外觀仍適用本次工作階段。 */ }
  }, [theme]);

  useEffect(() => {
    const controller = new AbortController();
    fetch('/api/v1/status', { signal: controller.signal })
      .then(async response => {
        if (!response.ok) throw new Error(`服務回應 ${response.status}`);
        setStatus(await response.json() as Status);
      })
      .catch((reason: Error) => { if (!controller.signal.aborted) setError(reason.message); });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    let active = true;
    fetch('/api/v1/auth/session', { cache: 'no-store' })
      .then(async response => {
        if (!response.ok) throw new Error(`登入狀態回應 ${response.status}`);
        return await response.json() as { authenticated: boolean };
      })
      .then(result => { if (active) setAuthenticated(Boolean(result.authenticated)); })
      .catch(() => { if (active) setAuthenticated(false); });
    return () => { active = false; };
  }, []);

  useEffect(() => {
    const unauthorized = () => setAuthenticated(false);
    window.addEventListener('dvn-unauthorized', unauthorized);
    return () => window.removeEventListener('dvn-unauthorized', unauthorized);
  }, []);

  useEffect(() => {
    if (!authenticated || page !== 'diagnostics') return;
    const controller = new AbortController();
    setConfigError('');
    setRuntimeConfig(null);
    api<RuntimeConfig>('config', { signal: controller.signal })
      .then(setRuntimeConfig)
      .catch((reason: Error) => { if (!controller.signal.aborted) setConfigError(reason.message); });
    return () => controller.abort();
  }, [authenticated, page]);

  async function logout() {
    try { await api('auth/logout', { method: 'POST' }); } catch { /* 已失效的 session 仍應回到登入畫面。 */ }
    setAuthenticated(false);
    setRuntimeConfig(null);
    setConfigError('');
  }

  return <main>
    <header><div><p className="eyebrow">DISTRIBUTED VIDEO NETWORK</p><h1>DVN 管理中心</h1></div>
      <div className="header-actions">
        <label>外觀 <select value={theme} onChange={event => setTheme(event.target.value as Theme)}>
          <option value="system">跟隨系統</option><option value="light">淺色</option><option value="dark">深色</option>
        </select></label>
        {authenticated === true && <button onClick={() => void logout()}>登出</button>}
      </div>
    </header>
    {error && <section><p role="alert">無法連線：{error}。請確認服務已啟動後重新整理。</p></section>}
    {!status && !error && <p role="status">正在取得服務資訊…</p>}
    {authenticated === null && <section><p role="status">正在檢查登入狀態…</p></section>}
    {authenticated === false && <Login service={status?.build.service} version={status?.build.version}
      onAuthenticated={() => setAuthenticated(true)} />}
    {authenticated === true && <>
      <nav className="main-nav" aria-label="管理分頁">
        {(Object.entries(pages) as [Page, string][]).filter(([key]) => !status || status.build.service === 'video-node'
          || key === 'overview' || key === 'monitor' || key === 'settings' || key === 'diagnostics').map(([key, label]) =>
          <a key={key} href={`#${key}`} aria-current={page === key ? 'page' : undefined}>{label}</a>)}
      </nav>
      <div className="page-heading"><h2>{pages[page]}</h2><p className="muted">{status?.build.service || '正在連線'}</p></div>
      {page === 'settings' && <Settings service={status?.build.service} />}
      {page === 'monitor' && <Monitor />}
      {status?.build.service === 'video-node' && <Library page={page} onNodeId={setNodeId} />}
      {status && page === 'overview' && <section><h2>節點服務</h2><p className="badge">服務運作中 · {status.build.service}</p><p>版本 {status.build.version}</p><a href="#diagnostics">查看完整服務資訊</a></section>}
      {status && status.build.service !== 'video-node' && page !== 'overview' && page !== 'monitor' && page !== 'diagnostics' && page !== 'settings' && <section><p>這個分頁僅適用於 Video Node。</p><a href="#overview">返回總覽</a></section>}
      {page === 'diagnostics' && status && <div className="split-layout">
        <section><h2>建置與連線資訊</h2>
          <p className="badge">服務運作中 · {status.build.service}</p>
          <dl>{Object.entries({
            ...(nodeId ? { '節點識別': nodeId } : {}),
            '版本': status.build.version, 'Commit': status.build.commit,
            '程式建置時間': formatTime(status.build.buildTime),
            '鏡像建置時間': !status.build.imageBuildTime || status.build.imageBuildTime === 'unknown' ? '未提供（建置時未寫入版本資訊）'
              : `${formatTime(status.build.imageBuildTime)} · 來源 ${!status.build.imageSourceDigest || status.build.imageSourceDigest === 'unknown' ? '未提供' : status.build.imageSourceDigest.slice(0, 12)}`,
            '發佈通道': status.build.channel,
            '協定版本': status.build.protocolVersion, '未提交變更': status.build.dirty ? '是' : '否',
            '本機監聽': status.listenAddress,
          }).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
        </section>
        <section><h2>目前生效設定</h2>
          {configError ? <p role="alert">{configError}</p> : !runtimeConfig ? <p role="status">正在取得設定…</p> : <dl className="dl-columns">{Object.entries({
            '設定來源': runtimeConfig.source,
            '監聽介面': runtimeConfig.effective.host,
            '優先端口': runtimeConfig.effective.preferredPort,
            '遞補端口': runtimeConfig.effective.fallbackPorts?.join(', ') || '未啟用',
            '資料目錄': runtimeConfig.effective.dataDirectory,
            '網頁目錄': runtimeConfig.effective.webDirectory,
            ...(runtimeConfig.network ? {
              '沿用上次端口': runtimeConfig.network.rememberEnabled ? '啟用' : '停用（固定端口）',
              '上次成功端口': runtimeConfig.network.previousPort || '無記錄',
              '本次實際端口': runtimeConfig.network.activePort,
              '狀態檔': runtimeConfig.network.stateFile,
              '狀態保存': runtimeConfig.network.saved ? '已保存' : '未保存',
              ...(runtimeConfig.network.warning ? { '狀態提醒': runtimeConfig.network.warning } : {}),
            } : {}),
            ...(status.build.service === 'video-node' ? {
              '自動掃描': runtimeConfig.effective.autoScan ? '啟用' : '停用',
              '掃描間隔': `${runtimeConfig.effective.scanIntervalSeconds} 秒`,
              '檔案穩定時間': `${runtimeConfig.effective.fileStableSeconds} 秒`,
            } : {}),
          }).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>}
        </section>
      </div>}
    </>}
  </main>;
}

createRoot(document.getElementById('root')!).render(<App />);
