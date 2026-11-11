# 利用ガイド

## コマンドごとのヘルプと進捗

```powershell
workscope begin --help
workscope check --help
```

`-h` でも表示できます。必須の引数と使用例を表示し、Gitが使えない場所でも終了コード0で終了します。ヘルプは開始記録を作成・更新しません。

`begin`・`check` が長くかかる場合は、約1秒ごとに次の形式で標準エラーへ進捗を表示します。1秒未満で完了する場合は省略します。

```text
進捗: Git待ち: git ls-files --stage -z | 確認済みファイル: 0件 | 取得: 1/2回目 | 経過: 1.0秒
進捗: ファイル走査 | 確認済みファイル: 120件 | 取得: 1/2回目 | 経過: 2.0秒
```

整合性を確かめるために全体を2回取得します。ファイル数は各回で数え直し、正常に確認できたファイル・リンク・削除済みの対象を数えます。ディレクトリ自体や無視された未追跡ファイルは数えません。1つの大きなファイルの読取中は、確認が終わるまで件数が増えないことがあります。進捗の「処理終了」は成功を意味せず、最終結果と終了コードで判断してください。

`--json`・`--json-summary` でも進捗は標準エラーへ出し、標準出力はJSONオブジェクト1件を保ちます。JSONをプログラムへ渡す場合は、標準エラーを標準出力へ結合しないでください。

## 判定と件数だけをJSONで受け取る

`begin`・`check` に `--json-summary` を付けると、変更パスの明細を省き、判定、開始記録の識別子、照合範囲・完全性、件数と問題を受け取れます。詳細なパスとハッシュが必要な場合は従来の `--json` を使います。両方の指定は引数の誤り（64）です。

```powershell
workscope begin --task '機能追加' --allow src --json-summary

# 編集後、同じタスクの開始記録と照合する
$text = workscope check --task '機能追加' --json-summary
$workscopeExit = $LASTEXITCODE
$result = $text | ConvertFrom-Json
$result.status
$result.coverage.complete
$result.counts
# 対話中は、加工前に保持した終了コードを確認する
$workscopeExit
```

`counts` は次の項目を持ちます。完全に照合できた場合の0は、該当する変更がないことを示します。

| 項目 | 件数の意味 |
| --- | --- |
| `preexisting_changes` | 開始記録にある開始前の変更 |
| `changes_since_begin` | 開始後の作業ファイルの変化 |
| `index_changes` | 開始後のステージ状態の変化 |
| `out_of_scope_changes` | 範囲外の作業ファイルとステージ状態の変化の合計 |
| `issues` | 出力した問題 |

同じ範囲外のパスでも、作業ファイルとステージ状態の両方が変われば2件と数えます。開始記録を新しく作った直後は、開始前の変更件数を示し、開始後・ステージ・範囲外の件数は0です。`begin --extend` は最初の開始状態からの件数を返すため、追加成功時にも変更が存在する場合があります。`issues` は機械判定用の `code` と日本語の `message` を含み、関係する `path` も必要に応じて示します。`--verbose` 指定時だけ伏字済みの `details` を追加します。

読み取り不能やHEAD変更などで `coverage.complete` が `false` になった場合、`counts` の開始後・ステージ・範囲外の総件数は `null` です。代わりに `observed_counts` の同名の3項目で、検出できた件数を下限として返します。例えば `counts.out_of_scope_changes: null` と `observed_counts.out_of_scope_changes: 1` は「範囲外を少なくとも1件検出したが、総件数は未確認」という意味です。下限が0でも、変更なしとは判断できません。

開始記録を取得できなければ `counts.preexisting_changes` も `null` です。正常な開始記録を取得できていれば、現在の照合が不完全でも開始前の件数は返します。問題数は常に整数です。完全な照合では `observed_counts` を省略します。要約でも終了コードと既存記録の保護は同じで、`check` は記録を更新しません。

### 非対話のPowerShellへ終了コードを引き継ぐ

判定不能時は、実行ファイルも自身の終了コード2とPowerShellでの対処を標準エラーへ表示します。JSON指定や `--no-progress` 指定でもこの診断は残ります。`workscope --help` と各コマンドの `--help` でも実行例を確認できます。

