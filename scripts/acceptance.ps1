#requires -Version 7.0

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not $IsWindows) {
    throw '実行ファイルの受入確認はWindowsで実行してください。'
}
$executable = (Resolve-Path -LiteralPath (Join-Path $PSScriptRoot '../dist/workscope.exe')).Path

function Assert-Acceptance([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw $Message }
}

function Invoke-Captured([string] $File, [string] $Directory, [string[]] $Arguments) {
    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $File
    $start.WorkingDirectory = $Directory
    $start.UseShellExecute = $false
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.StandardOutputEncoding = [System.Text.Encoding]::UTF8
    $start.StandardErrorEncoding = [System.Text.Encoding]::UTF8
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    # 呼出元のGit設定用環境変数を、一時リポジトリへ持ち込まない。
    foreach ($key in @($start.Environment.Keys)) {
        if ($key.StartsWith('GIT_', [System.StringComparison]::OrdinalIgnoreCase)) {
            $null = $start.Environment.Remove($key)
        }
    }
    $start.Environment['GIT_OPTIONAL_LOCKS'] = '0'
    $start.Environment['GIT_TERMINAL_PROMPT'] = '0'
    # 診断語句の確認を、利用者のGitの表示言語に依存させない。
    $start.Environment['LC_ALL'] = 'C'
    $start.Environment['LANGUAGE'] = 'C'
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $start
    try {
        $null = $process.Start()
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit()
        return @{ Code = $process.ExitCode; Text = $stdout.GetAwaiter().GetResult(); Error = $stderr.GetAwaiter().GetResult() }
    }
    finally { $process.Dispose() }
}

function Invoke-FixtureGit([string] $Directory, [string[]] $Arguments) {
    $response = Invoke-Captured 'git' $Directory (@('--no-optional-locks', '-C', $Directory, '-c', 'core.fsmonitor=false', '-c', 'core.untrackedCache=false') + $Arguments)
    Assert-Acceptance ($response.Code -eq 0) "一時リポジトリのGit操作に失敗しました: $($response.Error)"
    return $response.Text.TrimEnd("`r", "`n")
}

function Write-Fixture([string] $Directory, [string] $Name, [string] $Text) {
    $path = Join-Path $Directory $Name
    $null = [System.IO.Directory]::CreateDirectory([System.IO.Path]::GetDirectoryName($path))
    [System.IO.File]::WriteAllText($path, $Text, [System.Text.UTF8Encoding]::new($false))
}

function Test-Progress([string] $Text) {
    if ($Text -eq '') { return $true }
    $pattern = '^進捗: (Git待ち: git [A-Za-z0-9 =<>_./伏字-]+|処理の準備|Git作業場所の確認|許容範囲の確認|開始記録の確認|ファイル走査|許容範囲の再確認|開始記録の保存|開始後の変更の照合|開始記録の再確認|処理終了) \| 確認済みファイル: [0-9]+件 \| 取得: [0-2]/2回目 \| 経過: [0-9]+\.[0-9]秒$'
    foreach ($line in $Text.TrimEnd("`r", "`n").Split("`n")) {
        if ($line.TrimEnd("`r") -cnotmatch $pattern) { return $false }
    }
    return $true
}

function Invoke-JSON([string] $Directory, [string[]] $Arguments, [int] $Code, [string] $Status) {
    $response = Invoke-Captured $executable $Directory ($Arguments + @('--json'))
    Assert-Acceptance ($response.Code -eq $Code -and (Test-Progress $response.Error)) "終了コードまたは標準エラーが不正です: $($response.Code) $($response.Error) $($response.Text)"
    # Parseは後続のJSONや診断文も拒否する。ConvertFrom-Jsonだけで件数を判断しない。
    $document = [System.Text.Json.JsonDocument]::Parse($response.Text)
    try {
        Assert-Acceptance ($document.RootElement.ValueKind -eq [System.Text.Json.JsonValueKind]::Object) 'JSONがオブジェクト1件ではありません。'
    }
    finally { $document.Dispose() }
    $data = ConvertFrom-Json -InputObject $response.Text -AsHashtable
    foreach ($field in @('schema_version', 'tool', 'command', 'status', 'baseline_id', 'root', 'coverage', 'preexisting_changes', 'changes_since_begin', 'index_changes', 'out_of_scope_changes', 'issues')) {
        Assert-Acceptance ($data.ContainsKey($field)) "JSONの固定項目 $field がありません。"
    }
    Assert-Acceptance ($data.schema_version -eq 1 -and $data.tool -ceq 'workscope' -and $data.command -ceq $Arguments[0] -and $data.status -ceq $Status) 'JSONの識別情報または状態が不正です。'
    foreach ($field in @('preexisting_changes', 'changes_since_begin', 'index_changes', 'out_of_scope_changes', 'issues')) {
        Assert-Acceptance ($data[$field] -is [array]) "$field が配列ではありません。"
    }
    return @{ Data = $data; Text = $response.Text }
}

