# Windowsでの受入確認

[要件定義書の受入条件](requirements.md#6-受入条件)を、Goの自動テストとビルド済み実行ファイルの確認へ対応付ける。要件文書は元の定義を保持し、開発状況は[開発計画](development-plan.md)で管理する。

## 要約JSON出力の確認結果（2026-10-29）

[Issue #2](https://github.com/shouhon/workscope/issues/2)の追加要件を、CLIテストと実行ファイルの受入確認で検証した。

- `begin`・`check` の `--json-summary` が、識別情報・判定・照合範囲・件数・問題を返し、4種類の変更一覧を省くことを確認した。開始前の変更と追加編集、作業ファイルとインデックスの別集計、タスク名と開始記録ID、従来の詳細JSONとの件数・照合範囲の一致を確認した。
- HEAD変更時とWindowsの共有制限による実際の読取不能時に、総件数を `null` とし、確認できた範囲外の変更を `observed_counts` に保持することを確認した。Git未検出、開始記録の欠落・破損・保存失敗、引数誤りでも未確認を0件にせず、`--verbose` による診断詳細の出し分けと出力失敗時の終了コード2を確認した。
- `scripts/acceptance.ps1` は通常の作業ディレクトリとworktreeの両方で、要約と終了コード0・1・2・64、JSONオブジェクト1件、標準エラーの進捗、対象・Git状態・既存記録の保護を確認した。`--json` との併用を状態変更前に拒否し、選んだタスクの記録を使うことも確認した。
- 検証対象は開始コミット `5c4d88248cf4933b8101674063ade1e34266c62a` に今回の差分を加えたソースで、`go.mod`・`cmd/`・`internal/`・`scripts/` を `checkstamp` の入力とした。実行ID `4798b13c2572e7b2bf11869031106714` の `./scripts/verify.ps1` は終了コード0で成功し、結果採用前の `checkstamp status` も `valid`・終了コード0だった。
- ビルド済み実行ファイルの版表示が `internal/version/version.go` の正本と一致した。SHA-256は `E706F4B6D46F4539495AF2F17778BD11C8BD40448891D4E930C1ECEF8EFE82DB`。読み取り専用の補助担当による独立した静的確認で修正指摘はなく、実行結果と最終変更は主担当が確認した。外部依存は追加していない。

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
| 追加要件10. Git診断・進捗・個別ヘルプ | `TestOwnershipDiagnosticExplainsDirectGitDifference` で、環境設定を渡した直接Gitの成功と、内部Gitの所有権拒否、伏字、開始記録の保持、一時信頼による復旧と指定省略時の再拒否を確認する。`TestTrustRepositorySupportsWorktreeAndLimitsExistingWildcard` と不正引数テストで、worktreeの限定、恒久設定の保持、広い信頼指定の除外、ルートの一致を確認する。`TestGitUnavailableOutputAndExitCodes` で未検出・起動失敗・起動後の権限拒否を機械コードで区別し、要約と詳細、単一JSON、終了コード2を確認する。既存の分類・伏字・進捗・個別ヘルプの検査を維持し、受入スクリプトで保存失敗2と通常・worktreeの非破壊性を実行ファイルでも確認する |

## 空の状態保存先の診断・復旧の確認結果（2026-10-28）

- 基点は `a7ec58a134ee4f572c621400ac6461b0d6bea4a2`。Issue #1に対応し、空と確認できた既存の `.workscope` に限って、原因と安全な復旧案内を表示する。診断の修正としてCLI版を `0.6.0` から `0.6.1` へ更新した。`unknown`・`foreign_state`・終了コード2、JSONオブジェクト1件、保存形式と既存の保存先保護は維持した。
- `TestEmptyStateDiagnosticsPreserveDirectory` の再現条件は修正前に診断不足で失敗し、修正後は従来・タスク別の `begin`・`begin --replace`・`check` と通常・JSON・詳細表示の24通りが成功した。拒否の前後で空の保存先、対象ファイル、インデックス、HEAD、Git設定が変化しないことも確認した。
- `scripts/acceptance.ps1` で、通常リポジトリとworktreeの両方を確認した。空の保存先を自動採用せず、`--replace` や `--trust-repository` でも拒否すること、表示とJSONの診断の一致、隠しファイルを含めた空の保持を確認した。通常ディレクトリと未使用の退避先を確認して別名へ退避した後は、`begin` が `recorded`・0、続く `check` が `within_scope`・0となる。退避したディレクトリと既存の対象・Git状態も保持した。
- `checkstamp run --label issue-1-empty-state --input cmd --input internal --input scripts --input go.mod --input .github --input AGENTS.md --input docs/requirements.md --input docs/usage.md --input docs/architecture.md --input README.md --json-summary -- pwsh -NoProfile -File ./scripts/verify.ps1` が成功した。実行IDは `0e2871af1176282a850a7fa97eb2fe31`。採用直前の `checkstamp status` も `valid`、子の終了コード0、現在の変更件数0だった。この結果を記す文書は検証入力に含めていない。
- 整形、`go vet`、全Goテスト、Windowsビルド、ヘルプ・版表示、通常・worktreeの実行ファイルによる受入確認、Release判定テストが成功した。版表示は正本と同じ `workscope 0.6.1`。実行ファイルのSHA-256は `A909E551A4B0953617027B956C6406EB331CAEB3CD2E7FFA094B177F5813D871`。
- 読み取り専用の補助担当が診断の分岐・表示経路、保存先の保護、復旧手順と検査内容を独立確認し、重要な欠落は見つからなかった。実行検証と最終判断は主担当が行った。利用ガイドには空ではない保存先の別手順と、復旧後の記録から過去の開始点を再現できないことも記載した。
- 環境上の未確認事項：通常のsandbox実行は、コマンドの起動前に `helper_unknown_error: setup refresh had errors` で失敗した。上記の検証は承認経路で実行したものであり、同じ復旧をsandbox内で実行できることは未確認である。実際の所有権・アクセス権限・Git設定を変更して対処していない。

## Gitの一時信頼と診断分離の確認結果（2026-10-27）

- 基点は `b5ba91c200015277b182ce4bb517c7c0257d7333`。今回の製品コード・テスト・利用手順・受入スクリプトを加えた状態を確認した。引数の追加と診断コードの変更によりCLI版を `0.5.0` から `0.6.0` へ更新した。開始記録の形式と `schema_version: 1` は維持し、JSONの詳細は `--verbose` 時だけ省略可能な `issues[].details` に含める。
- `checkstamp run --label git-trust-diagnostics --input cmd --input internal --input scripts --input go.mod --input .github --input AGENTS.md --input docs/requirements.md --input docs/usage.md --input docs/architecture.md --input README.md --json-summary -- pwsh -NoProfile -File ./scripts/verify.ps1` が成功した。実行IDは `0fc34792633af4aee48267d7410242e6`。採用直前の `checkstamp status` も `valid`、子の終了コード0、現在の変更件数0を確認した。この結果を記す受入・開発計画文書は検証入力に含めていない。
- 整形、`go vet`、全Goテスト、Windowsビルド、ヘルプ・版表示、通常・worktreeの実行ファイルによる受入確認、Release判定テストが成功した。版表示は正本と同じ `workscope 0.6.0`。実行ファイルのSHA-256は `692488FAF4973CCC3E5B2F25C28AD4807888FDDB37527DD514F7CDE3461C4D4A`。
- 着手時のソースも記録失敗は2を返していた。報告時に1になった起動経路は未確認であり、原因を断定していない。今回、通常・worktreeの開始記録を読取可能なまま置換できない状態にして、実行ファイルの `begin --replace` が通常表示・JSONとも2を返し、既存記録・インデックス・Git設定が不変であることを確認した。Git自体が1で失敗する場合も、CLIの回帰テストでは2を返す。
- 所有者や権限は実際には変更せず、一時リポジトリと隔離したホームでGitの所有権拒否を再現した。親のGit設定を渡した直接実行は成功し、内部Gitは指定なしで拒否、`--trust-repository` 付きで復旧、再度指定を省略すると拒否した。通常のグローバル設定・別途指定した設定ファイル・リポジトリ設定・親の環境変数を変更していない。worktreeではそのルートだけを指定し、共通の管理領域を持つ別の作業ツリーの指定では拒否する。実際の共有先の所有者やUNCパスは未確認である。
- 読み取り専用の補助担当が一時信頼の伝播と範囲、設定の非保存、診断の分類・表示、終了コードを独立確認し、修正が必要な問題は見つからなかった。実行結果と最終差分の確認は主担当が行った。外部依存、Git設定、公開のための変更は追加していない。

## Git診断・進捗・個別ヘルプの確認結果（2026-10-27）

- 基点は `c4b4e39fcd7c882bf7cf47a3cb87555da7b7b51d`。今回の製品コード、テスト、利用手順、検証スクリプトを加えたソースを確認した。CLI版を機能追加により `0.4.0` から `0.5.0` へ更新し、保存・JSON形式は `schema_version: 1` と既存の終了コードを維持した。
- `checkstamp run --label cli-diagnostics-final --input cmd --input internal --input scripts --input go.mod --input .github --input AGENTS.md --input docs/requirements.md --input docs/usage.md --input docs/architecture.md --json-summary -- pwsh -NoProfile -File ./scripts/verify.ps1` が成功した。実行IDは `17059b9b37d66c2eb8040f890a540a9f`。同じ作業ディレクトリで `checkstamp status` が `valid`、子の終了コードが0であることを確認した。この結果を記す受入・開発計画文書は検証入力へ含めていない。
- 整形、静的検査、全Goテスト、Windowsビルド、個別ヘルプを含むヘルプ・版表示、通常・worktreeの受入確認、既存のRelease判定テストが成功した。実行ファイルの表示は正本と同じ `workscope 0.5.0`。SHA-256は `9EA6733DA194336FA5ABB2FEE0D333D1470BE40FEB6AFA014B4D60A37066FB76`。
- 所有者そのものは変更せず、一時リポジトリと隔離したホームでGitの所有権拒否を再現した。直接Gitには対象だけの `safe.directory` を環境設定で渡し、内部Gitでは除外される条件を確認した。内部Gitと同じく `GIT_*` を除いた子環境で通常のグローバル設定へ明示登録すると、`begin`・`check` が成功する。別の設定ファイルと親の環境は変更せず、利用者の実際のホーム・Git設定には書き込んでいない。元の指摘が発生した利用者環境そのものの原因は未確認である。
- テスト用Gitの待機中も進捗を表示でき、JSONが標準出力の1件であることを確認した。走査では追跡済み・未追跡の3件を取得回ごとに数え、無視対象と状態保存先を件数へ含めない。ヘルプはGit未導入の一時環境でも0で終了し、開始記録を作らない。
- 読み取り専用の補助担当が独立確認し、親の `GIT_CONFIG_GLOBAL` が残ったままでは案内した登録先と内部Gitの参照先がずれる点を指摘した。主担当が子環境を使う案内と復旧テストへ修正し、最終ソースへ必須検証を再実行した。修正後の検証と最終判断は主担当による。

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
