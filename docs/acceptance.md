# Windowsでの受入確認

[要件定義書の受入条件](requirements.md#6-受入条件)を、Goの自動テストとビルド済み実行ファイルの確認へ対応付ける。要件文書は元の定義を保持し、開発状況は[開発計画](development-plan.md)で管理する。

## 再実行

プロジェクトのルートで `./scripts/verify.ps1` を実行する。整形、静的検査、全Goテスト、Windowsビルド、ヘルプ・版表示に続き、`scripts/acceptance.ps1` が実行ファイルを確認する。ビルド済みの実行ファイルを確認する場合は `./scripts/acceptance.ps1` を単独で実行できる。

受入スクリプトは、自分で作った一時リポジトリだけを操作する。日本語・空白を含む通常の作業ディレクトリと追加のworktreeを作り、それぞれの配下からCLIを起動する。対象ファイル、インデックスの実体、HEAD、共通Git設定、worktree固有設定、worktreeの `.git` ファイル、開始記録のSHA-256を操作前後で比較する。開始記録の作成・明示した取り直し以外は、記録の変更を認めない。検証データは実行ごとに作り、終了時に除去する。

## 要件と検査の対応

| 受入条件 | 自動テストと実行ファイルでの確認 |
| --- | --- |
| 1. 開始前の未コミット変更を区別 | `TestCheckSeparatesPreexistingChangesAndReediting`。受入スクリプトでは追跡済み・未追跡の開始前変更を残したまま、開始後の変更が空で終了コード0となることを両作業ディレクトリで確認する |
| 2. 同じ変更済みファイルの再編集を検出 | 同テストで、Git状態の印が同じでもハッシュの変化を確認する。受入スクリプトでは `config.toml` の再編集を範囲外として報告し、インデックスの変化と分けて確認する |
| 3. ディレクトリの配下と似た名前の別パスを区別 | `TestCheckChangeKindsAndDirectoryBoundaries`、`TestCheckFileAllowDoesNotExpandAfterTypeChange`。受入スクリプトでは `Src` 内の追加・変更・削除と未作成の許容パスの配下を終了コード0、`Src2` の変更を1として確認する |
| 4. 空白・日本語・改行などを保持 | `TestWindowsCheckAllowHandlesCaseAndSeparators`、`TestWindowsAllowUsesActualNames`。受入スクリプトで日本語・空白・大小文字・区切りを実ファイルで確認する。改行・タブは `TestGitParsersPreserveWhitespaceAndNewlines` と `TestComparisonPreservesControlCharactersThroughJSON` の解析用データで、複数パスの分割、比較、範囲判定、JSONへの往復を確認する |
| 5. 判定不能を成功扱いにしない | `TestWindowsUnreadableFileNeverRecordsSuccess`、`TestWindowsCheckKeepsOutOfScopeChangesWithUnreadableFile`、`TestCheckRejectsMissingAndInvalidBaseline`、`TestCheckUnknownAndOutOfScopeCoexist`、`TestBeginSubmoduleNeverReportsCompleteSuccess`。受入スクリプトではHEAD変更と範囲外変更の併記、破損記録を終了コード2として確認する |
| 6. 外部リンク先を走査せずリンク自体を比較 | `TestBeginDoesNotFollowLinks`、`TestCheckLinkTargetChangesWithoutFollowingContents`、`TestCheckReportsNewParentLinkAlongsideUnreadableDescendant`。実際のシンボリックリンクを使い、リンク先内容の変更が差分に現れず、参照先文字列の変更が現れることを確認する。`TestWindowsJunctionIsNeverTraversed` はジャンクションを対象外として停止することを確認する |
| 7. 対象・Git状態・開始記録を許可なく変更しない | `TestBeginRecordsContentIndexAndGitStateWithoutChangingInputs`、`TestBeginProtectsExistingRecordAndReplacesExplicitly`、`TestWindowsFailedReplacementPreservesBaseline`。受入スクリプトで通常・worktree両方の `begin`、`check`、ヘルプ、版表示、引数エラー、既存記録の拒否、明示した取り直し、破損記録の拒否について対象とGit状態を比較し、許可しない記録更新も検出する |
| 8. 配列の安定順・JSON・終了コード | `TestCheckSeparatesPreexistingChangesAndReediting`、`TestCheckChangeKindsAndDirectoryBoundaries`、`internal/cli/run_test.go` のJSON・日本語・出力失敗テスト。受入スクリプトで同一状態の出力全体の一致、固定項目と配列、標準出力のJSONオブジェクト1件、標準エラー、終了コード0・1・2・64を確認する |
| 追加要件9. タスク名ごとの開始記録 | `TestTaskRecordsCoexistAndSelectIndependentBaselines`、`TestTaskRecordsCanPrecedeDefaultRecord`、`TestTaskNamesRejectUnsafeAndAmbiguousArguments`、`TestTaskRecordsProtectMismatchedAndUnreadableState`、保存時の競合テスト、`TestWindowsFailedReplacementPreservesBaseline`、CLIのタスク引数・表示テスト。受入スクリプトで従来の記録と複数タスクの併存、開始時点・許容範囲、未記録名と上書きの拒否、選択した記録だけの取り直しを通常・worktreeの両方で確認する |

## タスク別記録の確認結果（2026-10-26）

- 基点は `3c47a99`。タスク別記録の製品コード、テスト、受入スクリプトと文書を加えたソースを検証した。CLI版は機能追加により `0.3.1` から `0.4.0` へ更新し、保存・JSON形式は `schema_version: 1` を維持した。
- `checkstamp run --label task-records --input go.mod --input cmd --input internal --input scripts --input .github --json -- pwsh -NoProfile -File ./scripts/verify.ps1` が成功した。実行IDは `efb3a005b5436b4a1bdea6aa804805cc`。同じ作業ディレクトリで `checkstamp status` が `valid`、子プロセスの終了コードが0であることを確認した。記録は除外済みの `.checkstamp/` に置き、コミットしない。
- 整形、静的検査、全Goテスト、Windowsビルド、ヘルプ・版表示、通常・worktreeの実行ファイルでの受入確認、既存のRelease判定テストが成功した。実行ファイルの版表示は `workscope 0.4.0` で正本と一致した。SHA-256は `8A5A5AB65741683338CB27E77E0E58939028D067390A145EDFC2E2F43DEF8908`。
- 異なる開始時点と許容範囲を持つ `日本語 作業A` と `日本語 作業a` を作り、従来の記録と併存させた。それぞれの記録ID・変更件数・範囲判定、未記録名の拒否と、選択した記録だけの取り直しを実行ファイルで確認した。対象ファイル、Git状態、選択しなかった記録の保持も確認した。
- 自動テストでタスク記録の初回作成と後からの従来記録の追加、名前とファイル名の不一致、破損・リンクの拒否、後から作られた同名記録・空ディレクトリとの競合、Windowsの置換失敗時の記録保持を確認した。状態記録のリンク拒否テストは `go test -mod=readonly -count=1 -v ./internal/baseline -run '^TestTaskRecordsProtectMismatchedAndUnreadableState/symlink$'` でも実行し、スキップなしの成功を確認した。
- 読み取り専用の補助担当が記録保護・旧形式の読み取り・引数処理を独立点検し、後から作られた空ディレクトリの所有確認不足を指摘した。主担当が修正と再現テストを加え、最終ソースを必須検証で確認した。修正後の実行検証と最終判断は主担当による。

## 確認対象と結果（2026-10-22）

- 基点はM2完了コミット `ef8ad9242249d97c3aacb4b5bd22a7b67b2c48a1`。M3で追加したテスト・開発用スクリプト・文書を加えたソースを検証対象とする。製品のGoソースと保存形式は変更していない。
- CLI配布単位 `dist/workscope.exe` の版は `0.3.0` から据え置く。製品の動作・表示・配布内容を変更せず、受入確認と開発文書だけを追加したためである。保存・JSON形式は `schema_version: 1` を維持する。
- 確認環境はWindows 11 Home（ビルド26300）、`windows/amd64`、Go 1.27.3、PowerShell 7.6.7、Git 2.56.0.windows.2。
- 最終差分への `./scripts/verify.ps1` が成功した。整形、`go vet`、全Goテスト、Windowsビルド、ヘルプ・版表示、通常・worktreeでの実行ファイルの受入確認を通過した。版表示は `workscope 0.3.0` で正本と一致した。受入対象の実行ファイルのSHA-256は `73D55017A6955FABF88888A5EC9A2DBE35E84B57CA282CF2CC52D45B308A762B`。
- リンク3件とWindows固有テスト8件を `go test -mod=readonly -count=1 -v ./internal/baseline -run 'Test(BeginDoesNotFollowLinks|CheckLinkTargetChangesWithoutFollowingContents|CheckReportsNewParentLinkAlongsideUnreadableDescendant|Windows.*)'` で別途実行し、全11件が成功、スキップは0件だった。実際のリンク・ジャンクション、読取不能、置換失敗と以前の記録の保持を確認した。
- 読み取り専用の補助担当がM2実装と既存テストを独立点検し、worktreeでの非破壊性と改行名の比較・JSON経路の確認不足を指摘した。主担当が追加し、実行結果を確認した。追加後のテスト・スクリプト・対応表の静的確認では、重要な指摘はなかった。検査の実行と最終変更の確認は主担当による。

## 確認範囲と制限

Windowsで作成できない改行・タブ入りのファイル名を、Windowsの実ファイルで確認したとは扱わない。これらはNUL区切りのGit出力を模したデータから比較・JSON出力までを検証する。リンクやジャンクションの作成権限がない環境では該当Goテストがスキップされるため、`go test -v` の結果で実行できたかを区別する。

取得後に変わった状態、途中で変わって元に戻った操作、変更者の特定は保証しない。サブモジュールの内容とジャンクションは判定対象外であり、存在する場合は完全な成功としない。公開・署名・インストーラー・配備はM3の対象に含めない。
