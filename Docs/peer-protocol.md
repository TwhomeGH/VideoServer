# Peer 握手與能力協商（設計草案）

> 狀態：第一階段（protocol 版本範圍、`Negotiate`、`capabilityVersions`）已實作；register、heartbeat、lease、peer registry 與 catalog 同步尚未實作。現行 Central／Worker 仍沒有註冊、聯邦索引或派工。

## 目的

先固定服務之間「誰是誰、支援什麼、端點在哪、何時失效」的契約，讓之後的 Central 索引、Worker 派工與聯邦同步不必回頭改資料模型與 API。

非目標：本文不定義影片位元組傳輸、轉碼格式、搜尋排名或管理員權限；那些屬資料平面或產品功能。

## 名詞

| 名詞 | 定義 |
| --- | --- |
| serviceType | `video-node`、`central`、`transcode-worker` |
| nodeId | 節點持久識別；Video Node 存在 SQLite，重啟不變 |
| instanceId | 單次程序啟動的隨機識別；用來區分同一 nodeId 的新舊程序 |
| peerId | Central 配發給已註冊 peer 的識別；不由 peer 自行決定 |
| endpointRevision | 端點或網路位置改變時遞增的整數 |
| capability | 帶版本的功能字串，例如 `library.query.v1` |
| lease | Central 授予的存活期限；heartbeat 會延長 |

## 現況與目標差異

現在 `/api/v1/protocol` 回傳 `service`、`serviceType`、`contentTypes`、`protocolVersion`、`supportedVersions`、`capabilities`、OpenAPI 路徑，以及第一階段加入的 `minProtocolVersion`、`maxProtocolVersion` 與 `capabilityVersions`。Video Node 公告 `contentTypes: ["video"]`；Image／Post 會各自公告自己的內容類型。目前 `MinSupported`／`MaxSupported` 都是 1，所以實際仍只接受 v1；`Negotiate` 已預留版本範圍。Central／Worker 與 Video Node 各自啟動，不會互相發現。

目標是讓 Central 成為 **控制平面的 peer registry**：Node／Worker 註冊、回報能力與端點、定期 heartbeat；Central 只把 catalog／變更摘要納入索引，不代理所有影片流量。Video Node 在 Central 不在時仍可獨立運作。

## 契約原則

1. **控制平面與資料平面分離**：握手只交換身分、能力、端點、lease 與 catalog 游標；影片內容仍由 Node 的資料端點提供。
2. **版本範圍而非精確版本**：peer 宣告 min/max，Central 選交集；不能只靠 `Current`。
3. **能力可選且帶版本**：缺少可選能力時降級，不讓整個握手失敗；必要能力不足才拒絕。
4. **冪等重試**：register/heartbeat/更新必須可重送；Central 以 `(serviceType,nodeId,instanceId)` 去重。
5. **本地優先**：注冊、heartbeat 或 Central 失敗不阻擋 Video Node 的管理、掃描、播放與本機索引。
6. **不信任網路位置**：LAN 不是授權；peer 必須有獨立憑證，不能重用管理員密碼。
7. **租約而非永久在線**：Central 以 lease 判斷在線；過期只標離線，不急著刪除資料。
8. **可重播的 catalog 變更**：Central 需要的是 revision／cursor／idempotent event，不是原始檔案系統事件。

## 版本與能力協商

### Protocol

- `/api/v1/protocol` 增加 `minProtocolVersion`、`maxProtocolVersion`；`supportedVersions` 保留相容欄位。
- 每個請求仍帶 `X-DVN-Protocol-Version`；peer 在 register 時帶 `protocol: {min, max}`。
- Central 選擇交集最高版本；沒有交集時回 `426 Upgrade Required`（或既有 `409` 的相容錯誤碼），附 `supportedVersions`。
- 同一 major 內的 additive 欄位可忽略；breaking change 必須升 major 或新路徑。

### Capability

能力字串格式：`名稱.vN`。第一階段先保留既有 `capabilities` 名稱清單以維持相容，另回傳 `capabilityVersions` 映射（例如 `library.query: [1]`）；未來新增能力可使用 `名稱.vN`，舊 client 忽略未知欄位即可。

第一批建議：

| 能力 | 說明 |
| --- | --- |
| `auth.session.v1` | 管理 session |
| `config.overrides.v1` | 設定覆寫 |
| `system.restart.v1` | 重啟 |
| `monitor.resources.v1` | 資源監看 |
| `library.query.v1` | 影片查詢 |
| `video.range.v1` | Range 播放 |
| `video.probe.v1` | 技術資訊重新探測 |
| `jobs.import.v1` | 背景匯入 |
| `catalog.sync.v1` | 影片 catalog 變更摘要（規劃） |
| `watch.events.v1` | 節點側檔案監聽狀態（規劃，不上傳原始 OS 事件） |

- `requiredCapabilities` 與 `optionalCapabilities` 分開；Central 可拒絕缺少必要能力，或降級缺少可選能力。
- 未知能力忽略；同一能力多版本時取最高相容版本。
- 管理頁與 diagnostics 顯示「宣告能力」與「協商後能力」，避免把公告當成已連線。

## 身分與端點

```json
{
  "serviceType": "video-node",
  "nodeId": "6f...",
  "instanceId": "9c...",
  "endpointRevision": 3,
  "protocol": { "min": 1, "max": 2 },
  "capabilities": ["library.query.v1", "video.range.v1", "jobs.import.v1"],
  "endpoints": {
    "api": "https://node.example:18610",
    "health": "https://node.example:18610/healthz"
  },
  "build": { "version": "0.1.0", "commit": "...", "dirty": false }
}
```

