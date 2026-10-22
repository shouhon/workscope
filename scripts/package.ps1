#requires -Version 7.0

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
$previousToolchain = $env:GOTOOLCHAIN

Push-Location -LiteralPath $projectRoot
try {
    $env:GOTOOLCHAIN = 'local'
    $architecture = & go env GOARCH
    if ($LASTEXITCODE -ne 0 -or $architecture -ne 'amd64') {
        throw '配布物の作成対象はWindowsのamd64です。GOARCHの設定を確認してください。'
    }

    # 配布前の検証を省略せず、確認した実行ファイルをそのまま梱包する。
    & (Join-Path $PSScriptRoot 'verify.ps1')
    $versionSource = Get-Content -LiteralPath 'internal/version/version.go' -Raw
    $versionMatch = [regex]::Match($versionSource, '(?m)^const Current = "(\d+\.\d+\.\d+)"\r?$')
    if (-not $versionMatch.Success) {
        throw '版番号の正本を読み取れませんでした。'
    }
    $version = $versionMatch.Groups[1].Value
    $versionText = @(& ./dist/workscope.exe --version)
    if ($LASTEXITCODE -ne 0 -or $versionText.Count -ne 1 -or $versionText[0] -cne "workscope $version") {
        throw '実行ファイルの版番号と正本が一致しません。'
    }

    $archiveName = "workscope_${version}_windows_amd64.zip"
    $archivePath = Join-Path 'dist' $archiveName
    Compress-Archive -LiteralPath @('dist/workscope.exe', 'LICENSE') -DestinationPath $archivePath -Force
    $archiveHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    $manifest = [ordered]@{
        version = $version
        description = '作業開始時からの変化を照合し、指定範囲を外れた変更を報告するCLI'
        homepage = 'https://github.com/shouhon/workscope'
        license = 'MIT'
        architecture = [ordered]@{
            '64bit' = [ordered]@{
                url = "https://github.com/shouhon/workscope/releases/download/v$version/$archiveName"
                hash = $archiveHash
            }
        }
        bin = 'workscope.exe'
        depends = 'git'
    }
    $manifestPath = Join-Path 'dist' 'workscope.json'
    $manifest | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath $manifestPath -Encoding utf8NoBOM
    $manifestHash = (Get-FileHash -LiteralPath $manifestPath -Algorithm SHA256).Hash.ToLowerInvariant()
    @("$archiveHash  $archiveName", "$manifestHash  workscope.json") |
        Set-Content -LiteralPath 'dist/checksums.txt' -Encoding utf8NoBOM

    Write-Output "作成: $archivePath"
    Write-Output '作成: dist/workscope.json'
    Write-Output '作成: dist/checksums.txt'
}
finally {
    $env:GOTOOLCHAIN = $previousToolchain
    Pop-Location
}
