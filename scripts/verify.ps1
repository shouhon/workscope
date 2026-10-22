#requires -Version 7.0

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$previousToolchain = $env:GOTOOLCHAIN
$projectRoot = Split-Path -Parent $PSScriptRoot

Push-Location -LiteralPath $projectRoot
try {
    $env:GOTOOLCHAIN = 'local'

    Write-Output '整形を確認しています。'
    $packageDirectories = @(& go list -mod=readonly -f '{{.Dir}}' ./...)
    if ($LASTEXITCODE -ne 0) {
        throw 'パッケージ一覧の取得に失敗しました。'
    }
    $sourceFiles = @(Get-ChildItem -LiteralPath $packageDirectories -Filter '*.go' -File |
        Select-Object -ExpandProperty FullName)
    $unformatted = @(& gofmt -l @sourceFiles)
    if ($LASTEXITCODE -ne 0) {
        throw '整形の確認に失敗しました。'
    }
    if ($unformatted.Count -ne 0) {
        throw "未整形のGoファイルがあります。gofmt -w で整形してください。`n$($unformatted -join "`n")"
    }

    Write-Output '静的検査を実行しています。'
    & go vet -mod=readonly ./...
    if ($LASTEXITCODE -ne 0) {
        throw '静的検査に失敗しました。'
    }

    Write-Output 'テストを実行しています。'
    & go test -mod=readonly -count=1 ./...
    if ($LASTEXITCODE -ne 0) {
        throw 'テストに失敗しました。'
    }

    & (Join-Path $PSScriptRoot 'build.ps1')

    Write-Output '実行ファイルの版表示とヘルプを確認しています。'
    $versionText = @(& ./dist/workscope.exe --version)
    if ($LASTEXITCODE -ne 0 -or $versionText.Count -ne 1 -or
        $versionText[0] -notmatch '^workscope \d+\.\d+\.\d+$') {
        throw '実行ファイルの版表示が不正です。'
    }
    $helpText = @(& ./dist/workscope.exe --help)
    if ($LASTEXITCODE -ne 0 -or ($helpText -join "`n") -notmatch '使い方') {
        throw '実行ファイルのヘルプ表示が不正です。'
    }

    Write-Output '一時リポジトリで実行ファイルの受入確認を行っています。'
    & (Join-Path $PSScriptRoot 'acceptance.ps1')

    Write-Output "検証に成功しました。$($versionText[0])"
}
finally {
    $env:GOTOOLCHAIN = $previousToolchain
    Pop-Location
}
