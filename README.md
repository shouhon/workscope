# workscope

作業開始時の状態と現在の状態を照合し、指定した作業範囲を外れた変更を報告する、Windows向けのCLIです。開始前からある未コミット変更と、その後の変更を区別することを目的とします。

`begin` による開始記録と、`check` による内容・種類・インデックスの照合が利用できます。開始前からある変更と開始後の再編集を区別し、日本語またはJSONで結果を返します。記録にはSHA-256とGit状態を保存し、ファイル本文は保存しません。

## 開始記録を作る

ビルドした実行ファイルを、記録したいGit作業ディレクトリ内で実行します。`--allow` は実行場所にかかわらずGitルートからの相対パスです。

```powershell
workscope begin --allow AGENTS.md --allow src --json
```

記録先は `<Gitルート>/.workscope/baseline.json` です。既に記録がある場合は終了コード2で停止します。明示的に取り直す場合だけ、次を実行します。

```powershell
workscope begin --replace --allow AGENTS.md --allow src
```

ファイルとディレクトリに加え、これから作るパスも指定できます。ワイルドカード、ルート外、Git管理領域、状態保存先、リンクを親に含むパスは指定できません。Windowsの区切り・大小文字を扱い、同じ対象の重複指定をまとめます。サブモジュールや未対応のジャンクションがある場合、読めない対象がある場合、取得中に状態が変わる場合は記録せず終了コード2を返します。シンボリックリンクは参照先の文字列だけを記録し、参照先の内容は読みません。

追跡中のファイルは無視設定に一致しても対象です。無視された未追跡ファイル、Git管理領域、状態保存先は除外します。通常の作業ディレクトリとworktreeに対応し、対象ファイル・インデックス・Git設定を変更しません。`.workscope` に正常な自分の記録以外がある場合は、`--replace` でも変更しません。

状態保存先をGitから除外するには、利用者が `.gitignore` に `/.workscope/` を追加してください。CLIは自動変更しません。`--json` では標準出力にJSONオブジェクト1件を出力します。記録成功は0、記録不能は2、引数の誤りは64で終了します。

## 開始後の変更を照合する

```powershell
workscope begin --allow AGENTS.md --allow src
# 指定した範囲で作業する
workscope check
workscope check --json
$LASTEXITCODE
```

例えば開始前から `config.toml` に変更があっても、開始後に再編集しなければ範囲外の変更には含めません。開始後に同じファイルを再編集すると、Gitの変更済みの印が同じでも検出します。許容した `src` の配下は範囲内、`src2` の配下は範囲外です。これから作る許容パスは配下も含み、既存のファイルを許容した場合は、そのパスだけを含みます。

`check` は開始記録を更新しません。ファイルの追加・変更・削除・種類変更を報告し、ステージした内容を元に戻した場合も、インデックスのモード・オブジェクトID・競合の段階を別に比較します。名前変更は削除と追加として報告します。

| 終了コード | 照合結果 |
| --- | --- |
| 0 | 完全に照合でき、範囲外の変更なし。範囲内の変更はあってよい |
| 1 | 範囲外の変更あり |
| 2 | 記録なし・破損・別ルート・HEAD変更・読取失敗・サブモジュールなどで判定不能 |
| 64 | 引数の誤り |

範囲外の変更と判定不能が同時にある場合は、確認できた変更と問題の両方を報告して2で終了します。読めない対象を変更なしや削除と決めつけません。開始時の対象が無視された未追跡ファイルへ変わった場合は、内容を読まず判定不能とします。開始後に状態が変わり続ける場合も、完全な照合としません。

