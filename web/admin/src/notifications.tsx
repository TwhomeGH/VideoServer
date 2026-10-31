import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useI18n } from "./i18n";

/** 單一訊息編號和內文 */
type Toast = {
    id: number;
    message: string;
    tone: Tone
}

/** 通知語氣；success／info 短暫顯示，error 停留較久 */
type Tone = 'success' | 'error' | 'info'

/** 全域通知 API */
type Notify = {
    /** 顯示成功通知 */
    success: (message: string) => void;
    /** 顯示錯誤通知 */
    error: (message: string) => void;
    /** 顯示一般資訊通知 */
    info: (message: string) => void;
};

/** 保存目前通知 API；不在 Provider 內時為 null */
const NotificationContext = createContext<Notify | null>(null);

/**
 * 提供全域 toast 通知，讓任何頁面都能呼叫 success／error／info。
 * 必須包在應用程式最外層。
 */

export function NotificationProvider({ children }: { children:ReactNode}) {
    // 儲存 toast 陣列/計時器/dimiss

    const { t } = useI18n();
    const [toasts, setToasts] = useState<Toast[]>([]);
    
    // 建立多組訊息計時器
    const timers = useRef(new Map<number,number>)
    // 下一組訊息ID
    const nextId = useRef(0)

    /** 關閉通知 */
    const dismiss = useCallback( (id: number) => {
        const timer = timers.current.get(id);

        
        if (timer != undefined) {
            window.clearTimeout(timer)
            timers.current.delete(id)
        }

        // 設置通知訊息
        setToasts(current => current.filter(item => item.id !== id ))
    }, [])

    /** 發送通知 */
    const push = useCallback( (message:string, tone:Tone) => {
        const id = ++nextId.current;
        setToasts(current => [...current, { id, message, tone} ]);
        
        // 如果是錯誤增長通知顯示時間 預設5秒 其他3秒
        const ttl = tone === "error" ? 5000 : 3000;

        timers.current.set(id,window.setTimeout( () => dismiss(id), ttl));

    },[dismiss])


    // 處理通知完成清空計時器
    useEffect( () => () => {

        for (const timer of timers.current.values()) window.clearTimeout(timer);
            timers.current.clear();

    },[])


    // 主通知狀態回報
    const notify = useMemo<Notify>(() => (
        {
            success: message => push(message,'success'),
            error: message => push(message,'error'),
            info: message => push(message,'info'),
        }
    ) ,[push]);
    

    return (
        <NotificationContext.Provider value={notify}>
            {children}
            <div className="toast-stack" aria-live="polite">
                {
                    toasts.map(item => (
                        <div 
                            key={item.id} 
                            className={`toast toast-${item.tone}`}
                            role={item.tone === 'error' ? 'alert' : 'status'}
                        >
                        <span>{item.message}</span>
                        <button
                            type="button"
                            aria-label={t('notifications.dismiss')}
                            onClick={()=> dismiss(item.id)}
                        >
                            ×
                        </button>   
                        </div>

                    ))
                }
                

            </div>
        </NotificationContext.Provider>
    )
}

/**
 * 取得全域通知 API。
 * @throws 在 NotificationProvider 外使用時拋出錯誤。
 */

export function useNotify() {
    // 回傳 { success , error, info }
    const value = useContext(NotificationContext);

    if(!value) throw new Error('useNotify 必須在 NotificationProvider 內使用')

    return value

}