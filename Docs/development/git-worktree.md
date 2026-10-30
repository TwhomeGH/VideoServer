# Git worktree：同一個 repo，多個工作目錄

`git worktree` 讓同一個 Git repository 同時有多個工作目錄。每個目錄可以有不同分支或 commit，
但它們共用同一份 `.git` 物件庫與歷史，不需要像 `git clone` 一樣複製整個 repo。

## 與 clone 的差別

| | `git clone` | `git worktree` |
| --- | --- | --- |
| 物件庫 | 另一份 | 共用原本的 `.git` |
| 歷史同步 | 要 fetch／push | 共用，兩邊看到同一批 commit |
| 適合 | 獨立倉庫、完全不同 remote | 同一專案的多分支／舊 commit |
| 磁碟 | 較大 | 較小，但 `node_modules` 等仍需各自安裝 |

## 常用指令

```sh
# 看目前有哪些 worktree
git worktree list

# 從既有分支或 commit 開一個新目錄
git worktree add ../my-project-old <commit>

# 同時建立新分支（推薦要修改時使用）
git worktree add -b tutorial/01-copy-id ../my-project-old 3b9e35b^

# 分離 HEAD，只讀或暫時實驗（不要直接在 detached HEAD 上長期開發）
git worktree add --detach ../my-project-old c2a03ad

# 移除 worktree；先確認沒有未提交的重要變更
git worktree remove ../my-project-old

# 手動刪除目錄後，清掉 Git 的中繼資料
git worktree prune
```

## 這個專案的使用情境

- 從舊 commit 重做 tutorial，不影響目前工作區。
- 一邊保留 feature 改動，一邊開另一份 main 重現 bug。
- 比較兩個版本的 UI／API，不必一直 stash／checkout。
- 在獨立目錄跑舊版測試，避免污染目前 `local/development` 或 Docker volume。

範例：練習 Level 1。

```powershell
git worktree add -b tutorial/01-copy-id ..\VideoServer-tutorial-01 3b9e35b^
cd ..\VideoServer-tutorial-01
npm --prefix web/admin ci
$env:DVN_ADMIN_PASSWORD = 'tutorial-password-123'
npm run dev
```

Linux／macOS：

```sh
git worktree add -b tutorial/01-copy-id ../VideoServer-tutorial-01 3b9e35b^
cd ../VideoServer-tutorial-01
npm --prefix web/admin ci
DVN_ADMIN_PASSWORD=tutorial-password-123 npm run dev
```

## 注意事項

- **同一個分支不能同時 checkout 在兩個 worktree**；要修改就建新分支，或使用 detached HEAD。
- 每個 worktree 有各自的未提交變更、`node_modules`、建置產物與 `local/development`；不是共用工作檔。
- 不要把 worktree 建在 repository 內部；放在隔壁目錄最容易管理，也避免被掃描或提交。
- 兩個 worktree 同時跑 `npm run dev` 會搶 5173／18610 等端口；一次只啟動一組，或改端口。
- Docker Compose 的 project name、volume 與 ports 也可能衝突；不要在兩份目錄同時起同一組 testing stack。
- 手動刪掉 worktree 目錄不會自動清 Git 中繼資料，要跑 `git worktree prune`。
- worktree 內的 commit 仍在同一個 repo；完成後可用一般 `git push`，不要以為它是另一個 clone。

## 清理

```sh
cd F:\VideoServer
git worktree remove ..\VideoServer-tutorial-01
git branch -D tutorial/01-copy-id   # 只有確定不要保留分支時才刪
git worktree prune
```

**不要用 `git reset --hard` 清掉目前工作區**；worktree 的目的就是避免這種操作。