workscope自身の終了コードと、それを起動したPowerShellプロセスの終了コードは別です。`pwsh -Command` は、末尾の外部コマンドが2を返しても、明示的な `exit` がないと外側へ1を返すことがあります。JSONの加工などが後続して成功すると、外側が0になることもあります。[PowerShellの公式資料](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_pwsh?view=powershell-7.6#-command--c)も参照してください。

PowerShellから非対話の子プロセスを起動する場合や、AIのコマンド実行ツールへ渡す場合は、CLIの直後に `$LASTEXITCODE` を保持し、最後にその値を `exit` で返します。次はPowerShellから呼び出す例です。対象のGit作業ディレクトリで実行し、タスク名を保存済みの名前に置き換えてください。

```powershell
pwsh -NoProfile -NonInteractive -Command 'workscope check --task "機能追加" --json-summary --no-progress; $workscopeExit = $LASTEXITCODE; exit $workscopeExit'
# 外側でもworkscope自身の終了コードを取得する
$LASTEXITCODE
```

JSONを加工する場合も、加工前に保持した終了コードを返します。

```powershell
pwsh -NoProfile -NonInteractive -Command '
    $text = workscope check --task "機能追加" --json-summary --no-progress
    $workscopeExit = $LASTEXITCODE
    $result = $text | ConvertFrom-Json
    $result | Select-Object status, coverage, counts | ConvertTo-Json -Depth 5
    exit $workscopeExit
'
$LASTEXITCODE
```

`exit` は実行中のPowerShellを終了させるため、対話中は冒頭の例のように保持した値を確認してください。終了コード1は範囲外の変更あり、2は判定不能です。JSONの `status`・`coverage.complete` も併せて確認し、判定不能を成功や範囲外の変更へ読み替えないでください。`--no-progress` は進捗だけを省略します。原因を確認できるように標準エラーの診断は残し、JSONの標準出力へ結合しないでください。

## 開始記録を作る

ビルドした実行ファイルを、記録したいGit作業ディレクトリ内で実行します。`--allow` は実行場所にかかわらずGitルートからの相対パスです。

```powershell
workscope begin --allow AGENTS.md --allow src --json
```

記録先は `<Gitルート>/.workscope/baseline.json` です。既に記録がある場合は終了コード2で停止します。明示的に取り直す場合だけ、次を実行します。

```powershell
workscope begin --replace --allow AGENTS.md --allow src
```

ファイルとディレクトリに加え、これから作るパスも指定できます。ワイルドカード、ルート外、Git管理領域、状態保存先、リンクを親に含むパスは指定できません。Windowsの区切り・大小文字を扱い、同じ対象の重複指定をまとめます。サブモジュールや未対応のジャンクションがある場合、読めない対象がある場合、取得中に状態が変わる場合は記録せず終了コード2を返します。シンボリックリンクは参照先の文字列だけを記録し、参照先の内容は読みません。

追跡中のファイルは無視設定に一致しても対象です。無視された未追跡ファイル、Git管理領域、状態保存先は除外します。通常の作業ディレクトリとworktreeに対応し、対象ファイル・インデックス・Git設定を変更しません。`.workscope` に正常な自分の記録以外がある場合は、`--replace` でも変更しません。

状態保存先をGitから除外するには、利用者が `.gitignore` に `/.workscope/` を追加してください。CLIは自動変更しません。`--json` では標準出力にJSONオブジェクト1件を出力します。記録成功は0、記録不能は2、引数の誤りは64で終了します。

## 開始後に無視対象となったファイルを照合する

開始時に記録した未追跡ファイルを作業中に `.gitignore` へ追加すると、通常の `check` は `coverage_changed`・終了コード2を返します。対象を `--allow` で許容していても、読み取り範囲は広がりません。

内容を確認してよい記録済みファイルを、`check --include-ignored` で1件ずつ明示できます。診断の `issues[].path` を確認し、開始時と同じタスクを選んでください。次は、2件のログを除外した後に照合する例です。

```powershell
workscope check --task '除外設定' --include-ignored '.playwright-mcp/first.log' --include-ignored '.playwright-mcp/second.log' --json-summary --no-progress
$workscopeExit = $LASTEXITCODE
$workscopeExit
```

指定はGitルート相対です。`--include-ignored=パス` も使え、Windowsの大小文字・区切りを扱い、重複はまとめます。開始記録にハッシュがあるファイル・リンクだけを指定でき、ディレクトリ全体、未記録の対象、ワイルドカード、ルート外、Git管理領域、状態保存先は64で拒否します。現在は無視対象でない記録済みファイルを指定しても、通常と同じく照合します。

無視された配下全体は探索しません。新しくできた無視対象や、指定していない記録済み無視対象も読みません。リンクは参照先の文字列だけを確認します。内容・削除・種類の変化を通常どおり報告し、許容外の変更は1、読取不能・親リンクなどは2のままです。通常ファイルがディレクトリになっても、新しい配下へ対象を広げません。

開始記録のID・日時・ハッシュ・許容範囲を保持し、ファイル・インデックス・Git設定も変更しません。`begin --replace` で取り直したり内部JSONを編集したりする必要はありません。指定はこの実行だけに有効で、次の照合でも必要です。

通常表示では明示したパスを示し、両JSONでは `coverage.ignored_baseline_paths` に正規化済みの指定を返します。`coverage.included` には `explicit_ignored_baseline_paths` が加わり、除外側は `other_ignored_untracked_files` に変わります。読み取りが完了したかは `coverage.complete`・`issues` と終了コードで確認してください。指定していない既知ファイルが無視対象に残れば、照合全体は引き続き判定不能です。実行ファイルだけでも `workscope check --help` から同じ制約と操作例を確認できます。

## 開始状態を保持して許容パスを追加する

作業途中の追加依頼で変更対象が増えたら、同じ開始記録に `begin --extend` でパスを追加できます。追加する対象を編集する前に実行してください。開始記録ID、開始日時、HEAD、ファイルとインデックスの開始状態、開始前の変更は保持します。

```powershell
workscope begin --task '機能追加' --allow src
# srcの作業中に、利用手順も更新する追加依頼を受けた場合
workscope begin --task '機能追加' --extend --allow README.md --reason '追加依頼で利用手順も更新'
if ($LASTEXITCODE -ne 0) { throw '許容範囲を追加できませんでした。' }
# README.mdを編集した後も、最初の開始状態と照合する
workscope check --task '機能追加' --json
```

`--allow` は複数指定できます。`--reason`（`--reason=理由` も可）は必須で、空・前後の空白・制御文字は拒否します。`--replace` と併用できません。既存の許容範囲に含まれる指定は追加せず、新たな範囲が1件もなければ64を返します。削除・縮小は行いません。開始記録がない場合、HEAD変更、破損、読取不能、保存失敗・同時更新では2で停止し、元の記録を保持します。

追加成功の `begin` は終了コード0・`status: "extended"` を返します。無関係な範囲外変更が残っている場合も追加自体は成功するため、続けて `check` の結果を確認してください。`check` は最新の許容範囲で判定し、無関係な変更は引き続き範囲外とします。`--replace` は開始状態も取り直す別の操作で、新しい記録の追加履歴は空になります。他のタスクの記録は変更しません。

通常表示と `--json` は、当初の許容範囲 `initial_allow` と、追加順の `scope_extensions` を示します。履歴の各項目は `added_at`（UTC日時）、`reason`、`allow`（追加パスと種類）、`changed_paths` を持ちます。`changed_paths` は、新たに許容された範囲で、追加時点に開始状態との差分またはGitの未コミット変更があったパスの一覧です。作業ファイルとインデックスの両方が変わっていても同じパスは1件です。開始前からの未コミット変更も、追加時点に残っていれば含めます。

追加前の編集を、新しい開始状態へ移すことはありません。履歴は許可を後から追加した事実を示し、最初からその範囲の変更が許可されていた証拠にはしません。開始時に照合対象外だった無視された未追跡ファイルの内容も、後から開始状態へ補いません。

`--json-summary` でも `scope_extensions` を返しますが、各項目は日時・理由と `allow_count`・`changed_path_count` のみです。パス明細と `initial_allow` は省略します。履歴のない記録では、これらの追加項目自体を省略します。従来の記録はそのまま読み取れます。範囲追加に未対応の旧CLIは、保存先全体の形式検証で履歴付き記録を拒否します。拡張したタスクが1件あると同じ保存先の未拡張タスクも扱えないため、範囲追加後はその `.workscope` 全体を対応したCLIで扱ってください。

## 空の状態保存先から復旧する

既存の `.workscope` が通常の空ディレクトリなら、`begin` がそのまま最初の開始記録を保存します。追加の指定、利用者の確認、手作業の退避は不要です。未作成の場合も従来どおり作成します。

**次の編集前に**今回の許容範囲で `begin` を実行し、成功を確認してから編集してください。次は `src` を許容する例です。タスク名を使う場合は、両方に同じ `--task <名前>` を付けます。

```powershell
workscope begin --allow src --json
if ($LASTEXITCODE -ne 0) { throw '開始記録を作成できませんでした。' }
workscope check --json
```

CLIが隠し項目を含めて空であることを読み取り、開始時と保存直前のディレクトリの実体を確認します。リンク・ジャンクション、読取不能、別用途の内容、破損・未知形式の記録がある場合は利用せず停止します。確認中の差し替えや並行書き込みを検出した場合も、既存の内容を保護して終了コード2を返します。

`check` と `list` は空の保存先を初期化せず、`unknown`・`foreign_state`・終了コード2で停止します。診断には、次の編集前に `begin --allow <対象>` を実行する案内を表示します。拒否されたことだけを理由に保存先を削除したり、照合済みとして編集を続けたりしないでください。

開始点は、その `begin` を実行した時点です。それ以前の編集を開始後の変更として検証する記録を、後付けで再現することはできません。空の保存先に `--replace` は不要です。同指定は選んだ正常な記録を取り直すためのもので、破損・別用途の保存先に対する保護は解除しません。`--trust-repository` はGitの所有権確認に対処する指定です。

### 空ではない・読み取れない保存先の場合

空と推測して初期化せず、`check --json` の `issues[].code` と `issues[].path` から問題の対象を確認してください。`baseline.json` や `task-*.json` がある場合は、既存の内容を保護し、JSONとして読み取れるか、`tool`・`schema_version`・`root`・`task` がそのリポジトリ・タスクの記録と一致するかを確認します。未知の形式や破損した記録を手作業で正常扱いに変更しないでください。

`record.lock` や一時ファイルが残っている場合は、他の記録処理が実行中でないかを先に確認します。別用途の内容、リンク・ジャンクション、読取権限の不足も、それぞれの原因を確認する必要があります。これらを空と推測して退避したり、削除したりしないでください。

## タスク名ごとに開始記録を保持する

`begin` と `check` に `--task <名前>` を付けると、その名前の開始記録を使います。既存記録のコピーや、別タスクの記録の取り直しは不要です。

```powershell
workscope begin --task '機能追加' --allow src
# 作業を進め、別の作業を始める時点で記録する
workscope begin --task '文書修正' --allow docs
workscope check --task '機能追加' --json
workscope check --task '文書修正' --json

# 同じタスクの開始時点と許容範囲を明示的に取り直す
workscope begin --task '文書修正' --replace --allow docs
```

同じ名前の記録が既にある場合は、`--replace` がない限り終了コード2で停止します。取り直す場合も、他のタスクの記録は変更しません。`check` は記録を更新せず、存在しない名前では終了コード2を返します。最後に開始したタスクへの自動切り替えは行いません。

タスクごとに、開始時点と許容範囲を保持します。どのタスクの照合でも同じ作業ツリー全体を対象にするため、別の作業で加えた変更も、選んだ開始記録と許容範囲に基づいて報告されます。

タスク名には日本語と途中の空白を使えます。名前は完全一致で比較し、大小文字を区別します。空の名前、前後の空白、`/`・`\`、改行などの制御文字、`--task` の複数指定は引数の誤り（64）です。`--task=名前` でも指定できます。

保存先は `<Gitルート>/.workscope/task-<タスク名のUTF-8のSHA-256（64桁、小文字）>.json` です。タスク名を直接ファイルパスにせず、記録内の `task` に元の名前を保存します。JSON出力にも、名前を指定した場合だけ `task` を含めます。通常表示にはタスク名と、`begin` の実際の保存先を表示します。

`--task` を省略した場合は従来の `.workscope/baseline.json` だけを使い、取り直しもその記録だけに行います。旧形式の記録は移動・変換せず読めます。保存・JSON形式は `schema_version: 1` を維持します。タスク別記録のある保存先を旧版CLIで使うことはできないため、新しい版を使ってください。状態保存先に別用途の内容や破損した記録がある場合は、別タスクの指定でも記録を保護して停止します。

## 保存済みの開始記録を探す

タスク名を忘れた場合は、記録を変更せず一覧できます。

```powershell
workscope list
workscope list --json
# 一覧で選んだ名前を指定して照合する。
workscope check --task '文書修正'
```

表示するのはタスク名、記録ID、作成日時、許容範囲の件数です。既定記録を先頭に、その後をタスク名の昇順（大小文字を区別）に並べます。JSONの `records[]` は `task`、`default`、`baseline_id`、`created_at`、`allow_count` を持ちます。既定記録は `task: ""`・`default: true` です。

保存先がまだなければ `status: "listed"`、空配列と終了コード0を返し、保存先は作成しません。空の既存保存先、破損、読取不能、未対応形式、リンクや別用途の内容があれば `unknown`・2で停止します。失敗時の空配列を「記録なし」と判断せず、`status` と終了コードを確認してください。`--verbose` と `--trust-repository` は他のコマンドと同様に使えます。`list` に `--task`、`--allow`、`--replace`、`--json-summary` は指定できません。

一覧は現在のHEADや対象ファイルと照合しません。HEADが変わった記録も表示するため、存在するだけでは有効な照合結果を意味しません。開始記録・対象ファイル・Git設定は変更しません。

## 進捗だけを省略する

繰り返す照合で進捗が不要な場合は、`begin`・`check` に `--no-progress` を付けます。

```powershell
workscope begin --task '機能追加' --allow src --no-progress
workscope check --task '機能追加' --json-summary --no-progress
```

定期的な進捗と終了時の進捗だけを止め、エラーや必要な診断、判定内容、終了コードは維持します。通常表示、`--json`、`--json-summary` のいずれでも指定できます。診断を残すため、標準エラー全体を捨てる必要はありません。省略時は従来どおり、長い処理の進捗を標準エラーへ表示します。

## Gitを参照できない場合

`begin` と `check` は、Gitの失敗時に `unknown` と終了コード2を返します。`begin` の保存失敗も2です。Gitの終了コードが1でも、workscopeの判定不能は2です。1を返すのは `check` が範囲外の変更を検出した場合だけです。

通常表示とJSONの `issues[].message` は、失敗した操作・原因・次の対処を簡潔に示します。原因は次の `issues[].code` で区別できます。

| code | 原因 |
| --- | --- |
| `git_not_found` | PATH上でGitの実行ファイルを検出できない |
| `git_start_failed` | Gitの実行ファイルを検出したが起動できない |
| `git_ownership_rejected` | Gitが起動し、リポジトリの所有権確認で拒否した |
| `git_permission_denied` | Gitが起動し、アクセス権限の不足で拒否した |
| `git_not_repository` | 現在の場所がGit作業ツリーの外にある |
| `git_configuration_error` | Gitの設定を読み取れない |
| `git_failed` | その他のGit起動後の失敗 |

所有権確認で拒否された場合は、所有者と内容を確認して信頼できるGitルート1件を、各実行の `--trust-repository` で指定します。配下から起動しても、指定するのはGitルートの絶対パスです。worktreeでは、その作業ツリーのルートを指定します。

```powershell
workscope begin --allow src --trust-repository 'D:\作業\対象プロジェクト'
workscope check --trust-repository 'D:\作業\対象プロジェクト'
```

この指定は、その実行の子Gitすべてに `-c safe.directory= -c safe.directory=<指定ルート>` として渡します。既存の全件信頼設定もその実行では指定した1件へ絞り、グローバル設定やリポジトリの設定ファイルは変更しません。開始記録にも保存しないため、次の実行でも指定が必要です。空・複数指定・相対パス・ワイルドカード・制御文字・存在しないパスは拒否し、Gitが返すルートと指定したパスの実体も照合します。`--trust-repository=絶対パス` でも指定できます。[Gitの safe.directory の仕様](https://git-scm.com/docs/git-config#Documentation/git-config.txt-safedirectory)に従います。

信頼先の確認に失敗した場合は、次の診断で対処を選べます。空・複数指定・値の欠落は従来どおり `invalid_arguments`・64です。

| code | 状態・終了コード | 原因と対処 |
| --- | --- | --- |
| `invalid_trust_repository` | `invalid_arguments`・64 | 形式不正。Gitルート1件の絶対パスを指定し、ワイルドカードや制御文字を除く |
| `trust_repository_not_found` | `invalid_arguments`・64 | 最初の実体解決・参照で不存在を確認。指定先やリンク先の存在を確認する |
| `trust_repository_not_directory` | `invalid_arguments`・64 | ディレクトリ以外。Gitルートのディレクトリを指定する |
| `trust_repository_mismatch` | `invalid_arguments`・64 | 指定先とGitルートの実体が不一致。現在の作業ツリーのルートを指定する |
| `trust_repository_unreadable` | `unknown`・2 | アクセス拒否などで確認不能。指定先・親の読取・参照権限、実行環境の制限、確認中の消失を調べる |

`--verbose` を付けると、通常表示および両JSONの `issues[].details` に、実体解決・参照・種類確認・ルート照合などの失敗した段階と原因区分を表示します。アクセス拒否と不存在だけを安全に分類し、その他の参照エラーの原因は「未確認」とします。パスはSHA-256の識別子で示し、生の絶対パスやOSエラー本文は追加しません。初回確認後に参照先が消えた場合も、引数誤りとして断定せず2で停止します。拒否時は開始記録やGit設定を変更せず、別ルートや全リポジトリの信頼へ切り替えません。Git自体が所有権などで失敗した場合は、上表ではなく既存の `git_*` 診断を返します。

詳細を調べる場合は、同じコマンドに `--verbose` を追加します。`begin` の詳細指定も実際の記録操作なので、診断だけのために `--replace` を追加しないでください。

```powershell
workscope check --verbose
workscope check --json --verbose
```

通常表示では詳細を結果と同じ出力先へ追加します。JSONでは `issues[].details` に含め、`--verbose` がなければこの項目を省略します。詳細には起動エラー、Git自体の終了コード、伏字にした標準エラー、実行条件を含めます。Gitが起動しなかった場合、Gitの終了コードは「未取得（起動失敗）」です。標準出力はJSONオブジェクト1件を維持し、進捗は標準エラーへ出します。

Gitの標準エラーは `permission denied`、`detected dubious ownership`、`not a git repository` などの既知の診断語句だけを残します。パス、URL、認証情報、任意の設定値を含み得る部分や未知の内容は伏字にし、利用者由来の参照名も表示しません。

詳細には、PATHで選択したGitの実行パスと `git -C` の作業場所を、`パスSHA-256` として表示します。これは実行ファイルの内容のハッシュではなく、`/` 区切りの絶対パスのUTF-8から計算した識別子です。パスに含まれ得る秘密や個人名を伏せたまま、直接実行時と同じ場所か比較できます。Gitを起動前に特定できなかった場合は「未特定」です。

PowerShellでは、手元だけで次のように比較できます。設定値や生の診断全体を共有する必要はありません。

```powershell
function Get-PathId([string] $Path) {
    $normalized = [System.IO.Path]::GetFullPath($Path).Replace('\', '/')
    $bytes = [System.Text.Encoding]::UTF8.GetBytes($normalized)
    [Convert]::ToHexString([System.Security.Cryptography.SHA256]::HashData($bytes)).ToLowerInvariant()
}
$gitExe = (Get-Command git -CommandType Application | Select-Object -First 1).Source
Get-PathId $gitExe
Get-PathId (Get-Location).ProviderPath
```

workscopeはシェルのGit別名・関数を使わず、親の `GIT_*` をすべて除外します。別の作業領域やインデックスの記録を防ぐためです。詳細では `GIT_CONFIG_COUNT`・`GIT_CONFIG_KEY_*`・`GIT_CONFIG_VALUE_*`・`GIT_CONFIG_PARAMETERS`・`GIT_CONFIG_GLOBAL`・`GIT_CONFIG_SYSTEM`・`GIT_CONFIG_NOSYSTEM`・`GIT_DIR`・`GIT_WORK_TREE` などの指定が親にあったかを、値なしで表示します。その他は任意の変数名も伏せて件数だけを出します。HOME・USERPROFILE・XDG_CONFIG_HOMEなどの通常の設定探索条件は継承しますが、実際にどの設定ファイルを読めたかは未確認です。

このため、直接実行時だけ環境変数や `git -c` で `safe.directory` を渡した場合、workscope内のGitとは結果が異なり得ます。`--trust-repository` は親の環境設定を引き継ぐ代わりに、今回信頼するルートを明示する入口です。`git_permission_denied` はアクセス権限の問題なので、必要な対象の読取権限とGitの実行権限を確認してください。

### 制限付き実行環境で実体確認が拒否される場合

必要な対処は実行ファイルにも収録しています。外部文書がない場合は `workscope --help`、または `begin --help`・`check --help`・`list --help` の「制限付き実行環境で信頼先を確認できない場合」を読んでください。`trust_repository_unreadable` の通常表示と両JSONの `issues[].message` にも、承認された実行経路での確認、編集前の記録、確認不能時の停止、後付け記録の限界を案内します。これらの案内に `--verbose` は不要です。

Gitでルートを取得できても、`--trust-repository` の実体確認に必要な参照まで許可されているとは限りません。Windows向けのGoの実装は、リンク解決と名前の正規化でパスの各要素を参照します。そのため、作業ツリーを読めても親の参照が制限される環境では拒否され得ます。[リンク解決の実装](https://go.dev/src/path/filepath/symlink.go)と[Windowsの名前の正規化](https://go.dev/src/path/filepath/symlink_windows.go)が根拠ですが、診断だけで拒否されたパス要素やWindows APIを特定することはできません。

必要な権限は、Gitの起動、作業ツリーと対象ファイルの読み取り・列挙、Git管理領域（worktreeの共通管理領域を含む）の読み取り、ルートまでのパス要素とリンク先の実体確認です。`begin` だけは、その作業ツリーの `.workscope/` に作成・保存する権限も必要です。Git設定・インデックスへの書き込み権限は不要です。実体確認の権限不足を、パス文字列の一致で代用しません。

編集前に、所有者と内容を確認したGitルートで、記録していない診断用タスク名を選んで次を実行します。

```powershell
$repoRoot = (Get-Location).ProviderPath
git --no-optional-locks -C $repoRoot -c safe.directory= -c "safe.directory=$repoRoot" rev-parse --show-toplevel
$gitCode = $LASTEXITCODE
workscope check --task '権限確認用の未記録名' --trust-repository $repoRoot --json-summary --verbose --no-progress
$checkCode = $LASTEXITCODE
```

`trust_repository_unreadable`・2なら、確認段階と原因区分を読みます。環境が提供する承認経路で読み取り専用の同一 `check` を実行できる場合だけ、同じ実行ファイル・ルート・引数で比較してください。`baseline_missing` まで進めば信頼先とGitルートの確認は通過していますが、`begin` の保存や全対象の走査の成功はまだ確認できていません。

制限下で確認できない場合は、権限や隔離を一括解除せず、実行環境が対象と操作を承認した経路で、**編集する前に**次の記録を作ります。承認経路が使えない場合は開始記録を作れたと扱わず、そこで停止します。

```powershell
# 環境が承認した実行経路で、対象のGitルートから実行する。
workscope begin --task '今回の作業' --allow src --allow AGENTS.md --trust-repository $repoRoot --json-summary --no-progress
if ($LASTEXITCODE -ne 0) { throw '開始記録を作成できませんでした。編集を開始しないでください。' }
# status=recorded と baseline_id を確認してから編集する。
# 作業後も実体確認できる承認済みの実行経路で照合する。
workscope check --task '今回の作業' --trust-repository $repoRoot --json-summary --no-progress
```

許容パスは実際の作業に合わせ、`safe.directory=*`、別ルートへの切り替え、診断用の `--replace` は使いません。同じ環境でも対象や制限が変われば、再度確認が必要です。編集後に記録を作っても、過去の編集を開始後の変更として検証できません。既に編集していた場合は、編集前に取得していたGit状態と差分を手動で照合し、その限界を報告します。

2026-11-09の確認では、この開発環境の制限付きシェルはプロセス起動前の初期化で失敗しました。環境の承認経路では編集前の `begin` が成功しましたが、これはIssue #6の実体解決時のアクセス拒否の再現や、制限下での記録成功を示すものではありません。拒否されたパス要素と最小権限での代替APIの有効性は未確認で、実体確認処理は維持しています。

終了コードを調べる場合は、ビルドまたは導入した `workscope.exe` を直接実行し、直後の `$LASTEXITCODE` を確認してください。`go run` など別の起動用コマンドを経由した場合、そのコマンド自体の終了コードと区別する必要があります。

## 開始後の変更を照合する

```powershell
workscope begin --allow AGENTS.md --allow src
# 指定した範囲で作業する
workscope check
workscope check --json
$LASTEXITCODE
```

例えば開始前から `config.toml` に変更があっても、開始後に再編集しなければ範囲外の変更には含めません。開始後に同じファイルを再編集すると、Gitの変更済みの印が同じでも検出します。許容した `src` の配下は範囲内、`src2` の配下は範囲外です。これから作る許容パスは配下も含み、既存のファイルを許容した場合は、そのパスだけを含みます。

`check` は開始記録を更新しません。ファイルの追加・変更・削除・種類変更を報告し、ステージした内容を元に戻した場合も、インデックスのモード・オブジェクトID・競合の段階を別に比較します。名前変更は削除と追加として報告します。

| 終了コード | 照合結果 |
| --- | --- |
| 0 | 完全に照合でき、範囲外の変更なし。範囲内の変更はあってよい |
| 1 | 範囲外の変更あり |
| 2 | 記録なし・破損・別ルート・HEAD変更・読取失敗・サブモジュールなどで判定不能 |
| 64 | 引数の誤り |

範囲外の変更と判定不能が同時にある場合は、確認できた変更と問題の両方を報告して2で終了します。読めない対象を変更なしや削除と決めつけません。開始時の対象が無視された未追跡ファイルへ変わった場合は、既定では内容を読まず `coverage_changed`・判定不能とします。[記録済みファイルの明示照合](#開始後に無視対象となったファイルを照合する)で対象を指定できます。開始後に状態が変わり続ける場合も、完全な照合としません。

JSONの固定項目と意味は[要件定義書](requirements.md#5-出力と終了コード)を参照してください。`changes_since_begin` の各項目は `path`、`source: "worktree"`、`kind`、`before_kind`、`after_kind`、`before_sha256`、`after_sha256`、`within_scope` を持ちます。`kind` は `added`・`modified`・`deleted`・`type_changed` です。存在しない対象の種類は `missing`、ハッシュのない種類は空文字列で表します。

`index_changes` は `source: "index"`、パス、種類、範囲内かの判定と、記録時・現在のインデックス項目を `before`・`after` の配列で持ちます。`out_of_scope_changes` は範囲外だった両方の差分を含むため、同じパスが作業ファイルとインデックスにそれぞれ現れることがあります。各配列はパス順で安定させ、同じパスの範囲外差分は作業ファイル、インデックスの順です。

## PowerShellで別の作業ディレクトリに使う

このプロジェクトでビルドした後、実行ファイルの絶対パスを保持して対象へ移動します。次の対象パスと許容範囲は、自分のプロジェクトに合わせて変更してください。

```powershell
./scripts/build.ps1
$workscope = (Resolve-Path -LiteralPath './dist/workscope.exe').Path
Set-Location -LiteralPath 'D:\作業\対象プロジェクト'

& $workscope begin --allow 'src\日本語 空白' --allow 'AGENTS.md'
if ($LASTEXITCODE -ne 0) { throw '開始記録を作成できませんでした。' }

# ここで対象プロジェクトを編集する
& $workscope check --json
$checkCode = $LASTEXITCODE
switch ($checkCode) {
    0 { '照合完了。範囲外の変更はありません。' }
    1 { '範囲外の変更を確認してください。' }
    2 { '判定不能の理由を確認してください。' }
    64 { '引数を確認してください。' }
    default { throw "予期しない終了コード: $checkCode" }
}
```

worktreeでも、対象のworktree内から同じ手順で実行します。開始記録はそのworktreeのルートに保存され、`--allow` もそのルートを基準にします。JSONをプログラムへ渡す場合は、上の表示用 `switch` を使わず、CLIの標準出力と `$LASTEXITCODE` をそれぞれ受け取ってください。
