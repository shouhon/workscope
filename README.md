# workscope

作業開始時からの変化を照合し、指定した範囲を外れた変更を見つける、Windows向けのコマンドラインツールです。

AIに修正を任せるときなどに、開始前からある未コミット変更と、作業中に加わった変更を区別できます。たとえば `src` だけを変更する予定なら、別のファイルが再編集されていないかを確認できます。

[導入](#インストール) · [使い方](#使い方) · [利用ガイド](docs/usage.md) · [Releases](https://github.com/shouhon/workscope/releases) · [MITライセンス](LICENSE)

## 主な機能

- ファイル・ディレクトリ・これから作るパスを、作業で変更してよい範囲として指定。
- 開始前の変更を保持したまま、開始後の追加・再編集・削除・種類変更を検出。
- ステージした内容の変化も、作業ファイルとは別に照合。
- 日本語表示とJSON出力、終了コードによる自動処理に対応。
- 通常のGitリポジトリと追加の作業ツリー（worktree）で利用可能。

ファイル本文は保存せず、SHA-256とGitの状態を記録します。照合時は対象ファイル・Git設定・開始記録を変更しません。

## 動作環境

- Windows 11（配布する実行ファイルはx64／amd64用）
- Git for Windows（`git` コマンドが利用できること）

配布済みの実行ファイルを使う場合、Goは不要です。

## インストール

ソースコードは[GitHub](https://github.com/shouhon/workscope)、実行ファイルは[GitHub Releases](https://github.com/shouhon/workscope/releases)で配布します。

### Scoopを使う

[Scoop](https://scoop.sh/)を導入済みのPowerShellで実行します。必要なGitもScoop経由で導入されます。

```powershell
scoop install https://github.com/shouhon/workscope/releases/latest/download/workscope.json
workscope --version
```

Scoop定義のURLを直接指定する方法です。更新・アンインストールは次のとおりです。

```powershell
scoop update workscope
scoop uninstall workscope
```

### ZIPを使う

1. [Releases](https://github.com/shouhon/workscope/releases)から `workscope_<版番号>_windows_amd64.zip` と `checksums.txt` をダウンロードします。
2. `Get-FileHash -Algorithm SHA256 <ZIPのパス>` の値が `checksums.txt` と一致することを確認し、ZIPを展開します。
3. `workscope.exe` を任意のフォルダーへ置き、そのフォルダーを `PATH` へ追加するか、実行ファイルの絶対パスで呼び出します。

更新時は新しいZIPを同じ手順で確認して配置します。ZIPにはMITライセンスを同梱します。

## 使い方

対象のGitリポジトリ内で、作業前に開始記録を作ります。`--allow` は常にGitルートからの相対パスです。

```powershell
workscope begin --allow src --allow AGENTS.md
if ($LASTEXITCODE -ne 0) { throw '開始記録を作成できませんでした。' }

# ここで対象プロジェクトを編集する
workscope check
$LASTEXITCODE
```

開始前から `config.toml` に変更があっても、その後に再編集しなければ範囲外の変更には含めません。開始後に再編集すると検出します。`src` を許容しても、`src2` は範囲外です。

開始記録は `<Gitルート>/.workscope/baseline.json` に保存します。対象プロジェクトの `.gitignore` へ `/.workscope/` を追加してください。workscopeが自動で追記することはありません。

JSONで結果を受け取る場合や、開始記録を明示的に取り直す場合は次を使います。

```powershell
workscope check --json
workscope begin --replace --allow src --allow AGENTS.md
```

| 終了コード | 意味 |
| --- | --- |
| `0` | 記録成功、または照合完了・範囲外の変更なし |
| `1` | 範囲外の変更あり |
| `2` | 記録・読み取り・照合ができず判定不能 |
| `64` | 引数の誤り |

範囲外の変更と判定不能が同時にある場合は `2` で終了し、両方を報告します。

リンク先の内容は走査しません。サブモジュール、HEADの変更、読めない対象などがある場合は完全な照合と扱いません。変更者の特定や、途中で変更して元へ戻した操作の検出は保証しません。詳しくは[利用ガイド](docs/usage.md)を参照してください。

## ソースからビルド

[go.mod](go.mod)の指定を満たすGoと、PowerShell 7以降が必要です。標準ライブラリだけを使い、外部Goモジュールはありません。

```powershell
git clone https://github.com/shouhon/workscope.git
Set-Location workscope
./scripts/build.ps1
./dist/workscope.exe --version
./dist/workscope.exe --help
```

別のプロジェクトでは、生成した実行ファイルの絶対パスで呼び出すか、`PATH` の通ったフォルダーへ配置してください。

## 開発・貢献

変更後はプロジェクトのルートで必須検証を実行します。整形、静的検査、テスト、Windowsビルド、実行ファイルの受入確認を行います。

```powershell
./scripts/verify.ps1
```

不具合報告・改善提案は[Issues](https://github.com/shouhon/workscope/issues)へお願いします。コードを変更する際は[作業指示](AGENTS.md)を確認してください。版番号の正本は `internal/version/version.go` です。

## ドキュメント

- [利用ガイド](docs/usage.md)：許容範囲、記録の取り直し、JSON、PowerShellでの詳しい実行例
- [配布手順](docs/distribution.md)：GitHub Releases用ZIP・Scoop定義の作成と公開後の確認
- [要件定義書](docs/requirements.md)：出力・保存形式と受入条件
- [構成と処理の境界](docs/architecture.md)：パッケージの責務と読み書きの範囲
- [開発計画](docs/development-plan.md)・[受入確認](docs/acceptance.md)：実装工程と検証結果

## ライセンス

[MIT License](LICENSE)
