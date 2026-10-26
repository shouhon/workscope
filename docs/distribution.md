# 配布手順

ソースコードは [shouhon/workscope](https://github.com/shouhon/workscope)、Windows用の実行ファイルは同じリポジトリの [GitHub Releases](https://github.com/shouhon/workscope/releases)で配布する。ライセンスは [MIT](../LICENSE) とする。導入方法はScoop、またはZIPの手動展開とする。

## mainへのpushによる自動公開

[検証とRelease](../.github/workflows/ci-release.yml)が、`main` へのpushと、`main` を対象にしたプルリクエストで必須検証を実行する。`shouhon/workscope` の `main` へのpushで `internal/version/version.go` の `version.Current` が上がったときだけ、検証した配布物をGitHub Releasesへ自動公開する。

1. push前のコミットとpush後のコミットから正本の版番号を読み取る。複数コミットをまとめてpushした場合も、全体の前後を比較する。
2. 版番号に変更があれば `scripts/package.ps1` を実行し、必須検証を通ったWindows用ZIP、Scoop定義、SHA-256一覧の3ファイルを保存する。
3. 公開専用のジョブが同じ実行の配布物を取得し、版番号・URL・ハッシュを再確認する。push後のコミットを対象に `v<版番号>` のタグとReleaseを作り、3ファイルを添付する。

版番号に変更がない場合や、正本のコメントだけが変わった場合は、検証だけを行う。プルリクエストと派生リポジトリでは公開しない。自動化の導入前の版は遡って公開しない。複数の版更新を一度にpushした場合は、push後の最終版だけを公開する。

ブランチの新規作成で比較元がない場合は公開しない。比較元の履歴を取得できない、不正な版番号、版番号の後退、必須検証の失敗はエラーとして停止する。版番号を自動で増やす処理はなく、更新条件は引き続き [AGENTS.md](../AGENTS.md#版番号の管理)に従う。

検証ジョブはリポジトリの読み取り権限だけを持ち、公開ジョブだけに `contents: write` を与える。認証にはGitHubが用意する `GITHUB_TOKEN` を使い、個人用トークンや追加のシークレットは必要としない。リポジトリや組織の設定でActionsや利用する公式アクションが禁止されている場合は、管理者による設定が必要になる。[権限の仕様](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#permissions)を参照する。

公開ジョブは[同時実行の待ち行列](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#concurrency)へ最大100件を並べ、実行中の公開を新しいpushで中断しない。検証の完了順が逆転した場合は、公開済みの最新版より古い版を「最新版」に設定しない。これにより `releases/latest/download/workscope.json` が旧版へ戻ることを防ぐ。

### 失敗と再実行

- 検証や通信で失敗した場合は、GitHub Actionsの元の実行から失敗したジョブを再実行する。版番号を変えない新しいpushは公開の再試行にならない。
- 同じタグのReleaseが公開済みなら、その版を変更せず成功として終了する。既存の添付を上書きしない。
- 同じタグが別コミットを指している場合は停止する。タグを移動せず、対象を確認して新しい版番号で公開する。
- 添付の途中失敗などで同じ版の下書きが残った場合も停止する。下書きの対象コミットと添付を確認し、下記の手動公開の条件を満たしてから復旧する。自動で削除・差し替えは行わない。
- 検証を待つ間に `main` の `.github/workflows/` が変更され、公開対象コミットと内容が異なった場合は、GitHubの権限制限で公開が失敗することがある。`GITHUB_TOKEN` には必要なワークフロー変更権限を追加できない。検証した対象コミットを別のコミットへ差し替えず、手動公開を依頼するか、現在の `main` で新しい版番号に更新してpushする。[Release APIの制限](https://docs.github.com/en/rest/releases/releases#create-a-release)を参照する。

[release-plan.ps1](../scripts/release-plan.ps1)は現在の作業ディレクトリのGit履歴を読み取るだけで、タグや開始記録を変更しない。[publish-release.ps1](../scripts/publish-release.ps1)は実際の外部公開を行うため、手元で検証目的に実行しない。公開処理の自動テストはAPIと `gh` を置き換えた [test-release.ps1](../scripts/test-release.ps1)で行い、必須検証へ組み込んでいる。

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

## 手動で公開する

自動公開の条件以外で手動公開する場合は、対象版のタグ・Release公開の明示依頼がある場合だけ行う。配布物を作成したコミットとタグの参照先を一致させ、次を確認する。

1. 必須検証が成功し、作業ツリーに未コミットの変更がない。
2. ZIP直下に `workscope.exe` とMITライセンスがあり、展開後の版表示が正本と一致する。
3. ZIPのSHA-256が `workscope.json` と `checksums.txt` の両方に一致する。
4. タグ名、Scoop定義の版番号、ZIP名、ZIP URLが同じ版を指している。
5. 同じタグのReleaseがまだ公開されていない。公開済みの同じ版へ別の内容を差し替えない。

GitHubの [Releases](https://github.com/shouhon/workscope/releases) で対象コミットに `v<版番号>` のタグを指定し、上記3ファイルをすべて添付して公開する。通常の安定版として最新Releaseに設定する。下書き・プレリリースは、READMEに示す `releases/latest/download/` での導入対象にしない。

変更内容、対応OS、導入コマンドをReleaseの説明へ記す。署名は現時点では行わない。

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
