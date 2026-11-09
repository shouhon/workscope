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

func TestBeginEmptyStateOutputContracts(t *testing.T) {
	for _, task := range []string{"", "日本語 作業"} {
		for _, format := range []string{"text", "--json", "--json-summary"} {
			t.Run(task+"/"+format, func(t *testing.T) {
				dir := summaryRepo(t)
				if err := os.Mkdir(filepath.Join(dir, ".workscope"), 0700); err != nil {
					t.Fatal(err)
				}
				t.Chdir(dir)
				args := []string{"begin", "--allow", "input.txt", "--no-progress"}
				if task != "" {
					args = append(args, "--task", task)
				}
				if format != "text" {
					args = append(args, format)
				}
				var stdout, stderr bytes.Buffer
				if code := cli.Run(args, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
					t.Fatalf("空の保存先で開始できません: %d %s / %s", code, &stdout, &stderr)
				}
				if format == "text" {
					if !strings.Contains(stdout.String(), "開始記録を保存しました") {
						t.Fatalf("記録成功の表示がありません: %s", &stdout)
					}
				} else {
					var result baseline.Result
					decoder := json.NewDecoder(&stdout)
					if err := decoder.Decode(&result); err != nil {
						t.Fatal(err)
					}
					if err := decoder.Decode(new(any)); err != io.EOF {
						t.Fatalf("JSONが1件ではありません: %v", err)
					}
					if result.Status != "recorded" || result.Task != task || result.BaselineID == "" || !result.Coverage.Complete || len(result.Issues) != 0 {
						t.Fatalf("空の保存先の記録成功契約が不正: %+v", result)
					}
				}
				checkArgs := []string{"check", "--no-progress"}
				if task != "" {
					checkArgs = append(checkArgs, "--task", task)
				}
				checked := runSummary(t, checkArgs, 0)
				if checked.Status != "within_scope" {
					t.Fatalf("保存した記録を照合できません: %+v", checked)
				}
				requireSummaryCounts(t, checked, 0, 0, 0, 0, 0)
			})
		}
	}
}
