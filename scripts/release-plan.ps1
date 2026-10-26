#requires -Version 7.0
param(
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-fA-F]{40}$')][string] $Before,
    [Parameter(Mandatory)][ValidatePattern('^[0-9a-fA-F]{40}$')][string] $After
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-VersionAtCommit([string] $Commit) {
    $source = @(& git show "${Commit}:internal/version/version.go")
    if ($LASTEXITCODE -ne 0) {
        throw "コミット $Commit の版番号を取得できませんでした。履歴を省略せず取得してください。"
    }
    $matches = [regex]::Matches(($source -join "`n"), '(?m)^const Current = "((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))"\r?$')
    if ($matches.Count -ne 1) {
        throw "コミット $Commit の版番号の正本を読み取れませんでした。"
    }
    return $matches[0].Groups[1].Value
}

$version = Get-VersionAtCommit $After
if ($Before -eq ('0' * 40)) {
    # ブランチの新規作成には比較元がないため、既存版を自動公開しない。
    return [pscustomobject]@{ Changed = $false; Version = $version }
}
$previousVersion = Get-VersionAtCommit $Before
if ([version] $version -lt [version] $previousVersion) {
    throw "版番号を後退させることはできません: $previousVersion → $version"
}

# HEADの親だけでなく、push全体の前後を比較する。
[pscustomobject]@{ Changed = ($version -cne $previousVersion); Version = $version }
