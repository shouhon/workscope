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
  workscope begin --help / -h    開始記録の使い方を表示する
  workscope check --help / -h    照合の使い方を表示する
  workscope begin --allow <対象> [--allow <対象> ...] [--task <名前>] [--json | --json-summary]
  workscope begin --replace --allow <対象> [--allow <対象> ...] [--task <名前>] [--json | --json-summary]
  workscope check [--json | --json-summary] [--task <名前>]
  begin / check の共通指定: [--trust-repository <Gitルートの絶対パス>] [--verbose]

記録する照合範囲:
  Gitルートを基準に、追跡中のファイルと無視されていない未追跡ファイルを扱う。
  --allow はルートからの相対パスで指定する。ワイルドカードは扱わない。
  無視された未追跡ファイル、Gitの管理領域、.workscope/ は対象外とする。
  サブモジュールは判定対象外とし、存在するときは完全な照合と扱わない。

開始記録の保存先: <Gitルート>/.workscope/baseline.json
  --task を指定すると .workscope/task-<タスク名のSHA-256>.json に別々に保存する。
  begin と check で同じタスク名を指定する。省略時は従来の開始記録だけを扱う。
  日本語・空白を使える。大小文字は区別し、前後の空白・パス区切り・制御文字は拒否する。
Gitの設定や .gitignore は自動変更しない。
状態をGitから除外するには、利用者が .gitignore に /.workscope/ を追加する。

既存記録は --replace なしで上書きしない。取り直しは指定したタスクの記録だけに行う。
別用途の .workscope は使用しない。
リンクは参照先の文字列だけを記録し、リンク配下は --allow に指定できない。
これから作るパスも指定できる。パスは現在地ではなくGitルートを基準にする。
check は開始前の変更と開始後の内容・種類・インデックスの変化を分けて表示する。
check は開始記録を更新しない。ディレクトリと未作成の許容パスは配下を含む。
終了コード: 0=記録成功・範囲外の変更なし、1=範囲外の変更あり、2=判定不能、64=引数の誤り。
begin は記録成功で0、記録不能で2、引数の誤りで64を返す。1はcheckの範囲外検出だけに使う。
Gitの失敗時は操作・原因・次の対処を表示する。--verbose で伏字済みの詳細を表示する。
--json --verbose では issues[].details に詳細を含める。通常はこの項目を省略する。
--json-summary は変更の明細を省き、判定・照合範囲・件数・問題をJSONで返す。
完全に照合できない総件数はnull、確認できた変更件数は observed_counts に下限として示す。
--json と --json-summary は併用できない。
--trust-repository は確認して信頼したGitルート1件だけに、この実行中の信頼設定を渡す。
絶対パスを指定し、begin と check のそれぞれで指定する。恒久的なGit設定は変更しない。
長い処理では段階・確認済みファイル数・経過時間を約1秒ごとに標準エラーへ表示する。
範囲外の変更と判定不能が同時にある場合は、両方を表示して2を返す。
`

const beginHelp = `workscope begin - 作業開始時の状態と許容範囲を記録する

使い方:
  workscope begin --allow <対象> [--allow <対象> ...] [--task <名前>] [--replace] [--json | --json-summary]
                  [--trust-repository <Gitルートの絶対パス>] [--verbose]
  workscope begin --help / -h

引数:
  --allow <対象>  必須。Gitルートからの相対パス。複数指定できる。
                   ファイル・ディレクトリ・これから作るパスを指定する。
  --task <名前>   任意。タスク別の開始記録を使う。checkでも同じ名前を指定する。
  --replace       選んだ開始記録を明示的に取り直す。他の記録は変更しない。
  --json          結果を標準出力へJSONオブジェクト1件で返す。
  --json-summary  変更の明細を省き、判定・件数・問題をJSONで返す。--json と併用不可。
  --trust-repository <絶対パス>  信頼するGitルート1件を、この実行だけGitへ渡す。
                   所有者と内容を事前に確認する。恒久設定や開始記録には保存しない。
  --verbose       Git診断の詳細を表示する。--json では issues[].details に含める。

使用例:
  workscope begin --allow src --allow AGENTS.md
  workscope begin --task '文書修正' --allow docs --json
  workscope begin --task '機能追加' --allow src --json-summary
  workscope begin --task '文書修正' --replace --allow docs
  workscope begin --allow src --trust-repository 'D:\作業\対象プロジェクト'