- `nodeId` 持久；`instanceId` 每次啟動重新產生。`endpointRevision` 在 listen 位址或 port 改變時遞增。
- 端點必須是 peer 可達的位址；不能直接回 `0.0.0.0`。Docker 需回主機／對外位址或由部署層設定。
- Central 回傳 `peerId`、`leaseTtlSeconds`、`heartbeatIntervalSeconds`、協商後的 protocol 與能力，以及 Central 自己的端點。
- 同一 nodeId 出現新 instance 時，舊 instance 標為 superseded；若舊 instance 仍在 heartbeat，回 `409 Conflict`，由管理員決定接管，避免 split brain。

## 註冊、heartbeat 與 lease

建議流程：

1. `POST /api/v1/peers/register`：帶身分、端點、版本範圍、能力與 peer 憑證。
2. Central 驗證、配發 `peerId` 與 lease，回傳協商結果。
3. `POST /api/v1/peers/{peerId}/heartbeat`：延長 lease；可附端點／能力更新（需遞增 revision）。
4. `DELETE /api/v1/peers/{peerId}`：正常下線。
5. Central 定期掃描 lease；過期標 `offline`，保留 peer 資料與歷史；重新註冊可恢復。

- Heartbeat 間隔建議 `leaseTtlSeconds / 3`；Central 以**收到時間**計算 lease，不信任 peer 時鐘。
- 連續失敗採 exponential backoff + jitter；不阻塞本地工作。
- Register 需 idempotent：同一 `(nodeId,instanceId)` 重送回同一 peer；不同 instance 走接管規則。
- Endpoint/capability 更新要有 `endpointRevision` 或 `capabilityRevision`，避免舊請求覆蓋新狀態。

## 認證與授權

- 管理員 session 與 peer 認證**分開**；peer 不可用管理員密碼。
- 第一版可用 bootstrap token（環境變數／檔案）換取 peer session token；正式環境建議 mTLS。
- Token 需有 service scope、效期、輪替與撤銷；日誌不得輸出完整 token。
- 寫入請求加 timestamp + nonce 或 request signature，降低重放；Central 對 peer 端點做 rate limit。
- TLS 由部署層終止時，Central 需能驗證轉發來源或使用 mTLS；不能只靠來源 IP。

## 失敗與邊界

| 情境 | 行為 |
| --- | --- |
| Central 不在 | Node 本地照常運作；註冊／heartbeat 重試；管理頁顯示最後錯誤 |
| Node 不在 | Central 保留 catalog 與 peer 記錄；lease 過期標離線，不刪影片索引 |
| 網路分區 | 兩邊各自運作；恢復後重新註冊／heartbeat；以 revision 與 cursor 補齊 |
| 重複 instance | 舊 instance 標 superseded；仍有 heartbeat 時回 409，避免同時宣稱同一 node |
| 版本不相容 | 回 426/409，附支援版本；不建立半套 peer |
| 缺少必要能力 | 拒絕或標 degraded；不派發該 peer 不支援的工作 |
| 端點過期 | Central 使用最新 revision；舊端點不再派工 |
| 時鐘偏差 | lease 由 Central 計算；peer 只使用回傳的 interval |
| 重送／重放 | Idempotency key／nonce；register、catalog 事件與派工皆需冪等 |

## 與 catalog／檔案監聽的邊界

Central 不應直接消費 OS 的 create/write/delete 事件。Node 先把 watcher 與 reconciliation 的結果整理成 **catalog 變更**：

- 每個 Node 有單調遞增的 `catalogRevision`。
- 變更事件包含 `eventId`、`revision`、`kind`（upsert/remove/rename）、video ID、hash、metadata 摘要與時間。
- Central 以 cursor 拉取或 Node 推送；支援重播、ack、去重與 snapshot + tail。
- 原始檔案系統事件、暫存檔、debounce 細節留在 Node；Central 只看穩定的 catalog。
- 這部分等 Central 索引階段再實作；握手契約先保留 `catalog.sync.v1` 能力。

## 可觀測性

- `/api/v1/protocol`：本服務版本範圍、能力、OpenAPI。
- Node：握手狀態（未註冊／等待／已連線／錯誤）、Central 位址、lease 剩餘、最後 heartbeat、協商能力。
- Central：peer 清單、serviceType、nodeId、instanceId、endpointRevision、lease、最後錯誤、catalog cursor。
- 日誌只記 peerId／nodeId／狀態，不記 token。

## 分階段落地

1. **契約與版本**：本文件、protocol 範圍欄位、能力版本化；不改實際註冊。
2. **Registry 與 heartbeat**：Central peer 表、register/heartbeat/deregister、Node 客戶端、管理頁 peer 狀態。
3. **能力與端點更新**：revision、degraded、peer 端點選擇；仍不搬影片。
4. **Catalog 同步**：revision/cursor/事件重播，接上檔案監聽與 Central 索引。
5. **Worker 派工**：Worker 註冊、能力／政策比對、lease 與工作狀態；最後才談跨節點排程。

## 待決策

- Peer token 第一版用 bootstrap token 還是直接 mTLS。
- 同一 nodeId 新 instance 的接管是否需要管理員確認。
- Catalog 用 Node push 還是 Central pull；是否需要 SSE/長輪詢。
- Central 回傳的端點是否允許私有位址與多個候選。
- 能力版本是否強制 `vN` 後綴，或先保留現有無版本字串並新增 `capabilityVersions`。
