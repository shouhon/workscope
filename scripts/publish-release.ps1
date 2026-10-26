#requires -Version 7.0
param(
    [Parameter(Mandatory)][ValidatePattern('^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$')][string] $Version,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-fA-F]{40}$')][string] $Commit,
    [Parameter(Mandatory)][ValidatePattern('^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$')][string] $Repository,
    [string] $ArtifactDirectory = 'dist'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (-not $env:GH_TOKEN) { throw '公開用のGH_TOKENがありません。' }
$tag = "v$Version"
$repositoryURL = "https://github.com/$Repository"
$headers = @{ Authorization = "Bearer $env:GH_TOKEN"; Accept = 'application/vnd.github+json' }

function Get-GitHubObject([string] $Path) {
    try {
        return Invoke-RestMethod -Method Get -Uri "https://api.github.com/repos/$Repository/$Path" -Headers $headers
    }
    catch {
        # 認証・通信・サーバーの失敗を「存在しない」と扱わない。
        if ($_.Exception -is [Microsoft.PowerShell.Commands.HttpResponseException] -and
            $_.Exception.Response.StatusCode -eq [System.Net.HttpStatusCode]::NotFound) {
            return $null
        }
        throw
    }
}

# タグ指定の取得APIは公開済みだけを返すため、下書きも含む一覧を最後まで調べる。
$page = 1
do {
    $releases = @(Invoke-RestMethod -Method Get -Uri "https://api.github.com/repos/$Repository/releases?per_page=100&page=$page" -Headers $headers)
    foreach ($existing in $releases) {
        if ($existing.tag_name -cne $tag) { continue }
        if ($existing.draft) {
            throw "$tag の下書きが既にあります。添付と対象コミットを確認し、手動で復旧してください。"
        }
        Write-Output "$tag は公開済みのため、変更せずに終了します。"
        return
    }
    $page++
} while ($releases.Count -eq 100)
$tagRef = Get-GitHubObject "git/ref/tags/$tag"
if ($null -ne $tagRef) {
    $tagCommit = Get-GitHubObject "commits/$tag"
    if ($null -eq $tagCommit -or $tagCommit.sha -cne $Commit) {
        throw "$tag の既存タグが公開対象とは別のコミットを指しています。"
    }
}

# 検証の完了順が逆転しても、古い版を最新版にしない。公開ジョブは直列に実行する。
$latest = Get-GitHubObject 'releases/latest'
$makeLatest = $true
if ($null -ne $latest) {
    if ($latest.tag_name -cnotmatch '^v((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))$') {
        throw '公開済みの最新版のタグ名から版番号を読み取れませんでした。'
    }
    $makeLatest = [version] $Version -gt [version] $Matches[1]
}

$archiveName = "workscope_${Version}_windows_amd64.zip"
$archivePath = Join-Path $ArtifactDirectory $archiveName
$manifestPath = Join-Path $ArtifactDirectory 'workscope.json'
$checksumPath = Join-Path $ArtifactDirectory 'checksums.txt'
$archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
$manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
$manifest = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
if ($manifest.version -cne $Version -or
    $manifest.architecture.'64bit'.url -cne "$repositoryURL/releases/download/$tag/$archiveName" -or
    $manifest.architecture.'64bit'.hash -cne $archiveHash -or
    ((Get-Content -LiteralPath $checksumPath) -join "`n") -cne "$archiveHash  $archiveName`n$manifestHash  workscope.json") {
    throw '配布物の版番号、URL、SHA-256が一致しません。'
}

$notes = @"
Windows x64（amd64）用のworkscope $tag です。Git for Windowsが必要です。

- ZIP、Scoop定義、SHA-256一覧を添付しています。ZIPにはMITライセンスを同梱しています。
- 整形、静的検査、全テスト、Windowsビルド、版表示・ヘルプ、通常リポジトリとworktreeでの受入確認を実行しました。
- Scoopでの導入: scoop install $repositoryURL/releases/latest/download/workscope.json
- [利用ガイド]($repositoryURL/blob/$tag/docs/usage.md) · [変更履歴]($repositoryURL/commits/$tag)

対象コミット: $Commit

コード署名は行っていません。
"@
$assets = @($archivePath, $manifestPath, $checksumPath)
# ghは添付を下書きへアップロードし、すべて成功した後に公開する。既存添付を差し替えない。
& gh release create $tag @assets --repo $Repository --target $Commit --title "workscope $tag" --notes $notes "--latest=$($makeLatest.ToString().ToLowerInvariant())"
if ($LASTEXITCODE -ne 0) { throw "$tag の公開に失敗しました。既存のReleaseと添付を確認してください。" }