function Get-ProtectedState([string] $Directory, [bool] $IncludeBaseline = $true) {
    $paths = @('config.toml', '.gitignore', 'Src/変更 日本語.txt', 'Src/削除.txt', 'Src/追加 日本語.txt', 'Src2/別.txt', 'index-only.txt', '未追跡 空白.txt', '新規/配下/追加.txt', 'ignored/hidden.txt', '.git') |
        ForEach-Object { Join-Path $Directory $_ }
    $index = Invoke-FixtureGit $Directory @('rev-parse', '--path-format=absolute', '--git-path', 'index')
    $gitDirectory = Invoke-FixtureGit $Directory @('rev-parse', '--absolute-git-dir')
    $commonDirectory = Invoke-FixtureGit $Directory @('rev-parse', '--path-format=absolute', '--git-common-dir')
    $config = Join-Path $commonDirectory 'config'
    Assert-Acceptance ((Test-Path -LiteralPath $index -PathType Leaf) -and (Test-Path -LiteralPath $config -PathType Leaf)) '比較対象のインデックスまたは共通Git設定が見つかりません。'
    $paths += @($index, $config, (Join-Path $gitDirectory 'HEAD'), (Join-Path $gitDirectory 'config.worktree'))
    if ($IncludeBaseline) {
        $paths += Join-Path $Directory '.workscope/baseline.json'
        $paths += @(Get-ChildItem -LiteralPath (Join-Path $Directory '.workscope') -Filter 'task-*.json' -File |
            Sort-Object Name | Select-Object -ExpandProperty FullName)
    }
    $hashes = [ordered]@{}
    foreach ($path in $paths) {
        $hashes[$path] = if (Test-Path -LiteralPath $path -PathType Leaf) {
            (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
        } elseif (Test-Path -LiteralPath $path -PathType Container) { 'directory' } else { 'missing' }
    }
    return ConvertTo-Json -InputObject $hashes -Compress
}

# この実行で作った一時領域だけを使用し、利用者のリポジトリは操作しない。
$temporary = [System.IO.Directory]::CreateTempSubdirectory('workscope-acceptance-').FullName
try {
    $emptyHooks = Join-Path $temporary '空のフック'
    $null = [System.IO.Directory]::CreateDirectory($emptyHooks)
    # 起動用コマンドを介さず、実行ファイル自身のGit失敗時の終了コードを確認する。
    $outsideGit = Join-Path $temporary 'Git外'
    $null = [System.IO.Directory]::CreateDirectory($outsideGit)
    foreach ($arguments in @(@('begin', '--allow', 'new.txt'), @('check'))) {
        $unknown = Invoke-JSON $outsideGit $arguments 2 'unknown'
        $message = $unknown.Data.issues[0].message
        Assert-Acceptance ($unknown.Data.issues[0].code -ceq 'git_not_repository' -and
            $message.Contains('失敗したGit操作: git rev-parse --show-toplevel') -and
            $message.Contains('原因:') -and $message.Contains('次の対処:') -and
            -not $unknown.Data.issues[0].ContainsKey('details') -and
            -not $message.Contains('パスSHA-256')) 'Git診断の分類または要約が不正です。'
        $verbose = Invoke-JSON $outsideGit ($arguments + @('--verbose')) 2 'unknown'
        Assert-Acceptance ($verbose.Data.issues[0].details.Contains('Gitの終了コード: 128') -and
            $verbose.Data.issues[0].details.Contains('not a git repository') -and
            $verbose.Data.issues[0].details.Contains('パスSHA-256')) '指定したGit診断の詳細がありません。'
    }
    Assert-Acceptance (-not (Test-Path -LiteralPath (Join-Path $outsideGit '.workscope'))) 'Git外で状態保存先を作成しました。'
    foreach ($mode in @('通常', 'worktree')) {
        $primary = Join-Path $temporary "$mode/主 作業"
        $null = [System.IO.Directory]::CreateDirectory($primary)
        $null = Invoke-FixtureGit $primary @('init', '-q', '--initial-branch=main', "--template=$emptyHooks")
        $null = Invoke-FixtureGit $primary @('config', 'core.autocrlf', 'false')
        $null = Invoke-FixtureGit $primary @('config', 'core.hooksPath', $emptyHooks)
        foreach ($name in @('config.toml', 'Src/変更 日本語.txt', 'Src/削除.txt', 'Src2/別.txt', 'index-only.txt')) {
            Write-Fixture $primary $name '開始時の本文'
        }
        Write-Fixture $primary '.gitignore' "ignored/`n"
        $null = Invoke-FixtureGit $primary @('add', '.')
        $commit = @('-c', 'user.name=workscope-test', '-c', 'user.email=test@example.invalid', '-c', 'commit.gpgsign=false', 'commit', '-qm', '受入確認')
        $null = Invoke-FixtureGit $primary $commit
        $directory = $primary
        if ($mode -eq 'worktree') {
            $directory = Join-Path $temporary "$mode/追加 作業ツリー"
            $null = Invoke-FixtureGit $primary @('worktree', 'add', '--detach', '-q', $directory)
        }
        $subdirectory = Join-Path $directory 'Src'
        Write-Fixture $directory 'config.toml' '開始前の変更'
        Write-Fixture $directory '未追跡 空白.txt' '開始前の未追跡'
        Write-Fixture $directory 'ignored/hidden.txt' '照合しない本文'
        $before = Get-ProtectedState $directory $false
        # 空の保存先を保護し、案内に従って退避した後だけ新しい開始点を作れることを確認する。
        $statePath = Join-Path $directory '.workscope'
        $null = [System.IO.Directory]::CreateDirectory($statePath)
        foreach ($arguments in @(
            @('begin', '--task', '復旧確認', '--allow', 'Src'),
            @('begin', '--task', '復旧確認', '--replace', '--allow', 'Src'),
            @('check', '--task', '復旧確認', '--trust-repository', $directory)
        )) {
            foreach ($verbose in @($false, $true)) {
                $diagnosticArgs = if ($verbose) { $arguments + @('--verbose') } else { $arguments }
                $unknown = Invoke-JSON $subdirectory $diagnosticArgs 2 'unknown'
                Assert-Acceptance ($unknown.Data.issues.Count -eq 1 -and
                    $unknown.Data.issues[0].code -ceq 'foreign_state' -and
                    $unknown.Data.issues[0].path -ceq '.workscope' -and
                    -not $unknown.Data.coverage.complete) "$mode の空の保存先の判定が不正です。"
                $message = $unknown.Data.issues[0].message
                foreach ($phrase in @('既存の .workscope は空', '保存先は変更していません', '通常の空ディレクトリ', '別名へ退避', '次の編集前に begin', '空の状態保存先から復旧する')) {
                    Assert-Acceptance ($message.Contains($phrase)) "$mode の空の保存先の診断に $phrase がありません。"
                }
                $text = Invoke-Captured $executable $subdirectory $diagnosticArgs
                Assert-Acceptance ($text.Code -eq 2) "$mode の空の保存先の通常表示で終了コードが不正です。"
                if ($arguments[0] -eq 'begin') {
                    Assert-Acceptance ($text.Text -eq '' -and $text.Error.Contains($message)) "$mode のbeginの診断がJSONと一致しません。"
                } else {
                    Assert-Acceptance ($text.Text.Contains($message) -and (Test-Progress $text.Error)) "$mode のcheckの診断がJSONと一致しません。"
                }
                Assert-Acceptance (@(Get-ChildItem -LiteralPath $statePath -Force -ErrorAction Stop).Count -eq 0) "$mode の拒否した空の保存先に書き込みました。"
            }
        }
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory $false)) "$mode の空の保存先の拒否で対象・Git状態が変化しました。"
        $emptyState = Get-Item -LiteralPath $statePath -Force -ErrorAction Stop
        Assert-Acceptance ($emptyState.PSIsContainer -and -not ($emptyState.Attributes -band [System.IO.FileAttributes]::ReparsePoint)) '退避する保存先が通常のディレクトリではありません。'
        $backup = Join-Path $directory '.workscope.empty-backup'
        Assert-Acceptance (-not (Test-Path -LiteralPath $backup -ErrorAction Stop)) '退避先が既に存在します。'
        Rename-Item -LiteralPath $emptyState.FullName -NewName '.workscope.empty-backup' -ErrorAction Stop
        $recorded = Invoke-JSON $subdirectory @('begin', '--allow', 'src\', '--allow', 'SRC', '--allow', '新規\配下', '--trust-repository', $directory) 0 'recorded'
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory $false)) "$mode のbeginが対象・Git状態を変更しました。"
        Assert-Acceptance ($recorded.Data.coverage.complete -and $recorded.Data.preexisting_changes.Count -eq 2) "$mode の開始前の変更または照合範囲が不正です。"
        $before = Get-ProtectedState $directory
        $unchanged = Invoke-JSON $subdirectory @('check') 0 'within_scope'
        Assert-Acceptance ($unchanged.Data.changes_since_begin.Count -eq 0 -and $unchanged.Data.index_changes.Count -eq 0) "$mode の開始前の変更を開始後の変更と扱いました。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の変更なしの照合が既存状態を変更しました。"
        Assert-Acceptance ((Get-Item -LiteralPath $backup -Force -ErrorAction Stop).PSIsContainer -and
            @(Get-ChildItem -LiteralPath $backup -Force -ErrorAction Stop).Count -eq 0) "$mode の退避した空の保存先が保持されていません。"

        $before = Get-ProtectedState $directory
        foreach ($argument in @('--help', '--version')) {
            $response = Invoke-Captured $executable $subdirectory @($argument)
            Assert-Acceptance ($response.Code -eq 0 -and $response.Error -eq '' -and $response.Text.Length -gt 0) "$argument の表示に失敗しました。"
        }
        foreach ($command in @('begin', 'check')) {
            foreach ($flag in @('--help', '-h')) {
                $response = Invoke-Captured $executable $subdirectory @($command, $flag)
                Assert-Acceptance ($response.Code -eq 0 -and $response.Error -eq '' -and $response.Text.Contains('使用例:')) "$command $flag の表示に失敗しました。"
            }
        }
        $null = Invoke-JSON $subdirectory @('begin', '--allow', 'Src') 2 'unknown'
        $null = Invoke-JSON $subdirectory @('begin', '--allow', '../範囲外') 64 'invalid_arguments'
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の表示・拒否したbeginが既存状態を変更しました。"

        # 記録の読取を許したまま置換を拒否し、保存失敗時の終了コードを直接確認する。
        $recordPath = Join-Path $directory '.workscope/baseline.json'
        $recordHandle = [System.IO.File]::Open($recordPath, [System.IO.FileMode]::Open,
            [System.IO.FileAccess]::Read, [System.IO.FileShare]::Read)
        try {
            $failedSave = Invoke-JSON $subdirectory @('begin', '--replace', '--allow', 'Src') 2 'unknown'
            Assert-Acceptance ($failedSave.Data.issues[0].code -ceq 'save_failed') "$mode の保存失敗を再現できませんでした。"
            $failedText = Invoke-Captured $executable $subdirectory @('begin', '--replace', '--allow', 'Src')
            Assert-Acceptance ($failedText.Code -eq 2 -and $failedText.Text -eq '' -and
                $failedText.Error.Contains('開始記録の置換に失敗')) "$mode の保存失敗時の通常表示または終了コードが不正です。"
        }
        finally { $recordHandle.Dispose() }
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の保存失敗で既存状態が変わりました。"

        $null = Invoke-JSON $subdirectory @('check', '--trust-repository', $directory) 0 'within_scope'
        $null = Invoke-JSON $subdirectory @('check', '--trust-repository', $subdirectory) 64 'invalid_arguments'
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の信頼指定が既存状態を変更しました。"

        Write-Fixture $directory 'Src/変更 日本語.txt' '範囲内の変更'
        Write-Fixture $directory 'Src/追加 日本語.txt' '範囲内の追加'
        Remove-Item -LiteralPath (Join-Path $directory 'Src/削除.txt')
        Write-Fixture $directory '新規/配下/追加.txt' '未作成パスの配下'
        $before = Get-ProtectedState $directory
        $within = Invoke-JSON $subdirectory @('check') 0 'within_scope'
        Assert-Acceptance ($within.Data.changes_since_begin.Count -eq 4 -and $within.Data.out_of_scope_changes.Count -eq 0) "$mode の範囲内の追加・変更・削除を判定できません。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の範囲内照合が既存状態を変更しました。"

        Write-Fixture $directory 'config.toml' '開始後の再編集'
        Write-Fixture $directory 'Src2/別.txt' '似た名前の範囲外'
        Write-Fixture $directory 'index-only.txt' 'インデックスだけに残す本文'
        $null = Invoke-FixtureGit $directory @('add', 'config.toml', 'index-only.txt')
        Write-Fixture $directory 'index-only.txt' '開始時の本文'
        Write-Fixture $directory 'ignored/hidden.txt' '対象外の追加編集'
        $before = Get-ProtectedState $directory
        $outside = Invoke-JSON $subdirectory @('check') 1 'out_of_scope'
        $again = Invoke-JSON $subdirectory @('check') 1 'out_of_scope'
        Assert-Acceptance ($outside.Text -ceq $again.Text) "$mode の同一条件でJSONの並び順が変化しました。"
        Assert-Acceptance ($outside.Data.changes_since_begin.Count -eq 6 -and $outside.Data.index_changes.Count -eq 2 -and $outside.Data.out_of_scope_changes.Count -eq 4) "$mode の再編集・範囲外・インデックスの集計が不正です。"
        $expected = @('Src/追加 日本語.txt', 'Src/削除.txt', 'Src/変更 日本語.txt', '新規/配下/追加.txt')
        foreach ($change in $outside.Data.changes_since_begin) {
            Assert-Acceptance ($change.within_scope -eq ($expected -ccontains $change.path) -and -not $change.path.Contains('\')) "許容範囲またはパス区切りが不正です: $($change.path)"
        }
        $response = Invoke-Captured $executable $subdirectory @('check')
        Assert-Acceptance ($response.Code -eq 1 -and (Test-Progress $response.Error) -and $response.Text.Contains('範囲外の変更: 4件') -and $response.Text.Contains('開始前からあった変更: 2件')) "$mode の日本語表示が不正です。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の照合が対象・Git状態・開始記録を変更しました。"

        $null = Invoke-FixtureGit $directory $commit
        $before = Get-ProtectedState $directory
        $unknown = Invoke-JSON $subdirectory @('check') 2 'unknown'
        Assert-Acceptance (-not $unknown.Data.coverage.complete -and 'head_changed' -cin $unknown.Data.issues.code -and $unknown.Data.out_of_scope_changes.Count -gt 0) "$mode の判定不能と範囲外の併記が不正です。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の判定不能時に既存状態を変更しました。"

        $before = Get-ProtectedState $directory $false
        $replacement = Invoke-JSON $subdirectory @('begin', '--replace', '--allow', 'Src') 0 'recorded'
        Assert-Acceptance ($replacement.Data.baseline_id -cne $recorded.Data.baseline_id) "$mode の明示的な取り直しが反映されません。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory $false)) "$mode の明示的な取り直しが対象・Git状態を変更しました。"

        # 異なる開始時点と許容範囲を持つタスクを、従来の記録と併存させる。
        $defaultPath = Join-Path $directory '.workscope/baseline.json'
        $defaultHash = (Get-FileHash -LiteralPath $defaultPath -Algorithm SHA256).Hash
        $before = Get-ProtectedState $directory $false
        $taskA = Invoke-JSON $subdirectory @('begin', '--task', '日本語 作業A', '--allow', 'Src') 0 'recorded'
        Assert-Acceptance ($taskA.Data.task -ceq '日本語 作業A' -and $taskA.Data.baseline_id -cne $replacement.Data.baseline_id) "$mode のタスクの記録が不正です。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory $false)) "$mode のタスク記録が対象・Git状態を変更しました。"
        Write-Fixture $directory 'config.toml' 'タスクA開始後の変更'
        $before = Get-ProtectedState $directory $false
        $taskB = Invoke-JSON $subdirectory @('begin', '--task=日本語 作業a', '--allow', 'config.toml') 0 'recorded'
        Assert-Acceptance ($taskB.Data.task -ceq '日本語 作業a' -and $taskB.Data.baseline_id -cne $taskA.Data.baseline_id) "$mode の大小文字が異なるタスクを区別できません。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory $false)) "$mode の別タスクの追加が対象・Git状態を変更しました。"
        Assert-Acceptance ($defaultHash -ceq (Get-FileHash -LiteralPath $defaultPath -Algorithm SHA256).Hash) "$mode のタスク追加が従来の記録を変更しました。"

        $before = Get-ProtectedState $directory
        $checkedA = Invoke-JSON $subdirectory @('check', '--task=日本語 作業A') 1 'out_of_scope'
        $checkedB = Invoke-JSON $subdirectory @('check', '--task', '日本語 作業a') 0 'within_scope'
        Assert-Acceptance ($checkedA.Data.baseline_id -ceq $taskA.Data.baseline_id -and $checkedA.Data.changes_since_begin.Count -eq 1 -and $checkedA.Data.out_of_scope_changes[0].path -ceq 'config.toml') "$mode のタスクAの開始時点・許容範囲が不正です。"
        Assert-Acceptance ($checkedB.Data.baseline_id -ceq $taskB.Data.baseline_id -and $checkedB.Data.changes_since_begin.Count -eq 0) "$mode のタスクBの開始時点が不正です。"
        $missing = Invoke-JSON $subdirectory @('check', '--task', '未記録') 2 'unknown'
        Assert-Acceptance ($missing.Data.baseline_id -ceq '' -and $missing.Data.issues[0].code -ceq 'baseline_missing') "$mode の未記録名で別の開始記録を使いました。"
        $null = Invoke-JSON $subdirectory @('begin', '--task', '日本語 作業A', '--allow', 'Src') 2 'unknown'
        $null = Invoke-JSON $subdirectory @('check', '--task=') 64 'invalid_arguments'
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の照合・拒否したタスク操作が既存状態を変更しました。"

        # 選んだタスクだけを取り直し、他の開始時点が維持されることを確かめる。
        $before = Get-ProtectedState $directory $false
        $taskReplacement = Invoke-JSON $subdirectory @('begin', '--replace', '--task', '日本語 作業a', '--allow', 'config.toml') 0 'recorded'
        Assert-Acceptance ($taskReplacement.Data.baseline_id -cne $taskB.Data.baseline_id -and $taskReplacement.Data.task -ceq '日本語 作業a') "$mode の選んだタスクを取り直せません。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory $false) -and $defaultHash -ceq (Get-FileHash -LiteralPath $defaultPath -Algorithm SHA256).Hash) "$mode のタスクの取り直しが対象・Git状態・従来の記録を変更しました。"
        $before = Get-ProtectedState $directory
        $againA = Invoke-JSON $subdirectory @('check', '--task', '日本語 作業A') 1 'out_of_scope'
        Assert-Acceptance ($againA.Text -ceq $checkedA.Text) "$mode の別タスクの取り直しでタスクAの記録が変化しました。"
        $againB = Invoke-JSON $subdirectory @('check', '--task', '日本語 作業a') 0 'within_scope'
        Assert-Acceptance ($againB.Data.baseline_id -ceq $taskReplacement.Data.baseline_id) "$mode の取り直したタスクを選べません。"
        $defaultCheck = Invoke-JSON $subdirectory @('check') 1 'out_of_scope'
        Assert-Acceptance ($defaultCheck.Data.baseline_id -ceq $replacement.Data.baseline_id -and -not $defaultCheck.Data.ContainsKey('task')) "$mode のタスク省略時に従来の記録を使いませんでした。"
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode のタスク照合が既存状態を変更しました。"

        Write-Fixture $directory '.workscope/baseline.json' '{'
        $before = Get-ProtectedState $directory
        $null = Invoke-JSON $subdirectory @('check') 2 'unknown'
        $null = Invoke-JSON $subdirectory @('begin', '--replace', '--allow', 'Src') 2 'unknown'
        Assert-Acceptance ($before -ceq (Get-ProtectedState $directory)) "$mode の破損した開始記録を変更しました。"
        Write-Output "受入確認に成功しました: $mode （終了コード0・1・2・64、JSON、日本語表示、タスク別記録、非破壊性）"
    }
}
finally {
    # 作成した絶対パスと一時領域の境界を確認してから、自分の検証データだけを除去する。
    $resolved = [System.IO.Path]::GetFullPath($temporary)
    $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolved.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase) -or
        -not [System.IO.Path]::GetFileName($resolved).StartsWith('workscope-acceptance-', [System.StringComparison]::Ordinal)) {
        throw '受入確認用の一時領域を安全に特定できません。'
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
