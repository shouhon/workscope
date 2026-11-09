package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

func TestNoProgressPreservesResultsDiagnosticsAndRecords(t *testing.T) {
	for _, mode := range []string{"text", "--json", "--json-summary"} {
		t.Run(mode, func(t *testing.T) {
			dir := summaryRepo(t)
			t.Chdir(dir)
			begin := []string{"begin", "--task", "表示確認", "--allow", "future", "--no-progress"}
			if mode != "text" {
				begin = append(begin, mode)
			}
			var stdout, stderr bytes.Buffer
			if code := cli.Run(begin, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("進捗抑制で記録に失敗しました: %d %s %s", code, &stdout, &stderr)
			}
			protected := map[string][]byte{}
			for _, name := range []string{"input.txt", ".git/index", ".git/HEAD", ".git/config", ".workscope/" + baseline.RecordName("表示確認")} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				protected[name] = data
			}
			for _, state := range []struct {
				name string
				code int
			}{{"within_scope", 0}, {"out_of_scope", 1}, {"unknown", 2}, {"invalid_arguments", 64}} {
				if state.code == 1 {
					summaryWrite(t, dir, "outside.txt", "範囲外")
				}
				args := []string{"check", "--task", "表示確認", "--verbose"}
				if state.code == 2 {
					args[2] = "記録なし"
				} else if state.code == 64 {
					args = append(args, "--unknown")
				}
				if mode != "text" {
					args = append(args, mode)
				}
				var normalOut, normalErr, quietOut, quietErr bytes.Buffer
				normalCode := cli.Run(args, &normalOut, &normalErr)
				quietCode := cli.Run(append(args, "--no-progress"), &quietOut, &quietErr)
				if normalCode != state.code || quietCode != state.code || normalOut.String() != quietOut.String() || strings.Contains(quietErr.String(), "進捗:") {
					t.Fatalf("%s %sで判定または出力が変化: %d %d %s / %s / %s", mode, state.name, normalCode, quietCode, &normalOut, &quietOut, &quietErr)
				}
				if state.code >= 2 && !strings.Contains(quietOut.String()+quietErr.String(), map[int]string{2: "開始記録", 64: "引数"}[state.code]) {
					t.Fatal("進捗とともに必要な診断を消しました")
				}
			}
			// beginの失敗診断も残し、既存の開始記録を取り直さない。
			stdout.Reset()
			stderr.Reset()
			if code := cli.Run(begin, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String()+stderr.String(), "開始記録が既にあります") || strings.Contains(stderr.String(), "進捗:") {
				t.Fatalf("記録拒否時の診断が不正: %d %s %s", code, &stdout, &stderr)
			}
			for name, before := range protected {
				after, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("表示指定が既存状態を変更しました: %s %v", name, err)
				}
			}
		})
	}
}
