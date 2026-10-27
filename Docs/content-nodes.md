# 內容節點：Video、Image、Post

## 原則

Video Node、Image Node、Post Node 是不同服務，不把圖片與文章塞進現有 `videos` 資料表。

Central 只保存聯邦索引與內容參照，不代理所有內容流量：

```text
contentRef {
  contentType: video | image | post
  nodeId
  itemId
  revision
}
```

## 服務與能力

| 服務 | serviceType | 核心能力 | 是否使用儲存池 |
| --- | --- | --- | --- |
| Video Node | `video-node` | 影片索引、FFprobe、Range 播放、轉碼 | 是 |
| Image Node | `image-node` | 圖片索引、縮圖、EXIF、尺寸與格式 | 是；可重用掃描政策 |
| Post Node | `post-node` | 文字／Markdown 文章、標籤、來源與授權 | 通常不需要 |

能力以 `capabilities`／`capabilityVersions` 公告，例如：

- `library.video`
- `library.image`
- `library.post`
- `storage.pool`
- `scan.policy`
- `media.probe.video`
- `media.thumbnail`
- `media.exif`
- `content.authoring`
- `transcode.video`

前端依能力隱藏不適用頁面，不依服務名稱硬寫所有分支。

## Image Node

- 與 Video Node 共用儲存池、volume GUID、掃描政策與檔案穩定觀察。
- 不共用 `video_media` 或 FFprobe；改存圖片尺寸、格式、EXIF、縮圖與可選的內容雜湊。
- 可先支援 JPEG、PNG、WebP、AVIF、GIF；動畫格式是否視為圖片或影片需另外定義。
- 等到 Image Node 真正開始時，再把 storage／scan 從 Video Node 抽成共用套件；現在不急著通用化影片資料模型。

## Post Node

- 文章是 authored content，不是檔案掃描結果；通常不需要儲存池。
- 以 Markdown／結構化欄位、標籤、來源、授權與版本為核心。
- 可引用 Image／Video Node 的 `contentRef`，但不把媒體檔複製進文章資料庫。
- 搜尋與索引由 Post Node 自己維護，Central 只彙整 federated result。

## Central 與聯邦

- 節點註冊時公告 serviceType、能力版本、endpoint revision。
- Central 的搜尋結果帶 contentType、nodeId、itemId；點開後由原節點提供內容。
- 影格、縮圖、文章與播放流量不經過 Central。
- 節點認證使用 peer token／未來的 mTLS，不重用管理員密碼。
- 多節點上線前，影片節點仍維持目前的單機管理與本機授權。

## 建議順序

1. 完成儲存池掃描政策、格式能力與全盤安全預設。
2. 做檔案 watcher 與 file identity，降低掃描延遲。
3. 先做 Image Node，重用 storage／scan，驗證第二種媒體節點。
4. 再做 Post Node；它與檔案掃描的耦合最低。
5. 最後接 Central 聯邦索引、peer 認證與跨節點搜尋。
