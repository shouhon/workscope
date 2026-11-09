package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

func TestListOutputAndArguments(t *testing.T) {
	dir := summaryRepo(t)
	t.Chdir(dir)
	for _, task := range []string{"", "文書 修正"} {
		args := []string{"begin", "--allow", "input.txt"}
		if task != "" {
			args = append(args, "--task", task)
		}
		runSummary(t, args, 0)
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "--json"}, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("一覧の実行失敗: %d %s %s", code, &stdout, &stderr)
	}
	var result baseline.ListResult
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&result); err != nil || decoder.Decode(new(any)) != io.EOF {
		t.Fatalf("一覧がJSON1件ではありません: %v", err)
	}
	if result.SchemaVersion != 1 || result.Tool != "workscope" || result.Command != "list" || result.Status != "listed" || result.Root == "" || result.Issues == nil || len(result.Records) != 2 || !result.Records[0].Default || result.Records[1].Task != "文書 修正" {
		t.Fatalf("一覧の出力契約が不正: %+v", result)
	}
	stdout.Reset()
	if code := cli.Run([]string{"list"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "既定（--task 省略）") || !strings.Contains(stdout.String(), "文書 修正") || !strings.Contains(stdout.String(), "一覧は照合結果ではありません") {
		t.Fatalf("通常表示が不正: %d %s", code, &stdout)
	}
	for _, flags := range [][]string{{"--task", "名前"}, {"--replace"}, {"--allow", "src"}, {"--json-summary"}, {"--trust-repository="}, {"--trust-repository", dir, "--trust-repository", dir}, {"--trust-repository", "relative"}} {
		stdout.Reset()
		stderr.Reset()
		args := append([]string{"list", "--json"}, flags...)
		if code := cli.Run(args, &stdout, &stderr); code != 64 || !strings.Contains(stdout.String(), `"status":"invalid_arguments"`) || stderr.Len() != 0 {
			t.Fatalf("引数誤りが不正: %v %d %s %s", args, code, &stdout, &stderr)
		}
	}
	if code := cli.Run([]string{"list", "--json"}, failedWriter{}, &stderr); code != 2 {
		t.Fatalf("一覧の出力失敗を成功扱いしました: %d", code)
	}
	if err := os.WriteFile(filepath.Join(dir, ".workscope", "baseline.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"list", "--json"}, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), `"status":"unknown"`) || !strings.Contains(stdout.String(), `"code":"foreign_state"`) {
		t.Fatalf("破損を正常な空一覧にしました: %d %s %s", code, &stdout, &stderr)
	}
}

func TestListHelpWithoutGitOrState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", t.TempDir())
	for _, flag := range []string{"--help", "-h"} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run([]string{"list", flag}, &stdout, &stderr); code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "使用例:") {
			t.Fatalf("一覧のヘルプが不正: %d %s %s", code, &stdout, &stderr)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"list", "--json", "--verbose"}, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), `"code":"git_not_found"`) || !strings.Contains(stdout.String(), `"details":`) {
		t.Fatalf("一覧のGit診断が不正: %d %s %s", code, &stdout, &stderr)
	}
	children, err := os.ReadDir(dir)
	if err != nil || len(children) != 0 {
		t.Fatalf("一覧とヘルプが状態を作成しました: %v %v", children, err)
	}
}
