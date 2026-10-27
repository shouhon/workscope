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

`--json` でも進捗は標準エラーへ出し、標準出力はJSONオブジェクト1件を保ちます。JSONをプログラムへ渡す場合は、標準エラーを標準出力へ結合しないでください。

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

`begin` と `check` は、Gitの失敗時に `unknown` と終了コード2を返します。通常表示とJSONの `issues[].message` に、失敗したGit操作、起動エラー、Git自体の終了コード、伏字にした標準エラーを含めます。Gitが起動しなかった場合、Gitの終了コードは「未取得（起動失敗）」です。Gitの終了コードが1でも、workscopeの判定不能は2です。1を返すのは `check` が範囲外の変更を検出した場合だけです。

Gitの標準エラーは `permission denied`、`detected dubious ownership`、`not a git repository` などの既知の診断語句だけを残します。パス、URL、認証情報、任意の設定値を含み得る部分や未知の内容は伏字にします。利用者由来の参照名も表示しません。`ownership`（所有権）、`permission`（アクセス権限）、`not_found`（Git未検出）、`not_repository`（Git作業ツリー外）などの分類と、それぞれの再実行の案内を追加します。サンドボックスが原因と断定するものではありません。

診断には、PATHで選択したGitの実行パスと `git -C` の作業場所を、`パスSHA-256` として表示します。これは実行ファイルの内容のハッシュではなく、`/` 区切りの絶対パスのUTF-8から計算した識別子です。パスに含まれ得る秘密や個人名を伏せたまま、直接実行時と同じ場所か比較できます。Gitを起動前に特定できなかった場合は「未特定」です。

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

workscopeはシェルのGit別名・関数を使わず、親の `GIT_*` をすべて除外します。別の作業領域やインデックスの記録を防ぐためです。診断では `GIT_CONFIG_COUNT`・`GIT_CONFIG_KEY_*`・`GIT_CONFIG_VALUE_*`・`GIT_CONFIG_PARAMETERS` による設定注入、`GIT_CONFIG_GLOBAL`・`GIT_CONFIG_SYSTEM`・`GIT_CONFIG_NOSYSTEM` などの設定場所、`GIT_DIR`・`GIT_WORK_TREE` などの指定が親にあったかを、値なしで表示します。その他は任意の変数名も伏せて件数だけを出します。HOME・USERPROFILE・XDG_CONFIG_HOMEなどの通常の設定探索条件は継承しますが、実際にどの設定ファイルを読めたかは未確認です。

このため、直接実行時だけ環境変数や `git -c` で `safe.directory` を渡した場合、workscope内のGitとは結果が異なり得ます。`ownership` の場合は、まず同じ利用者、Gitの実行パス、作業場所、設定の渡し方を確認してください。所有者と内容を確認して信頼できる共有リポジトリである場合だけ、次のプレースホルダー2か所を置き換え、同じ利用者で対象のGitルート1件を明示的に登録します。

`GIT_CONFIG_GLOBAL` などが残った環境で単に `git config --global` を実行すると、内部Gitが読まない別ファイルへ保存される場合があります。次の手順は、親の環境を変更せず、内部Gitと同じく `GIT_*` を除外した子環境から通常のグローバル設定へ登録します。

```powershell
$start = [System.Diagnostics.ProcessStartInfo]::new()
$start.FileName = '<確認したGitの絶対パス>'
$start.UseShellExecute = $false
foreach ($key in @($start.Environment.Keys)) {
    if ($key.StartsWith('GIT_', [System.StringComparison]::OrdinalIgnoreCase)) {
        $null = $start.Environment.Remove($key)
    }
}
foreach ($argument in @('config', '--global', '--add', 'safe.directory', '<確認したGitルートの絶対パス>')) {
    $start.ArgumentList.Add($argument)
}
$process = [System.Diagnostics.Process]::Start($start)
try {
    $process.WaitForExit()
    if ($process.ExitCode -ne 0) { throw '信頼するGitルートの登録に失敗しました。' }
}
finally { $process.Dispose() }
# その後、対象へ戻り、元のworkscopeコマンドを再実行する。
```

これは利用者が選ぶ永続的な設定変更です。対象を限定する扱いは[Gitの safe.directory の仕様](https://git-scm.com/docs/git-config#Documentation/git-config.txt-safedirectory)に従います。workscope自身は設定変更・権限昇格・再試行を行いません。`permission` はアクセス権限の問題として、必要な対象の読取権限とGitの実行権限を確認してから再実行してください。

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
