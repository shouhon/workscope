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
$code = $LASTEXITCODE
$result = $text | ConvertFrom-Json
$result.status
$result.coverage.complete
$result.counts
# スクリプトの終了時は、加工前に保持した $code を使う
```

`counts` は次の項目を持ちます。完全に照合できた場合の0は、該当する変更がないことを示します。

| 項目 | 件数の意味 |
| --- | --- |
| `preexisting_changes` | 開始記録にある開始前の変更 |
| `changes_since_begin` | 開始後の作業ファイルの変化 |
| `index_changes` | 開始後のステージ状態の変化 |
| `out_of_scope_changes` | 範囲外の作業ファイルとステージ状態の変化の合計 |
| `issues` | 出力した問題 |

同じ範囲外のパスでも、作業ファイルとステージ状態の両方が変われば2件と数えます。`begin` が成功した直後は、開始前の変更件数を示し、開始後・ステージ・範囲外の件数は0です。`issues` は機械判定用の `code` と日本語の `message` を含み、関係する `path` も必要に応じて示します。`--verbose` 指定時だけ伏字済みの `details` を追加します。

読み取り不能やHEAD変更などで `coverage.complete` が `false` になった場合、`counts` の開始後・ステージ・範囲外の総件数は `null` です。代わりに `observed_counts` の同名の3項目で、検出できた件数を下限として返します。例えば `counts.out_of_scope_changes: null` と `observed_counts.out_of_scope_changes: 1` は「範囲外を少なくとも1件検出したが、総件数は未確認」という意味です。下限が0でも、変更なしとは判断できません。

開始記録を取得できなければ `counts.preexisting_changes` も `null` です。正常な開始記録を取得できていれば、現在の照合が不完全でも開始前の件数は返します。問題数は常に整数です。完全な照合では `observed_counts` を省略します。要約でも終了コードと既存記録の保護は同じで、`check` は記録を更新しません。

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

## 空の状態保存先から復旧する

既存の `.workscope` が空の場合、`begin` と `check` は「既存の .workscope は空」と表示し、`unknown`・`foreign_state`・終了コード2で停止します。JSONの `issues[].message` と `--verbose` でも同じ原因と復旧案内を確認できます。空の保存先を自動で採用・削除・修復することはありません。

この手順は、Gitルート直下の `.workscope` が、リンクやジャンクションではない通常の空ディレクトリである場合だけ使います。他のworkscopeや保存先を変更する処理が動いていないことを確認してから、PowerShellで次を実行します。読み取りに失敗した場合は空と判断せず、操作を中止してください。

```powershell
$gitRoot = & git --no-optional-locks rev-parse --show-toplevel
if ($LASTEXITCODE -ne 0) { throw 'Gitルートを確認できません。' }
if ((Get-Location).ProviderPath -ne (Resolve-Path -LiteralPath $gitRoot -ErrorAction Stop).ProviderPath) {
    throw '対象のGitルートへ移動してから確認してください。'
}
$state = Get-Item -LiteralPath .workscope -Force -ErrorAction Stop
if (-not $state.PSIsContainer -or ($state.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) {
    throw '.workscope が通常のディレクトリではありません。'
}
if (@(Get-ChildItem -LiteralPath $state.FullName -Force -ErrorAction Stop).Count -ne 0) {
    throw '.workscope は空ではありません。既存の内容を確認してください。'
}
$backup = Join-Path $gitRoot '.workscope.empty-backup'
if (Test-Path -LiteralPath $backup -ErrorAction Stop) {
    throw '退避先が既に存在します。未使用の名前を選び、再確認してください。'
}
Rename-Item -LiteralPath $state.FullName -NewName '.workscope.empty-backup' -ErrorAction Stop
```

隠しファイルを含めて空であることと、退避先が未使用であることを確認してから、元のディレクトリを残したまま別名へ退避します。確認中に保存先が変化した場合も、操作を中止してください。

退避後、**次の編集前に**今回の許容範囲で `begin` を実行し、続けて `check` で確認します。次は `src` を許容する例です。タスク名を使う場合は、両方に同じ `--task <名前>` を付けてください。

```powershell
workscope begin --allow src --json
if ($LASTEXITCODE -ne 0) { throw '開始記録を作成できませんでした。' }
workscope check --json
```

復旧後の記録は、その `begin` を実行した時点が開始点です。それ以前の編集を開始後の変更として検証する記録を、後付けで再現することはできません。`--replace` は空・破損・別用途の保存先に対する保護を解除しません。`--trust-repository` はGitの所有権確認に対処する指定であり、空の保存先の解消には使えません。

### 空ではない・読み取れない保存先の場合

上の空ディレクトリ用の退避手順を使わず、`check --json` の `issues[].code` と `issues[].path` から問題の対象を確認してください。`baseline.json` や `task-*.json` がある場合は、既存の内容を保護し、JSONとして読み取れるか、`tool`・`schema_version`・`root`・`task` がそのリポジトリ・タスクの記録と一致するかを確認します。未知の形式や破損した記録を手作業で正常扱いに変更しないでください。

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

範囲外の変更と判定不能が同時にある場合は、確認できた変更と問題の両方を報告して2で終了します。読めない対象を変更なしや削除と決めつけません。開始時の対象が無視された未追跡ファイルへ変わった場合は、内容を読まず判定不能とします。開始後に状態が変わり続ける場合も、完全な照合としません。

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
