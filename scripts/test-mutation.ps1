#requires -Version 7.0

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
trap {
    Write-Error -ErrorRecord $_ -ErrorAction Continue
    exit 1
}

$cases = @(
    @{
        Name = '許容したファイルの配下へ範囲を広げる'
        Package = './internal/baseline'
        Test = 'TestCheckFileAllowDoesNotExpandAfterTypeChange'
        Before = 'allow.Kind != "file" && beneath(name, allow.Path)'
        After = 'beneath(name, allow.Path)'
    }
    @{
        Name = '種類が同じファイルの内容変更を見落とす'
        Package = './internal/baseline'
        Test = 'TestCheckSeparatesPreexistingChangesAndReediting'
        Before = 'old.Kind == now.Kind && old.SHA256 == now.SHA256'
        After = 'old.Kind == now.Kind'
    }
    @{
        Name = '別の診断がない場合のHEAD変更を見落とす'
        Package = './internal/baseline'
        Test = 'TestCheckUnknownAndOutOfScopeCoexist'
        Before = 'if current.Head != record.Head {'
        After = 'if current.Head != record.Head && len(current.Issues) > 0 {'
    }
    @{
        Name = 'インデックスだけの範囲外変更を成功扱いする'
        Package = './internal/baseline'
        Test = 'TestCheckReportsIndexOnlyChanges'
        Before = 'if len(result.OutOfScopeChanges) != 0 {'
        After = 'if len(result.OutOfScopeChanges) != 0 && len(result.ChangesSinceBegin) != 0 {'
    }
    @{
        Name = '同じ長さの開始記録への差し替えを見落とす'
        Package = './internal/baseline'
        Test = 'TestCheckRejectsSameLengthBaselineReplacement'
        Before = 'if err != nil || !bytes.Equal(data, confirmed) {'
        After = 'if err != nil || (len(data) != len(confirmed) && !bytes.Equal(data, confirmed)) {'
    }
    @{
        Name = '作業ファイルにも差分があるとインデックスの範囲外変更を落とす'
        Package = './internal/cli'
        Test = 'TestJSONSummaryCountsAndRecordProtection'
        Before = "for _, change := range result.IndexChanges {`n`t`tif !change.WithinScope {"
        After = "for _, change := range result.IndexChanges {`n`t`tif !change.WithinScope && len(result.ChangesSinceBegin) == 0 {"
    }
)

function Invoke-TargetTest($case, [string[]] $buildOptions) {
    $lines = @(& go test -mod=readonly -count=1 -json @buildOptions -run "^$($case.Test)$" $case.Package)
    $code = $LASTEXITCODE
    $events = @($lines | ConvertFrom-Json -AsHashtable)
    $testEvents = @($events | Where-Object { $_['Test'] -eq $case.Test -and $_['Action'] -in 'pass', 'fail' })
    if ($testEvents.Count -eq 1) {
        $action = $testEvents[0]['Action']
        if ($action -eq 'pass' -and $code -eq 0) { return 'pass' }
        if ($action -eq 'fail' -and $code -eq 1) { return 'fail' }
    }
    $lines | Write-Host
    throw "対象テストの結果を確認できませんでした。$($case.Test)（終了コード $code）"
}

$projectRoot = Split-Path -Parent $PSScriptRoot
$source = Join-Path $projectRoot 'internal/baseline/check.go'
$original = [IO.File]::ReadAllText($source).Replace("`r`n", "`n")
foreach ($case in $cases) {
    if ([regex]::Matches($original, [regex]::Escape($case.Before)).Count -ne 1) {
        throw "変異を適用する箇所を1件に特定できませんでした。$($case.Name)"
    }
}

$tempDirectory = Join-Path ([IO.Path]::GetTempPath()) ('workscope-mutation-' + [guid]::NewGuid().ToString('N'))
$null = New-Item -ItemType Directory -Path $tempDirectory
$mutatedFile = Join-Path $tempDirectory 'check.go'
$overlayFile = Join-Path $tempDirectory 'overlay.json'
$previousToolchain = $env:GOTOOLCHAIN
Push-Location -LiteralPath $projectRoot
try {
    $env:GOTOOLCHAIN = 'local'
    $overlay = @{ Replace = @{ $source = $mutatedFile } } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($overlayFile, $overlay)
    $missed = 0
    foreach ($case in $cases) {
        if ((Invoke-TargetTest $case) -ne 'pass') {
            throw "通常の実装で対象テストが失敗しました。$($case.Test)"
        }
        [IO.File]::WriteAllText($mutatedFile, $original.Replace($case.Before, $case.After))
        if ((Invoke-TargetTest $case -buildOptions @('-overlay', $overlayFile)) -eq 'fail') {
            Write-Output "検出 $($case.Name)（$($case.Test)）"
        } else {
            $missed++
            Write-Output "未検出 $($case.Name)（$($case.Test)）"
        }
    }
    if ($missed -gt 0) { throw "検出できなかった変異が $missed 件あります。" }
    Write-Output "変異 $($cases.Count) 件を検出しました。"
}
finally {
    $env:GOTOOLCHAIN = $previousToolchain
    Pop-Location
    [IO.File]::Delete($overlayFile)
    [IO.File]::Delete($mutatedFile)
    [IO.Directory]::Delete($tempDirectory)
}
exit 0
