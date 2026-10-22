// Package cli は引数、表示、終了コードを扱う。
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"workscope/internal/baseline"
	"workscope/internal/version"
)

const help = `workscope - 作業開始後の変更が許容範囲内か照合するCLI

使い方:
  workscope --help / -h    この説明を表示する
  workscope --version      版番号を表示する
  workscope begin --allow <対象> [--allow <対象> ...] [--json]
  workscope begin --replace --allow <対象> [--allow <対象> ...] [--json]

開発予定（現在は未実装）:
  workscope check [--json]

記録する照合範囲:
  Gitルートを基準に、追跡中のファイルと無視されていない未追跡ファイルを扱う。
  --allow はルートからの相対パスで指定する。ワイルドカードは扱わない。
  無視された未追跡ファイル、Gitの管理領域、.workscope/ は対象外とする。
  サブモジュールは判定対象外とし、存在するときは完全な照合と扱わない。

開始記録の保存先: <Gitルート>/.workscope/baseline.json
Gitの設定や .gitignore は自動変更しない。
状態をGitから除外するには、利用者が .gitignore に /.workscope/ を追加する。

既存記録は --replace なしで上書きしない。別用途の .workscope は使用しない。
リンクは参照先の文字列だけを記録し、リンク配下は --allow に指定できない。
これから作るパスも指定できる。パスは現在地ではなくGitルートを基準にする。
check はまだ利用できないため、この版では開始後の変更を照合できない。
終了コード: 0=記録成功、2=記録不能、64=引数の誤り。
`

// Run はCLIを実行し、プロセスに返す終了コードを返す。
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 {
		switch args[0] {
		case "--help", "-h":
			return writeOutput(stdout, stderr, help)
		case "--version":
			return writeOutput(stdout, stderr, "workscope "+version.Current+"\n")
		}
	}
	if len(args) > 0 && args[0] == "begin" {
		return runBegin(args[1:], stdout, stderr)
	}

	fmt.Fprintln(stderr, "引数が正しくないか、まだ対応していない機能です。workscope --help を確認してください。")
	return 64
}

func runBegin(args []string, stdout, stderr io.Writer) int {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}
	allow := []string{}
	replace := false
	var argumentError string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--json":
		case arg == "--replace":
			replace = true
		case arg == "--allow":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
				argumentError = "--allow の対象がありません。"
				break
			}
			i++
			allow = append(allow, args[i])
		case strings.HasPrefix(arg, "--allow="):
			allow = append(allow, strings.TrimPrefix(arg, "--allow="))
		default:
			argumentError = "begin の引数が正しくありません。workscope --help を確認してください。"
		}
		if argumentError != "" {
			break
		}
	}
	result := baseline.NewResult()
	code := 64
	if argumentError != "" {
		result.Status = "invalid_arguments"
		result.Coverage.Complete = false
		result.Issues = append(result.Issues, baseline.Issue{Code: "invalid_arguments", Message: argumentError})
	} else {
		dir, err := os.Getwd()
		if err != nil {
			code = 2
			result.Coverage.Complete = false
			result.Issues = append(result.Issues, baseline.Issue{Code: "cwd_unavailable", Message: "現在の作業ディレクトリを確認できません。"})
		} else {
			result, code = baseline.Begin(dir, allow, replace)
		}
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintln(stderr, "標準出力への書き込みに失敗しました。")
			return 2
		}
		return code
	}
	if code != 0 {
		for _, issue := range result.Issues {
			fmt.Fprintln(stderr, issue.Message)
		}
		return code
	}
	return writeOutput(stdout, stderr, fmt.Sprintf("開始記録を保存しました。開始時の変更: %d件\n保存先: %s/.workscope/baseline.json\n照合対象: 追跡中・無視されていない未追跡ファイル、リンク自体、インデックス。\n対象外: 無視された未追跡ファイル、Git管理領域、状態保存先、サブモジュールの内容。\n", len(result.PreexistingChanges), result.Root))
}

func writeOutput(stdout, stderr io.Writer, text string) int {
	if _, err := io.WriteString(stdout, text); err != nil {
		fmt.Fprintln(stderr, "標準出力への書き込みに失敗しました。")
		return 2
	}
	return 0
}
