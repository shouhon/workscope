package cli

const unknownExitDiagnostic = `workscope自身の終了コード: 2（判定不能）。1（範囲外の変更あり）とは区別してください。
pwsh -Command の外側では1になることがあります。非対話実行ではCLI直後に $workscopeExit = $LASTEXITCODE を保持し、最後に exit $workscopeExit で返してください。
対話中は $LASTEXITCODE を確認してください。exit はシェルを終了します。実行例: workscope check --help
`

const powershellExitHelp = `
PowerShellで終了コードを受け取る:
  workscope自身の終了コードと、呼び出し元のPowerShellの終了コードは別。
  pwsh -Command 経由では、判定不能の2が外側で1になることがある。
  非対話実行ではCLIの直後に $LASTEXITCODE を保持し、最後に同じ値を返す。
  事前にbeginしたタスクの名前へ置き換え、対象のGit作業ディレクトリで実行する。
  pwsh -NoProfile -NonInteractive -Command 'workscope check --task "今回の作業" --json-summary --no-progress; $workscopeExit = $LASTEXITCODE; exit $workscopeExit'
  JSONを加工する場合も、加工前に保持した $workscopeExit を最後に返す。
  対話中は $LASTEXITCODE を確認する。exit は実行中のシェルを終了するため使わない。
  1=範囲外の変更あり、2=判定不能。JSONの status と coverage.complete も確認する。
  判定不能時はJSON指定・--no-progress指定でも、終了コードと対処を標準エラーへ表示する。
  標準エラーの診断を捨てず、JSONの標準出力へ結合しない。
`
