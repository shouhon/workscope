#requires -Version 7.0

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$planScript = Join-Path $PSScriptRoot 'release-plan.ps1'
$publishScript = Join-Path $PSScriptRoot 'publish-release.ps1'

function Assert-Release([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw $Message }
}

function Assert-ReleaseFailure([scriptblock] $Action, [string] $Message) {
    $failed = $false
    try { & $Action | Out-Null } catch { $failed = $true }
    Assert-Release $failed $Message
}

function Invoke-ReleaseFixtureGit([string[]] $Arguments) {
    $start = [System.Diagnostics.ProcessStartInfo]::new('git')
    $start.WorkingDirectory = $fixture
    $start.UseShellExecute = $false
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    foreach ($key in @($start.Environment.Keys)) {
        if ($key.StartsWith('GIT_', [System.StringComparison]::OrdinalIgnoreCase)) {
            $null = $start.Environment.Remove($key)
        }
    }
    foreach ($argument in (@('-c', "core.hooksPath=$emptyHooks", '-c', 'user.name=workscope-test',
        '-c', 'user.email=test@example.invalid', '-c', 'commit.gpgsign=false') + $Arguments)) {
        $start.ArgumentList.Add($argument)
    }
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $start
    try {
        $null = $process.Start()
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        $process.WaitForExit()
        Assert-Release ($process.ExitCode -eq 0) "一時リポジトリのGit操作に失敗しました: $($stderr.GetAwaiter().GetResult())"
        return $stdout.GetAwaiter().GetResult().TrimEnd("`r", "`n")
    }
    finally { $process.Dispose() }
}

function New-ReleaseRevision([string] $Version, [string] $Comment) {
    $source = "package version`n// $Comment`nconst Current = `"$Version`"`n"
    Set-Content -LiteralPath (Join-Path $fixture 'internal/version/version.go') -Value $source -Encoding utf8NoBOM
    $null = Invoke-ReleaseFixtureGit @('add', 'internal/version/version.go')
    $null = Invoke-ReleaseFixtureGit @('commit', '-qm', $Comment)
    return Invoke-ReleaseFixtureGit @('rev-parse', 'HEAD')
}

# 公開処理の外部呼び出しを置き換え、本物のAPIやghを実行しない。
function Invoke-PublishFixture([hashtable] $Case) {
    $calls = [System.Collections.Generic.List[object]]::new()
    $requests = [System.Collections.Generic.List[string]]::new()
    function Invoke-RestMethod($Method, $Uri, $Headers) {
        $requests.Add($Uri)
        if ($Case.ContainsKey('APIError')) {
            $response = [System.Net.Http.HttpResponseMessage]::new([System.Net.HttpStatusCode] $Case.APIError)
            throw [Microsoft.PowerShell.Commands.HttpResponseException]::new('検証用のAPI失敗', $response)
        }
        if ($Uri -match '/releases\?per_page=100&page=([0-9]+)$') {
            $page = [int] $Matches[1]
            if ($Case.ContainsKey('DraftOnSecondPage')) {
                if ($page -eq 1) {
                    return @(1..100 | ForEach-Object { [pscustomobject]@{ tag_name = "v1.0.$_"; draft = $false } })
                }
                return [pscustomobject]@{ tag_name = 'v0.3.1'; draft = $true }
            }
            if ($Case.ContainsKey('Existing')) {
                return [pscustomobject]@{ tag_name = 'v0.3.1'; draft = $Case.Existing -eq 'draft' }
            }
            return @()
        }
        if ($Uri.EndsWith('/git/ref/tags/v0.3.1') -and $Case.ContainsKey('TagCommit')) {
            return [pscustomobject]@{ ref = 'refs/tags/v0.3.1' }
        }
        if ($Uri.EndsWith('/commits/v0.3.1') -and $Case.ContainsKey('TagCommit')) {
            return [pscustomobject]@{ sha = $Case.TagCommit }
        }
        if ($Uri.EndsWith('/releases/latest') -and $Case.ContainsKey('Latest')) {
            return [pscustomobject]@{ tag_name = $Case.Latest }
        }
        $response = [System.Net.Http.HttpResponseMessage]::new([System.Net.HttpStatusCode]::NotFound)
        throw [Microsoft.PowerShell.Commands.HttpResponseException]::new('検証用の404', $response)
    }
    function gh {
        $calls.Add(@($args))
        $global:LASTEXITCODE = if ($Case.ContainsKey('GHError')) { 1 } else { 0 }
    }
    $failure = $null
    try {
        & $publishScript -Version '0.3.1' -Commit $targetCommit -Repository 'shouhon/workscope' -ArtifactDirectory $artifacts | Out-Null
    }
    catch { $failure = $_.Exception.Message }
    return @{ Calls = $calls; Requests = $requests; Error = $failure }
}

