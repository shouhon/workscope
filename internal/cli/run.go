// Package cli は引数、表示、終了コードを扱う。
package cli

import (
	"fmt"
	"io"

	"workscope/internal/version"
)

const help = `workscope - 作業開始後の変更が許容範囲内か照合するCLI

使い方（現在利用できる機能）:
  workscope --help / -h    この説明を表示する
  workscope --version      版番号を表示する

開発予定（現在は未実装）:
  workscope begin --allow <対象> [--allow <対象> ...] [--json]
  workscope begin --replace --allow <対象> [--allow <対象> ...] [--json]
  workscope check [--json]

実装する照合範囲:
  Gitルートを基準に、追跡中のファイルと無視されていない未追跡ファイルを扱う。
  --allow はルートからの相対パスで指定する。ワイルドカードは扱わない。
  無視された未追跡ファイル、Gitの管理領域、.workscope/ は対象外とする。
  サブモジュールは判定対象外とし、存在するときは完全な照合と扱わない。

開始記録の保存予定先: <Gitルート>/.workscope/baseline.json
Gitの設定や .gitignore は自動変更しない。
状態をGitから除外するには、利用者が .gitignore に /.workscope/ を追加する。

セットアップ段階のため、begin・check・JSON出力はまだ利用できない。
未対応の引数は終了コード64で拒否し、開始記録や対象ファイルを変更しない。
`

// Run はCLIを実行し、プロセスに返す終了コードを返す。
// 現段階では表示だけを行い、Gitの呼び出しやファイルの読み書きは行わない。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 {
		switch args[0] {
		case "--help", "-h":
			return writeOutput(stdout, stderr, help)
		case "--version":
			return writeOutput(stdout, stderr, "workscope "+version.Current+"\n")
		}
	}

	fmt.Fprintln(stderr, "引数が正しくないか、まだ対応していない機能です。workscope --help を確認してください。")
	return 64
}

func writeOutput(stdout, stderr io.Writer, text string) int {
	if _, err := io.WriteString(stdout, text); err != nil {
		fmt.Fprintln(stderr, "標準出力への書き込みに失敗しました。")
		return 2
	}
	return 0
}
