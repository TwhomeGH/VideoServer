import { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import './style.css';
import { Library } from './Library';
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
  };
};

function initialTheme(): Theme {
  try {
    const value = localStorage.getItem('dvn-theme');
    if (value === 'light' || value === 'dark') return value;
  } catch { /* 無儲存權限時仍可使用系統外觀。 */ }
  return 'system';
}

function App() {
  const page = usePage();
  const [theme, setTheme] = useState<Theme>(initialTheme);
  const [status, setStatus] = useState<Status | null>(null);
  const [error, setError] = useState('');
  const [runtimeConfig, setRuntimeConfig] = useState<RuntimeConfig | null>(null);
  const [configError, setConfigError] = useState('');

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
    if (page !== 'diagnostics') return;
    const controller = new AbortController();
    setConfigError('');
    fetch('/api/v1/config', { signal: controller.signal }).then(async response => {
      if (!response.ok) throw new Error(`設定資訊回應 ${response.status}，請確認後端已更新並重啟`);
      setRuntimeConfig(await response.json() as RuntimeConfig);
    }).catch((reason: Error) => { if (!controller.signal.aborted) setConfigError(reason.message); });
    return () => controller.abort();
  }, [page]);

  return <main>
    <header><div><p className="eyebrow">DISTRIBUTED VIDEO NETWORK</p><h1>DVN 管理中心</h1></div>
      <label>外觀 <select value={theme} onChange={event => setTheme(event.target.value as Theme)}>
        <option value="system">跟隨系統</option><option value="light">淺色</option><option value="dark">深色</option>
      </select></label>
    </header>
    <nav className="main-nav" aria-label="管理分頁">
      {(Object.entries(pages) as [Page, string][]).filter(([key]) => !status || status.build.service === 'video-node' || key === 'overview' || key === 'diagnostics').map(([key, label]) =>
        <a key={key} href={`#${key}`} aria-current={page === key ? 'page' : undefined}>{label}</a>)}
    </nav>
    <div className="page-heading"><h2>{pages[page]}</h2><p className="muted">{status?.build.service || '正在連線'}</p></div>
    {error && <section><p role="alert">無法連線：{error}。請確認服務已啟動後重新整理。</p></section>}
    {!status && !error && <p role="status">正在取得服務資訊…</p>}
    {status?.build.service === 'video-node' && <Library page={page} />}
    {status && page === 'overview' && <section><h2>節點服務</h2><p className="badge">服務運作中 · {status.build.service}</p><p>版本 {status.build.version}</p><a href="#diagnostics">查看完整服務資訊</a></section>}
    {status && status.build.service !== 'video-node' && page !== 'overview' && page !== 'diagnostics' && <section><p>這個分頁僅適用於 Video Node。</p><a href="#overview">返回總覽</a></section>}
    {page === 'diagnostics' && status && <section><h2>建置與連線資訊</h2>
        <p className="badge">服務運作中 · {status.build.service}</p>
        <dl>{Object.entries({
          '版本': status.build.version, 'Commit': status.build.commit,
          '建置時間': status.build.buildTime, '發佈通道': status.build.channel,
          '協定版本': status.build.protocolVersion, '未提交變更': status.build.dirty ? '是' : '否',
          '本機監聽': status.listenAddress,
        }).map(([label, value]) => <div key={label}><dt>{label}</dt><dd>{value}</dd></div>)}</dl>
    </section>}
    {page === 'diagnostics' && status && <section><h2>目前生效設定</h2>
      {configError ? <p role="alert">{configError}</p> : !runtimeConfig ? <p role="status">正在取得設定…</p> : <dl>{Object.entries({
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
    </section>}
  </main>;
}

createRoot(document.getElementById('root')!).render(<App />);
