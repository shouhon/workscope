package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"workscope/internal/cli"
)

const expectedUnknownExitDiagnostic = `workscope自身の終了コード: 2（判定不能）。1（範囲外の変更あり）とは区別してください。
pwsh -Command の外側では1になることがあります。非対話実行ではCLI直後に $workscopeExit = $LASTEXITCODE を保持し、最後に exit $workscopeExit で返してください。
対話中は $LASTEXITCODE を確認してください。exit はシェルを終了します。実行例: workscope check --help
`

func validCommandStderr(text string, code int) bool {
	if code == 2 {
		if !strings.HasSuffix(text, expectedUnknownExitDiagnostic) {
			return false
		}
		text = strings.TrimSuffix(text, expectedUnknownExitDiagnostic)
	}
	return validProgress(text)
}

func TestUnknownExitAdviceIsVisibleInEveryOutputMode(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, command := range []string{"begin", "check", "list"} {
		modes := []string{"", "--json"}
		if command != "list" {
			modes = append(modes, "--json-summary")
		}
		for _, mode := range modes {
			for _, quiet := range []bool{false, true} {
				if command == "list" && quiet {
					continue
				}
				args := []string{command}
				if command == "begin" {
					args = append(args, "--allow", "input.txt")
				}
				if mode != "" {
					args = append(args, mode)
				}
				if quiet {
					args = append(args, "--no-progress")
				}
				var stdout, stderr bytes.Buffer
				if code := cli.Run(args, &stdout, &stderr); code != 2 || !strings.HasSuffix(stderr.String(), expectedUnknownExitDiagnostic) {
					t.Fatalf("%v の終了コードと対処が表示されません: %d %s / %s", args, code, &stdout, &stderr)
				}
				if strings.Count(stderr.String(), "workscope自身の終了コード:") != 1 {
					t.Fatalf("%v の終了コードを重複表示しました: %s", args, &stderr)
				}
				if mode != "" {
					var result map[string]any
					decoder := json.NewDecoder(&stdout)
					if err := decoder.Decode(&result); err != nil || decoder.Decode(new(any)) != io.EOF || result["status"] != "unknown" || !validCommandStderr(stderr.String(), 2) {
						t.Fatalf("%v の単一JSONと診断が不正: %v %s / %s", args, err, &stdout, &stderr)
					}
				}
			}
		}
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("判定不能の案内で状態を作成しました: %v %v", entries, err)
	}
}

func TestHelpIncludesPowerShellExitAdviceWithoutGit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", t.TempDir())
	for _, prefix := range [][]string{nil, {"begin"}, {"check"}, {"list"}} {
		for _, flag := range []string{"--help", "-h"} {
			args := append(append([]string{}, prefix...), flag)
			var stdout, stderr bytes.Buffer
			if code := cli.Run(args, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("%v のヘルプに失敗しました: %d %s", args, code, &stderr)
			}
			for _, want := range []string{"pwsh -NoProfile -NonInteractive -Command", "$workscopeExit = $LASTEXITCODE", "exit $workscopeExit", "対話中は $LASTEXITCODE", "1=範囲外の変更あり、2=判定不能", "coverage.complete", "標準エラーの診断を捨てず"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("%v に %q がありません", args, want)
				}
			}
		}
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("ヘルプで状態を作成しました: %v %v", entries, err)
	}
}
