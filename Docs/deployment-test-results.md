# Docker 部署驗收紀錄

日期：2026-10-27（Asia/Taipei）

環境：Windows 主機、Docker Desktop Linux Engine、linux/amd64 image。
執行入口：`npm run test:deployment`。本次從原始碼在本機建置，不經 CI。

| 項目 | 結果 |
|---|---|
| 本機建置 Video Node 與打包前端 | 通過 |
| 生成兩秒 H.264/AAC MP4 測試素材 | 通過 |
| Compose 外掛 JSON 設定、唯讀影片掛載與資料卷 | 通過 |
| 主機映射端口讀取 API、HTML 與 JavaScript | 通過 |
| 註冊 /media 並匯入中文檔名 | 通過 |
| SHA-256、Tag 去重與原作者資料保存 | 通過 |
| Range 206、HEAD 200 與完整檔案內容比對 | 通過 |
| 編輯資料與外掛 config 指定資料庫子目錄 | 通過 |
| 強制重建容器並驗證 NodeID、儲存池與影片資料保留 | 通過 |
| 來源離線後保留索引、播放回應 503，恢復後重新可用 | 通過 |

本次測試專案：`dvn-test-1793037058315`。
報告與範例資料：`data/deployment-tests/dvn-test-1793037058315/result.json`。
測試容器已停止，資料卷保留供檢查；未使用現有影片庫。

範圍限制：尚未驗證瀏覽器實際解碼／播放器互動、ARM64、原生 Linux systemd 安裝、NAS 斷線與權限、真實網路環境及轉碼。HTTP Range 傳輸通過不代表所有影片編碼均可在瀏覽器播放。
