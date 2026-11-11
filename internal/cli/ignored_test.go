package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

func TestIncludeIgnoredCLIFormatsAndScope(t *testing.T) {
	for _, format := range []string{"text", "--json", "--json-summary"} {
		t.Run(format, func(t *testing.T) {
			dir := summaryRepo(t)
			if err := os.Mkdir(filepath.Join(dir, "logs"), 0o755); err != nil {
				t.Fatal(err)
			}
			summaryWrite(t, dir, "logs/first", "開始時")
			summaryWrite(t, dir, "logs/second", "開始時")
			started, code := baseline.Begin(dir, []string{".gitignore", "logs/first"}, false, "除外設定")
			if code != 0 {
				t.Fatalf("準備失敗: %d %+v", code, started)
			}
			summaryWrite(t, dir, ".gitignore", "/logs/\n")
			summaryWrite(t, dir, "logs/first", "許容範囲内の変更")
			summaryGit(t, dir, "add", ".gitignore")
			t.Chdir(filepath.Join(dir, "logs"))
			ordinary := runSummary(t, []string{"check", "--task", "除外設定", "--no-progress"}, 2)
			if len(ordinary.Issues) != 2 || !strings.Contains(ordinary.Issues[0].Message, "check --include-ignored") || !strings.Contains(ordinary.Issues[0].Message, "workscope check --help") {
				t.Fatalf("既定の診断から復旧手順へ到達できません: %+v", ordinary)
			}
			paths := []string{filepath.Join(dir, ".workscope", baseline.RecordName("除外設定")), filepath.Join(dir, ".git", "index"), filepath.Join(dir, ".git", "config")}
			before := map[string][]byte{}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = data
			}
			for _, wantCode := range []int{0, 1} {
				if wantCode == 1 {
					summaryWrite(t, dir, "logs/second", "許容外の変更")
				}
				args := []string{"check", "--task", "除外設定", "--include-ignored", "logs/first", "--include-ignored=logs/second", "--include-ignored", "./logs/first", "--no-progress"}
				if format != "text" {
					args = append(args, format)
				}
				var stdout, stderr bytes.Buffer
				if code := cli.Run(args, &stdout, &stderr); code != wantCode || stderr.Len() != 0 {
					t.Fatalf("明示指定の終了コードまたは診断が不正: %d %s / %s", code, &stdout, &stderr)
				}
				if format == "text" {
					for _, needed := range []string{"明示指定で照合に含める記録済みパス: 2件", "logs/first", "logs/second", "対象外: 明示指定以外の無視された未追跡ファイル"} {
						if !strings.Contains(stdout.String(), needed) {
							t.Fatalf("通常表示に照合範囲 %q がありません: %s", needed, &stdout)
						}
					}
				} else {
					var data struct {
						BaselineID string            `json:"baseline_id"`
						Task       string            `json:"task"`
						Coverage   baseline.Coverage `json:"coverage"`
						Counts     map[string]int    `json:"counts"`
						Changes    []baseline.Change `json:"changes_since_begin"`
					}
					decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
					if err := decoder.Decode(&data); err != nil {
						t.Fatal(err)
					}
					if err := decoder.Decode(new(any)); err != io.EOF {
						t.Fatalf("JSONが1件ではありません: %v", err)
					}
					if data.BaselineID != started.BaselineID || data.Task != "除外設定" || !data.Coverage.Complete || !reflect.DeepEqual(data.Coverage.IgnoredBaselinePaths, []string{"logs/first", "logs/second"}) {
						t.Fatalf("開始記録または照合範囲が不正: %+v", data)
					}
					if format == "--json-summary" && (data.Counts["out_of_scope_changes"] != wantCode || data.Counts["index_changes"] != 1 || data.Counts["changes_since_begin"] != 2+wantCode) {
						t.Fatalf("要約の件数が不正: %+v", data)
					}
					if format == "--json" && len(data.Changes) != 2+wantCode {
						t.Fatalf("明細の変更が不正: %+v", data)
					}
				}
			}
			for _, path := range paths {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before[path], after) {
					t.Fatalf("照合で開始記録かGitの状態を変更しました: %s %v", path, err)
				}
			}
		})
	}
}

func TestIncludeIgnoredCLIInvalidArguments(t *testing.T) {
	for _, args := range [][]string{
		{"check", "--include-ignored"},
		{"check", "--include-ignored="},
		{"check", "--include-ignored", "--no-progress"},
		{"begin", "--allow", ".", "--include-ignored=known"},
		{"list", "--include-ignored=known"},
	} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(append(args, "--json"), &stdout, &stderr); code != 64 || !strings.Contains(stdout.String(), `"status":"invalid_arguments"`) || stderr.Len() != 0 {
			t.Fatalf("不正な指定を受け付けました: %v %d %s / %s", args, code, &stdout, &stderr)
		}
	}
}