既存記録は --replace なしでは上書きしない。
パスは現在地でなくGitルート基準。ワイルドカード、ルート外、管理領域は指定できない。
Git管理領域、.workscope/、無視された未追跡ファイル、リンク先の内容は走査しない。
状態保存先の除外は、利用者が .gitignore に /.workscope/ を追加する。
長い処理の進捗は標準エラーへ表示し、JSONには混ぜない。
要約の未確認の総件数はnullとし、確認できた変更件数は observed_counts に下限として示す。
終了コード: 0=記録成功、2=記録不能、64=引数の誤り。
`

const checkHelp = `workscope check - 開始記録と現在の状態を照合する

使い方:
  workscope check [--task <名前>] [--json | --json-summary] [--trust-repository <Gitルートの絶対パス>] [--verbose]
  workscope check --help / -h

引数:
  必須の引数はない。事前に begin --allow <対象> で開始記録を作る。
  --task <名前>  beginと同じ名前の記録を選ぶ。省略時は従来の記録を使う。
  --json         結果を標準出力へJSONオブジェクト1件で返す。
  --json-summary  変更の明細を省き、判定・件数・問題をJSONで返す。--json と併用不可。
  --trust-repository <絶対パス>  信頼するGitルート1件を、この実行だけGitへ渡す。
                  begin の指定は保存されないため、check でも指定する。
  --verbose      Git診断の詳細を表示する。--json では issues[].details に含める。

使用例:
  workscope check
  workscope check --task '文書修正' --json
  workscope check --task '機能追加' --json-summary
  workscope check --trust-repository 'D:\作業\対象プロジェクト' --verbose

開始前からある変更と開始後の変化、インデックスの変化を分けて表示する。
開始記録・対象ファイル・Git設定を更新しない。--allow と --replace は指定できない。
長い処理の進捗は標準エラーへ表示し、JSONには混ぜない。
要約の未確認の総件数はnullとし、確認できた変更件数は observed_counts に下限として示す。
終了コード: 0=範囲外の変更なし、1=範囲外の変更あり、2=判定不能、64=引数の誤り。
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
	jsonOutput, jsonSummary := false, false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
		if arg == "--json-summary" {
			jsonSummary = true
		}
	}
	task, taskSet := "", false
	trust, trustSet, verbose := "", false, false
	var argumentError string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--help", arg == "-h":
			return writeOutput(stdout, stderr, checkHelp)
		case arg == "--json", arg == "--json-summary":
		case arg == "--verbose":
			verbose = true
		case arg == "--trust-repository", strings.HasPrefix(arg, "--trust-repository="):
			if trustSet {
				argumentError = "--trust-repository は1つだけ指定してください。"
				break
			}
			trustSet = true
			if arg == "--trust-repository" {
				if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
					argumentError = "--trust-repository の絶対パスがありません。"
					break
				}
				i++
				trust = args[i]
			} else {
				trust = strings.TrimPrefix(arg, "--trust-repository=")
			}
			if trust == "" {
				argumentError = "--trust-repository の絶対パスがありません。"
			}
		case arg == "--task", strings.HasPrefix(arg, "--task="):
			if taskSet {
				argumentError = "--task は1つだけ指定してください。"
				break
			}
			taskSet = true
			if arg == "--task" {
				if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
					argumentError = "--task の名前がありません。"
					break
				}
				i++
				task = args[i]
			} else {
				task = strings.TrimPrefix(arg, "--task=")
			}
			if task == "" {
				argumentError = "--task の名前がありません。"
			}
		default:
			argumentError = "check の引数が正しくありません。workscope --help を確認してください。"
		}
		if argumentError != "" {
			break
		}
	}
	if argumentError == "" && jsonOutput && jsonSummary {
		argumentError = "--json と --json-summary は併用できません。"
	}
	result.Task = task
	code := 2
	if argumentError != "" {
		code = 64
		result.Status = "invalid_arguments"
		result.Issues = append(result.Issues, baseline.Issue{Code: "invalid_arguments", Message: argumentError})
	} else if dir, err := os.Getwd(); err != nil {
		result.Issues = append(result.Issues, baseline.Issue{Code: "cwd_unavailable", Message: "現在の作業ディレクトリを確認できません。"})
	} else {
		result, code = withProgress(stderr, func(observer baseline.Observer) (baseline.Result, int) {
			return baseline.CheckWithOptions(dir, task, baseline.Options{TrustRepository: trust}, observer)
		})
	}
	if code == 2 || code == 64 {
		result.Coverage.Complete = false
	}
	selectDiagnostics(&result, verbose)
	if jsonOutput || jsonSummary {
		var output any = result
		if jsonSummary {
			output = summarize(result)
		}
		if err := json.NewEncoder(stdout).Encode(output); err != nil {
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
	if result.Task != "" {
		fmt.Fprintf(&text, "タスク: %q\n", result.Task)
	}
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
		if issue.Details != "" {
			fmt.Fprintf(&text, "  詳細:\n  %s\n", issue.Details)
		}
	}
	text.WriteString("照合対象: 追跡中・無視されていない未追跡ファイル、リンク自体、インデックス。\n対象外: 無視された未追跡ファイル、Git管理領域、状態保存先、サブモジュールの内容。\n")
	if writeOutput(stdout, stderr, text.String()) != 0 {
		return 2
	}
	return code
}

