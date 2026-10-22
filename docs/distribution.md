# 配布手順

ソースコードは [shouhon/workscope](https://github.com/shouhon/workscope)、Windows用の実行ファイルは同じリポジトリの [GitHub Releases](https://github.com/shouhon/workscope/releases)で配布する。ライセンスは [MIT](../LICENSE) とする。導入方法はScoop、またはZIPの手動展開とする。

## 配布物を作成する

Windowsのamd64環境で、[README](../README.md#ソースからビルド)の開発環境を用意する。公開対象の変更をコミットし、作業ツリーが空であることを確認してから、リポジトリのルートで実行する。

```powershell
./scripts/package.ps1
```

このスクリプトは `verify.ps1` による必須検証を実行し、検証した実行ファイルから次を作成する。ネットワークへの接続、タグの作成、アップロードは行わない。既存の同名のローカル生成物は作り直す。

| ファイル | 内容 |
| --- | --- |
| `dist/workscope_<版番号>_windows_amd64.zip` | ZIP直下の `workscope.exe` と `LICENSE` |
| `dist/workscope.json` | Scoop用の定義。版ごとのReleaseのZIP URL、SHA-256、実行ファイル名、Gitへの依存を含む |
| `dist/checksums.txt` | ZIPとScoop定義のSHA-256 |

版番号は `internal/version/version.go` の `version.Current` から読み、実行ファイルの `--version` と一致しなければ停止する。タグ名は `v<版番号>` とする。版番号の更新条件は [AGENTS.md](../AGENTS.md#版番号の管理)を正本とし、生成した定義へ手で転記しない。

初回の配布対象はWindows 11のx64（amd64）とする。ARM64用・32ビット用は配布対象に含めない。GoやScoopを自動取得しない。生成物はすべてGit管理対象外の `dist/` に置く。

## 公開する

公開は別の操作であり、対象版のタグ・Release公開の明示依頼がある場合だけ行う。配布物を作成したコミットとタグの参照先を一致させ、次を確認する。

1. 必須検証が成功し、作業ツリーに未コミットの変更がない。
2. ZIP直下に `workscope.exe` とMITライセンスがあり、展開後の版表示が正本と一致する。
3. ZIPのSHA-256が `workscope.json` と `checksums.txt` の両方に一致する。
4. タグ名、Scoop定義の版番号、ZIP名、ZIP URLが同じ版を指している。
5. 同じタグのReleaseがまだ公開されていない。公開済みの同じ版へ別の内容を差し替えない。

GitHubの [Releases](https://github.com/shouhon/workscope/releases) で対象コミットに `v<版番号>` のタグを指定し、上記3ファイルをすべて添付して公開する。通常の安定版として最新Releaseに設定する。下書き・プレリリースは、READMEに示す `releases/latest/download/` での導入対象にしない。

初回公開時には、READMEの「初回Releaseの公開準備中」という案内を取り除く。変更内容、対応OS、導入コマンドをReleaseの説明へ記す。署名は現時点では行わない。

## Scoopでの導入・更新

[Scoopは公開URLの定義ファイルから導入できる](https://github.com/ScoopInstaller/Scoop/wiki/Creating-an-app-manifest#share-with-the-world)。本プロジェクトではバケットの追加や既存の公式バケットへの登録を必要としない。

```powershell
scoop install https://github.com/shouhon/workscope/releases/latest/download/workscope.json
workscope --version
scoop update workscope
```

最新版を指す同じURLで `workscope.json` を配信し、定義内のZIP URLは版ごとに固定する。ScoopはURL指定で導入したアプリの更新時に、[導入元URLを再利用する](https://github.com/ScoopInstaller/Scoop/blob/master/libexec/scoop-update.ps1)。定義の項目は [Scoopの仕様](https://github.com/ScoopInstaller/Scoop/wiki/App-Manifests)に従う。

## 公開後の確認と旧版への復帰

公開後はReleaseから配布物を改めて取得し、ハッシュ、展開後の起動・版表示、Scoop経由の導入を確認する。未公開URLへアクセスできたものとして扱わない。更新の確認は、旧版をScoopで導入した専用環境で新しい版への `scoop update workscope` を実行する。

旧版へ戻す場合は [Releasesの一覧](https://github.com/shouhon/workscope/releases)から目的の版のZIPと `checksums.txt` を取得し、ハッシュを確認して別のフォルダーへ展開する。その `workscope.exe` を絶対パスで呼び出せば、Scoopの導入状態を変えずに使用できる。

開始記録の読取互換性は版ごとに確認する。互換性を確認できない旧版で既存記録を取り直さない。`.workscope/` の削除や `begin --replace` は復帰手順に含めない。
