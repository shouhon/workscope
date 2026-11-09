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

func TestScopeExtensionCLIOutputAndReadOnlyCheck(t *testing.T) {
	for _, format := range []string{"text", "--json", "--json-summary"} {
		t.Run(format, func(t *testing.T) {
			dir := summaryRepo(t)
			summaryWrite(t, dir, "second.txt", "追加対象の開始点")
			summaryWrite(t, dir, "outside.txt", "範囲外の開始点")
			summaryGit(t, dir, "add", "second.txt", "outside.txt")
			summaryCommit(t, dir)
			started, code := baseline.Begin(dir, []string{"input.txt"}, false, "日本語 作業")
			if code != 0 {
				t.Fatalf("準備失敗: %d %+v", code, started)
			}
			summaryWrite(t, dir, "input.txt", "開始後の編集")
			summaryWrite(t, dir, "second.txt", "許容前の編集")
			summaryWrite(t, dir, "outside.txt", "許容しない編集")
			t.Chdir(dir)
			args := []string{"begin", "--extend", "--allow=second.txt", "--reason=利用者からの追加依頼", "--task", "日本語 作業", "--no-progress", "--trust-repository", dir}
			if format != "text" {
				args = append(args, format)
			}
			var stdout, stderr bytes.Buffer
			if code := cli.Run(args, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("範囲追加失敗: %d %s / %s", code, &stdout, &stderr)
			}
			if format == "text" {
				for _, want := range []string{"許容範囲を追加しました", "当初の許容パス", "利用者からの追加依頼", "追加時にすでに変更があったパス: 1件", "second.txt"} {
					if !strings.Contains(stdout.String(), want) {
						t.Fatalf("追加結果に %q がありません: %s", want, &stdout)
					}
				}
			} else {
				var data map[string]any
				decoder := json.NewDecoder(&stdout)
				if err := decoder.Decode(&data); err != nil {
					t.Fatal(err)
				}
				if err := decoder.Decode(new(any)); err != io.EOF {
					t.Fatalf("JSONが1件ではありません: %v", err)
				}
				if data["status"] != "extended" || data["baseline_id"] != started.BaselineID || data["command"] != "begin" {
					t.Fatalf("追加結果の識別情報が不正: %+v", data)
				}
				history := data["scope_extensions"].([]any)
				event := history[0].(map[string]any)
				if len(history) != 1 || event["reason"] != "利用者からの追加依頼" || event["added_at"] == "" {
					t.Fatalf("追加履歴が不正: %+v", data)
				}
				if format == "--json-summary" {
					if event["changed_path_count"] != float64(1) || event["allow_count"] != float64(1) || event["allow"] != nil || event["changed_paths"] != nil || data["initial_allow"] != nil {
						t.Fatalf("要約の履歴が不正: %+v", data)
					}
				} else if len(event["changed_paths"].([]any)) != 1 || len(data["initial_allow"].([]any)) != 1 {
					t.Fatalf("明細に当初の範囲か追加前の変更がありません: %+v", data)
				}
			}
			path := filepath.Join(dir, ".workscope", baseline.RecordName("日本語 作業"))
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			stdout.Reset()
			checkArgs := []string{"check", "--task", "日本語 作業", "--no-progress"}
			if code := cli.Run(checkArgs, &stdout, &stderr); code != 1 || !strings.Contains(stdout.String(), "許容範囲の追加履歴: 1件") || !strings.Contains(stdout.String(), "範囲外の変更: 1件") {
				t.Fatalf("履歴付きの照合結果が不正: %d %s / %s", code, &stdout, &stderr)
			}
			for _, flag := range []string{"--json", "--json-summary"} {
				stdout.Reset()
				if code := cli.Run(append(checkArgs, flag), &stdout, &stderr); code != 1 || !strings.Contains(stdout.String(), `"scope_extensions"`) || !strings.Contains(stdout.String(), "利用者からの追加依頼") {
					t.Fatalf("%s の照合で履歴がありません: %d %s / %s", flag, code, &stdout, &stderr)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("checkが記録を書き換えました: %v", err)
			}
		})
	}
}

func TestScopeExtensionCLIRejectsInvalidArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, args := range [][]string{
		{"begin", "--extend", "--allow", "file"},
		{"begin", "--extend", "--reason", "追加"},
		{"begin", "--extend", "--replace", "--allow", "file", "--reason", "追加"},
		{"begin", "--allow", "file", "--reason", "追加"},
		{"begin", "--extend", "--allow", "file", "--reason"},
		{"begin", "--extend", "--allow", "file", "--reason="},
		{"begin", "--extend", "--allow", "file", "--reason", "  "},
		{"begin", "--extend", "--allow", "file", "--reason", "理由\n追記"},
		{"begin", "--extend", "--allow", "file", "--reason", "追加", "--reason=再指定"},
		{"check", "--extend"},
		{"check", "--reason", "追加"},
	} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(append(args, "--json", "--no-progress"), &stdout, &stderr); code != 64 || !strings.Contains(stdout.String(), `"status":"invalid_arguments"`) || stderr.Len() != 0 {
			t.Fatalf("不正な指定を拒否していません: %v code=%d %s / %s", args, code, &stdout, &stderr)
		}
	}
}