$temporary = [System.IO.Directory]::CreateTempSubdirectory('workscope-release-').FullName
$previousToken = $env:GH_TOKEN
$previousExitCode = Get-Variable -Name LASTEXITCODE -Scope Global -ValueOnly -ErrorAction SilentlyContinue
try {
    $fixture = Join-Path $temporary 'repository'
    $emptyHooks = Join-Path $temporary 'hooks'
    $artifacts = Join-Path $temporary 'artifacts'
    $null = New-Item -ItemType Directory -Path (Join-Path $fixture 'internal/version'), $emptyHooks, $artifacts
    $null = Invoke-ReleaseFixtureGit @('init', '-q', '--initial-branch=main', "--template=$emptyHooks")
    $initial = New-ReleaseRevision '0.3.0' '開始版'
    $commentOnly = New-ReleaseRevision '0.3.0' 'コメントだけの変更'
    $bumped = New-ReleaseRevision '0.3.1' '修正版'
    $afterBump = New-ReleaseRevision '0.3.1' '版更新後の別コミット'
    $invalid = New-ReleaseRevision '0.3.1-beta' '不正な版'
    Push-Location -LiteralPath $fixture
    try {
        $plan = & $planScript -Before $initial -After $commentOnly
        Assert-Release (-not $plan.Changed) 'コメントだけの変更で公開対象になりました。'
        $plan = & $planScript -Before $commentOnly -After $bumped
        Assert-Release ($plan.Changed -and $plan.Version -ceq '0.3.1') '版更新を検出できません。'
        $plan = & $planScript -Before $initial -After $afterBump
        Assert-Release $plan.Changed '複数コミットのpushの途中にある版更新を検出できません。'
        $plan = & $planScript -Before $bumped -After $afterBump
        Assert-Release (-not $plan.Changed) '版番号を変えない後続pushで公開対象になりました。'
        $plan = & $planScript -Before ('0' * 40) -After $bumped
        Assert-Release (-not $plan.Changed) '比較元のないブランチ作成で公開対象になりました。'
        Assert-ReleaseFailure { & $planScript -Before $bumped -After $initial } '版番号の後退を拒否できません。'
        Assert-ReleaseFailure { & $planScript -Before $bumped -After $invalid } '不正な版番号を拒否できません。'
        Assert-ReleaseFailure { & $planScript -Before ('f' * 40) -After $bumped 2>$null } '取得できない比較元を拒否できません。'
    }
    finally { Pop-Location }

    $targetCommit = $bumped
    $env:GH_TOKEN = 'workscope-test-placeholder'
    $archiveName = 'workscope_0.3.1_windows_amd64.zip'
    $archivePath = Join-Path $artifacts $archiveName
    $manifestPath = Join-Path $artifacts 'workscope.json'
    Set-Content -LiteralPath $archivePath -Value '外部公開しない検証用の添付' -Encoding utf8NoBOM
    $archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    @{ version = '0.3.1'; architecture = @{ '64bit' = @{
        url = "https://github.com/shouhon/workscope/releases/download/v0.3.1/$archiveName"
        hash = $archiveHash
    } } } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $manifestPath -Encoding utf8NoBOM
    $manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    @("$archiveHash  $archiveName", "$manifestHash  workscope.json") |
        Set-Content -LiteralPath (Join-Path $artifacts 'checksums.txt') -Encoding utf8NoBOM

    $result = Invoke-PublishFixture @{ Latest = 'v0.3.0' }
    Assert-Release ($null -eq $result.Error -and $result.Calls.Count -eq 1) "新規公開を開始できません: $($result.Error)"
    $arguments = $result.Calls[0]
    Assert-Release ($arguments[0] -ceq 'release' -and $arguments[1] -ceq 'create' -and $arguments[2] -ceq 'v0.3.1' -and
        $arguments -ccontains '--latest=true' -and $arguments -ccontains $targetCommit -and
        $arguments -ccontains $archivePath -and $arguments -ccontains $manifestPath -and
        $arguments -ccontains (Join-Path $artifacts 'checksums.txt')) 'タグ、対象コミット、添付、最新版の指定が不正です。'
    $result = Invoke-PublishFixture @{}
    Assert-Release ($null -eq $result.Error -and $result.Calls.Count -eq 1) '初回公開を開始できません。'
    $result = Invoke-PublishFixture @{ Existing = 'published' }
    Assert-Release ($null -eq $result.Error -and $result.Calls.Count -eq 0 -and $result.Requests.Count -eq 1) '公開済みの版を変更しようとしました。'
    $result = Invoke-PublishFixture @{ Existing = 'draft' }
    Assert-Release ($result.Error -like '*下書き*' -and $result.Calls.Count -eq 0) '既存の下書きを拒否できません。'
    $result = Invoke-PublishFixture @{ DraftOnSecondPage = $true }
    Assert-Release ($result.Error -like '*下書き*' -and $result.Calls.Count -eq 0 -and $result.Requests.Count -eq 2) '一覧の2ページ目にある下書きを拒否できません。'
    $result = Invoke-PublishFixture @{ TagCommit = ('1' * 40) }
    Assert-Release ($result.Error -like '*別のコミット*' -and $result.Calls.Count -eq 0) '別コミットのタグを拒否できません。'
    $result = Invoke-PublishFixture @{ TagCommit = $targetCommit; Latest = 'v0.4.0' }
    Assert-Release ($null -eq $result.Error -and $result.Calls.Count -eq 1 -and $result.Calls[0] -ccontains '--latest=false') '古い版の処理で最新版が後退します。'
    $result = Invoke-PublishFixture @{ APIError = 500 }
    Assert-Release ($null -ne $result.Error -and $result.Calls.Count -eq 0 -and $result.Requests.Count -eq 1) 'API障害を未公開として扱いました。'
    $result = Invoke-PublishFixture @{ GHError = $true }
    Assert-Release ($result.Error -like '*公開に失敗*' -and $result.Calls.Count -eq 1) '公開失敗を成功として扱うか、公開を繰り返しました。'
    Add-Content -LiteralPath $archivePath -Value '破損した添付'
    $result = Invoke-PublishFixture @{}
    Assert-Release ($result.Error -like '*SHA-256*' -and $result.Calls.Count -eq 0) 'ハッシュが一致しない配布物を拒否できません。'

    Write-Output 'Releaseの確認に成功しました: push前後の版比較、重複防止、タグ照合、最新版の維持、API失敗、配布物の照合'
}
finally {
    $env:GH_TOKEN = $previousToken
    $global:LASTEXITCODE = $previousExitCode
    $resolved = [System.IO.Path]::GetFullPath($temporary)
    $tempRoot = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath()).TrimEnd('\') + '\'
    if (-not $resolved.StartsWith($tempRoot, [System.StringComparison]::OrdinalIgnoreCase) -or
        -not [System.IO.Path]::GetFileName($resolved).StartsWith('workscope-release-', [System.StringComparison]::Ordinal)) {
        throw 'Release検証用の一時領域を安全に特定できません。'
    }
    Remove-Item -LiteralPath $resolved -Recurse -Force
}
