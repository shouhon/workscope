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
check は開始前の変更と開始後の内容・種類・インデックスの変化を分けて表示する。
check は開始記録を更新しない。ディレクトリと未作成の許容パスは配下を含む。
終了コード: 0=記録成功・範囲外の変更なし、1=範囲外の変更あり、2=判定不能、64=引数の誤り。
begin は記録成功で0、記録不能で2、引数の誤りで64を返す。1はcheckの範囲外検出だけに使う。
Gitの失敗時は操作・起動エラー・Gitの終了コード・伏字にした標準エラーを表示する。
範囲外の変更と判定不能が同時にある場合は、両方を表示して2を返す。
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
	if len(args) > 0 && args[0] == "check" {
		return runCheck(args[1:], stdout, stderr)
	}

	fmt.Fprintln(stderr, "引数が正しくないか、まだ対応していない機能です。workscope --help を確認してください。")
	return 64
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	result := baseline.NewResult()
	result.Command = "check"
	jsonOutput, invalid := false, false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		} else {
			invalid = true
		}
	}
	code := 2
	if invalid {
		code = 64
		result.Status = "invalid_arguments"
		result.Issues = append(result.Issues, baseline.Issue{Code: "invalid_arguments", Message: "check の引数が正しくありません。workscope --help を確認してください。"})
	} else if dir, err := os.Getwd(); err != nil {
		result.Issues = append(result.Issues, baseline.Issue{Code: "cwd_unavailable", Message: "現在の作業ディレクトリを確認できません。"})
	} else {
		result, code = baseline.Check(dir)
	}
	if code == 2 || code == 64 {
		result.Coverage.Complete = false
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintln(stderr, "標準出力への書き込みに失敗しました。")
			return 2
		}
		return code
	}
	if code == 64 {
		for _, issue := range result.Issues {
			fmt.Fprintln(stderr, issue.Message)
		}
		return code
	}
	var text strings.Builder
	switch result.Status {
	case "within_scope":
		text.WriteString("照合が完了しました。範囲外の変更はありません。\n")
	case "out_of_scope":
		text.WriteString("範囲外の変更があります。\n")
	default:
		text.WriteString("判定不能の対象があります。完全な照合はできません。\n")
	}
	fmt.Fprintf(&text, "開始前からあった変更: %d件\n", len(result.PreexistingChanges))
	for _, change := range result.PreexistingChanges {
		fmt.Fprintf(&text, "  %q (Git状態: %s)\n", change.Path, change.Status)
	}
	fmt.Fprintf(&text, "開始後の変更: %d件\n", len(result.ChangesSinceBegin))
	labels := map[string]string{"added": "追加", "modified": "変更", "deleted": "削除", "type_changed": "種類変更"}
	scope := func(within bool) string {
		if within {
			return "範囲内"
		}
		return "範囲外"
	}
	for _, change := range result.ChangesSinceBegin {
		fmt.Fprintf(&text, "  %s: %q (%s)\n", labels[change.Kind], change.Path, scope(change.WithinScope))
	}
	fmt.Fprintf(&text, "インデックスの変化: %d件\n", len(result.IndexChanges))
	for _, change := range result.IndexChanges {
		fmt.Fprintf(&text, "  %s: %q (%s)\n", labels[change.Kind], change.Path, scope(change.WithinScope))
	}
	fmt.Fprintf(&text, "範囲外の変更: %d件（作業ファイルとインデックスを別々に集計）\n", len(result.OutOfScopeChanges))
	fmt.Fprintf(&text, "判定不能の理由: %d件\n", len(result.Issues))
	for _, issue := range result.Issues {
		fmt.Fprintf(&text, "  %s", issue.Message)
		if issue.Path != "" {
			fmt.Fprintf(&text, " (%q)", issue.Path)
		}
		text.WriteByte('\n')
	}
	text.WriteString("照合対象: 追跡中・無視されていない未追跡ファイル、リンク自体、インデックス。\n対象外: 無視された未追跡ファイル、Git管理領域、状態保存先、サブモジュールの内容。\n")
	if writeOutput(stdout, stderr, text.String()) != 0 {
		return 2
	}
	return code
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
