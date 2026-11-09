package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"workscope/internal/baseline"
)

const listHelp = `workscope list - 保存済みの開始記録を読み取り専用で一覧する

使い方:
  workscope list [--json] [--trust-repository <Gitルートの絶対パス>] [--verbose]
  workscope list --help / -h

引数:
  必須の引数はない。現在の作業ツリーの開始記録を一覧する。
  --json         タスク名・既定記録の区別・記録ID・作成日時・許容範囲の件数をJSONで返す。
  --trust-repository <絶対パス>  事前に確認したGitルート1件だけを、この実行で信頼する。
  --verbose      失敗時の伏字済みの診断詳細を表示する。

使用例:
  workscope list
  workscope list --json
  workscope check --task '文書修正'

開始記録・対象ファイル・Git設定を変更しない。変更ファイルの走査と照合は行わない。
HEADが変わった記録も一覧する。現在の変更が範囲内かどうかは check で確認する。
保存先がなければ0件。空の既存保存先・破損・読取不能・未対応形式では失敗する。
終了コード: 0=一覧取得成功、2=一覧取得不能、64=引数の誤り。
`

func runList(args []string, stdout, stderr io.Writer) int {
	result := baseline.ListResult{SchemaVersion: baseline.SchemaVersion, Tool: "workscope", Command: "list", Status: "unknown", Records: []baseline.RecordSummary{}, Issues: []baseline.Issue{}}
	jsonOutput, verbose, trustSet := false, false, false
	trust, argumentError := "", ""
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		}
	}
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case arg == "--help", arg == "-h":
			return writeOutput(stdout, stderr, listHelp)
		case arg == "--json":
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
		default:
			argumentError = "list の引数が正しくありません。workscope list --help を確認してください。"
		}
		if argumentError != "" {
			break
		}
	}
	code := 2
	if argumentError != "" {
		result.Status, code = "invalid_arguments", 64
		result.Issues = append(result.Issues, baseline.Issue{Code: "invalid_arguments", Message: argumentError})
	} else if dir, err := os.Getwd(); err != nil {
		result.Issues = append(result.Issues, baseline.Issue{Code: "cwd_unavailable", Message: "現在の作業ディレクトリを確認できません。"})
	} else {
		result, code = baseline.List(dir, baseline.Options{TrustRepository: trust})
	}
	for i := range result.Issues {
		if !verbose {
			result.Issues[i].Details = ""
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
			fmt.Fprintf(stderr, "%s", issue.Message)
			if issue.Path != "" {
				fmt.Fprintf(stderr, " (%q)", issue.Path)
			}
			fmt.Fprintln(stderr)
			if issue.Details != "" {
				fmt.Fprintln(stderr, "詳細:\n"+issue.Details)
			}
		}
		return code
	}
	var text strings.Builder
	fmt.Fprintf(&text, "保存済みの開始記録: %d件\n", len(result.Records))
	for _, record := range result.Records {
		label := fmt.Sprintf("タスク %q", record.Task)
		if record.Default {
			label = "既定（--task 省略）"
		}
		fmt.Fprintf(&text, "  %s | 記録ID: %s | 作成日時: %s | 許容範囲: %d件\n", label, record.BaselineID, record.CreatedAt, record.AllowCount)
	}
	text.WriteString("現在の変更の照合は check で行ってください。一覧は照合結果ではありません。\n")
	return writeOutput(stdout, stderr, text.String())
}