func runBegin(args []string, stdout, stderr io.Writer) int {
	jsonOutput, jsonSummary := false, false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
		if arg == "--json-summary" {
			jsonSummary = true
		}
	}
	allow := []string{}
	replace := false
	task, taskSet := "", false
	trust, trustSet, verbose := "", false, false
	var argumentError string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--help", arg == "-h":
			return writeOutput(stdout, stderr, beginHelp)
		case arg == "--json", arg == "--json-summary":
		case arg == "--verbose":
			verbose = true
		case arg == "--trust-repository", strings.HasPrefix(arg, "--trust-repository="):
			if trustSet {
				argumentError = "--trust-repository は1つだけ指定してください。"
				break
			}
			trustSet = true
			if arg == "--trust-repository" {
				if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
					argumentError = "--trust-repository の絶対パスがありません。"
					break
				}
				i++
				trust = args[i]
			} else {
				trust = strings.TrimPrefix(arg, "--trust-repository=")
			}
			if trust == "" {
				argumentError = "--trust-repository の絶対パスがありません。"
			}
		case arg == "--replace":
			replace = true
		case arg == "--task", strings.HasPrefix(arg, "--task="):
			if taskSet {
				argumentError = "--task は1つだけ指定してください。"
				break
			}
			taskSet = true
			if arg == "--task" {
				if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
					argumentError = "--task の名前がありません。"
					break
				}
				i++
				task = args[i]
			} else {
				task = strings.TrimPrefix(arg, "--task=")
			}
			if task == "" {
				argumentError = "--task の名前がありません。"
			}
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
	if argumentError == "" && jsonOutput && jsonSummary {
		argumentError = "--json と --json-summary は併用できません。"
	}
	result := baseline.NewResult()
	result.Task = task
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
			result, code = withProgress(stderr, func(observer baseline.Observer) (baseline.Result, int) {
				return baseline.BeginWithOptions(dir, allow, replace, task, baseline.Options{TrustRepository: trust}, observer)
			})
		}
	}
	selectDiagnostics(&result, verbose)
	if jsonOutput || jsonSummary {
		var output any = result
		if jsonSummary {
			output = summarize(result)
		}
		if err := json.NewEncoder(stdout).Encode(output); err != nil {
			fmt.Fprintln(stderr, "標準出力への書き込みに失敗しました。")
			return 2
		}
		return code
	}
	if code != 0 {
		for _, issue := range result.Issues {
			fmt.Fprintln(stderr, issue.Message)
			if issue.Details != "" {
				fmt.Fprintln(stderr, "詳細:\n"+issue.Details)
			}
		}
		return code
	}
	var text strings.Builder
	if result.Task != "" {
		fmt.Fprintf(&text, "タスク: %q\n", result.Task)
	}
	fmt.Fprintf(&text, "開始記録を保存しました。開始時の変更: %d件\n保存先: %s/.workscope/%s\n照合対象: 追跡中・無視されていない未追跡ファイル、リンク自体、インデックス。\n対象外: 無視された未追跡ファイル、Git管理領域、状態保存先、サブモジュールの内容。\n", len(result.PreexistingChanges), result.Root, baseline.RecordName(task))
	return writeOutput(stdout, stderr, text.String())
}

func selectDiagnostics(result *baseline.Result, verbose bool) {
	if !verbose {
		for i := range result.Issues {
			result.Issues[i].Details = ""
		}
	}
}

func writeOutput(stdout, stderr io.Writer, text string) int {
	if _, err := io.WriteString(stdout, text); err != nil {
		fmt.Fprintln(stderr, "標準出力への書き込みに失敗しました。")
		return 2
	}
	return 0
}