JSONの固定項目と意味は[要件定義書](docs/requirements.md#5-出力と終了コード)を参照してください。`changes_since_begin` の各項目は `path`、`source: "worktree"`、`kind`、`before_kind`、`after_kind`、`before_sha256`、`after_sha256`、`within_scope` を持ちます。`kind` は `added`・`modified`・`deleted`・`type_changed` です。存在しない対象の種類は `missing`、ハッシュのない種類は空文字列で表します。

`index_changes` は `source: "index"`、パス、種類、範囲内かの判定と、記録時・現在のインデックス項目を `before`・`after` の配列で持ちます。`out_of_scope_changes` は範囲外だった両方の差分を含むため、同じパスが作業ファイルとインデックスにそれぞれ現れることがあります。各配列はパス順で安定させ、同じパスの範囲外差分は作業ファイル、インデックスの順です。

## 開発環境

- Windows 11
- Go：最低版は [go.mod](go.mod) の `go` 指定を参照
- PowerShell 7以降：開発用スクリプトの実行に使用
- Git for Windows：開始記録と、一時リポジトリでの受入テストで使用

Goの標準ライブラリだけを使います。外部モジュールがないため、依存パッケージのインストールと `go.sum` は不要です。モジュール名は、公開先を仮定しないローカル名 `workscope` としています。

## 実行と検証

プロジェクトのルートで実行します。

```powershell
go run ./cmd/workscope --help
go run ./cmd/workscope --version
```

整形確認、静的検査、テスト、ビルド、実行ファイルの起動確認をまとめて実行します。

```powershell
./scripts/verify.ps1
```

ビルドだけを実行する場合は次を使います。

```powershell
./scripts/build.ps1
./dist/workscope.exe --version
./dist/workscope.exe --help
```

両スクリプトはリポジトリのルートへ移動して実行し、終了時に元の作業場所へ戻ります。`GOTOOLCHAIN=local` を実行中だけ指定し、未導入のGoを自動取得しません。モジュール定義を勝手に更新しないよう、Goの検査とビルドには `-mod=readonly` を指定します。

検査の個別実行は次のとおりです。整形確認の対象は、`verify.ps1` が `go list` から取得する各パッケージのGoファイルです。

```powershell
go vet -mod=readonly ./...
go test -mod=readonly -count=1 ./...
go build -mod=readonly -trimpath -o ./dist/workscope.exe ./cmd/workscope
```

変更したGoファイルは `gofmt -w <ファイル>` で整形します。実際の終了コードを確認するときは、`go run` の代わりにビルドした実行ファイルを使用してください。

## 配置と生成物

| 場所 | 役割 |
| --- | --- |
| `cmd/workscope/` | プロセスの起動と終了コードの受け渡し |
| `internal/cli/` | 引数の受付、表示、対応するテスト |
| `internal/baseline/` | Gitルート・許容パス・対象の取得と開始記録の保存・照合 |
| `internal/version/version.go` | CLI版番号の正本 |
| `scripts/` | 開発用のビルドと検証 |
| `docs/` | 要件、開発計画、構成と処理の境界 |
| `dist/` | ローカルで作成するWindows実行ファイル。Git管理対象外 |
| `.cache/` | 必要に応じて使うローカルの作業用キャッシュ。Git管理対象外 |
| `.workscope/` | このリポジトリ自身を記録するときの状態保存先。Git管理対象外 |

環境変数によるアプリ設定や外部サービスへの接続はありません。開始記録の保存先は[要件定義書](docs/requirements.md)で定めています。開発用リポジトリには状態用の除外設定を用意していますが、CLIが利用者の `.gitignore` やGit設定を自動変更する設計にはしません。

初版の配布単位は `dist/workscope.exe` です。インストーラー、公開先、署名、自動配布は今回のセットアップに含めていません。ライセンスは未選択のため、配布条件や権利者の表記を推測して追加していません。

## 開発の基準

- [要件定義書](docs/requirements.md)：元の要件定義書を内容を変えずに移動した正本
- [開発計画](docs/development-plan.md)：工程ごとの完成条件、確認方法、現在地
- [構成と処理の境界](docs/architecture.md)：現在の構成と、後続実装で守る設計
- [エージェント向け指示](AGENTS.md)：変更範囲、必須検証、版番号の更新規則

構成とコマンドは、[Goのモジュール定義](https://go.dev/doc/modules/gomod-ref)、[Goのコマンド仕様](https://pkg.go.dev/cmd/go)、[Goの実行環境の選択](https://go.dev/doc/toolchain)を参照しています。
