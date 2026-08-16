# 常見問題 FAQ

**Q：`.claude/` 裡本來就有我自己的檔案，會被刪掉嗎？**
不會。agsy 只刪它自己建立的連結；真實目錄或檔案一律報錯請你手動處理，絕不代刪。

**Q：AI 工具直接改了掛載的檔案，我的來源被污染了嗎？**
沒有。改動實際落在 `.agsy/` 產物層，來源不受影響。
想留就 `agsy promote` 寫回；不想留，下次 `agsy apply` 重建就沒了。

**Q：AI 幫我「新寫」了一個檔案（不是改既有的），它會被保留嗎？**
不會——這是目前最重要的注意事項。新增的檔案不在 manifest 帳上，
status 看不見、下次 apply 會直接消失。想留請手動把它搬進來源資料夾。

**Q：兩個來源有同名檔案（共用庫和專案都有 `python-style.md`）怎麼辦？**
由你在 init 時選策略：兩份都留並加來源標記（rename）、只留優先級高的（first）、
或報錯請你手動處理（error）。skills 建議用 error——兩個相似 skill 並存會讓觸發不可預測。

**Q：`.agsy/` 要進版控嗎？**
不用，它是可重建產物（init 會幫你加進 `.gitignore`）。
`agsy.yaml` 則**應該**進版控——隊友 clone 後跑 `agsy apply` 就能長出相同掛載。

**Q：Windows 需要開發人員模式或系統管理員權限嗎？**
不用。agsy 在 Windows 用 junction 掛載，一般帳號就夠。

**Q：專案搬到別的路徑後連結壞了？**
重跑 `agsy apply` 即可（Windows junction 存絕對路徑，搬家後需要重建；
macOS / Linux 用相對 symlink 不受影響）。

**Q：status 說掛載正常，但工具還是讀不到我的指令？**
先確認 `.claude/`（等）不是你自己手動建立的真實目錄。
agsy 對每條連結驗證「是連結」且「指向正確位置」，有問題會標 ✘，`agsy apply` 可修復。

**Q：我想掛的工具不在 init 的清單上？**
不用等更新——adapter 只是 init 的出廠範本,不是執行期依賴。
自己在 `agsy.yaml` 的 `mount` 加一段即可，見[設定檔](config.md#mount掛載對應)。

**Q：介面可以顯示中文嗎？**
自動跟隨系統語言（`LANG` 為 zh 開頭顯示繁體中文，其他顯示英文）。
要強制語言：`AGSY_LANG=zh agsy status` 或 `AGSY_LANG=en …`。

**Q：我自己從網頁下載執行檔，Mac 說「無法驗證開發者」？**
瀏覽器下載會被 macOS 隔離（指令列下載不會）。點「完成」（別點「丟到垃圾桶」），
執行 `xattr -d com.apple.quarantine <路徑>` 清除即可——這是未付費簽章的正常結果，不是惡意軟體。

**Q：想回報問題或許願功能？**
到 [GitHub Issues](https://github.com/IngSquared99/agent-sync/issues) 開一張，
附上 `agsy version` 的輸出與重現步驟。
