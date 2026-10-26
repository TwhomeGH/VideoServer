/** ApiError 保留 HTTP 狀態，讓表單能區分登入失效與一般驗證錯誤。 */
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

/** api 集中處理同源 cookie、JSON 標頭與 401；401 會通知 App 回到登入畫面。 */
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  const response = await fetch(`/api/v1/${path}`, {
    credentials: 'same-origin',
    ...init,
    headers,
  });
  const result = await response.json().catch(() => ({}));
  if (!response.ok) {
    if (response.status === 401) window.dispatchEvent(new Event('dvn-unauthorized'));
    const message = typeof result.error === 'string' ? result.error : `服務回應 ${response.status}`;
    throw new ApiError(message, response.status);
  }
  return result as T;
}

/** jsonRequest 建立帶 JSON body 的請求設定。 */
export function jsonRequest(method: string, body: object): RequestInit {
  return { method, body: JSON.stringify(body) };
}
