#requires -Version 7.0

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$previousToolchain = $env:GOTOOLCHAIN
$projectRoot = Split-Path -Parent $PSScriptRoot

Push-Location -LiteralPath $projectRoot
try {
    # 未導入のGoを自動取得せず、手元の開発環境を使う。
    $env:GOTOOLCHAIN = 'local'
    $targetOS = & go env GOOS
    if ($LASTEXITCODE -ne 0) {
        throw 'Goの対象OSを取得できませんでした。'
    }
    if ($targetOS -ne 'windows') {
        throw '初版のビルド対象はWindowsです。GOOSの設定を確認してください。'
    }

    New-Item -ItemType Directory -Path 'dist' -Force | Out-Null
    & go build -mod=readonly -trimpath -o ./dist/workscope.exe ./cmd/workscope
    if ($LASTEXITCODE -ne 0) {
        throw 'ビルドに失敗しました。'
    }

    Write-Output '作成: dist/workscope.exe'
}
finally {
    $env:GOTOOLCHAIN = $previousToolchain
    Pop-Location
}
