package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

type jsonSummary struct {
	SchemaVersion  int               `json:"schema_version"`
	Tool           string            `json:"tool"`
	Command        string            `json:"command"`
	Status         string            `json:"status"`
	BaselineID     string            `json:"baseline_id"`
	Task           string            `json:"task"`
	Root           string            `json:"root"`
	Coverage       baseline.Coverage `json:"coverage"`
	Counts         map[string]*int   `json:"counts"`
	ObservedCounts map[string]int    `json:"observed_counts"`
	Issues         []baseline.Issue  `json:"issues"`
}

func runSummary(t *testing.T, args []string, wantCode int) jsonSummary {
	t.Helper()
	var stdout, stderr bytes.Buffer
	args = append(append([]string{}, args...), "--json-summary")
	if code := cli.Run(args, &stdout, &stderr); code != wantCode || !validProgress(stderr.String()) {
		t.Fatalf("%v: 終了コード=%d、期待値=%d、出力=%s / %s", args, code, wantCode, &stdout, &stderr)
	}
	var result jsonSummary
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("JSONが1件ではありません: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"preexisting_changes", "changes_since_begin", "index_changes", "out_of_scope_changes"} {
		if _, exists := fields[name]; exists {
			t.Fatalf("要約に変更の明細 %s があります", name)
		}
	}
	for _, name := range []string{"preexisting_changes", "changes_since_begin", "index_changes", "out_of_scope_changes", "issues"} {
		if _, exists := result.Counts[name]; !exists {
			t.Fatalf("要約に件数 %s がありません", name)
		}
	}
	if result.SchemaVersion != 1 || result.Tool != "workscope" || result.Command != args[0] || result.Issues == nil || (result.Task == "") != (fields["task"] == nil) {
		t.Fatalf("要約の固定項目が不正: %+v", result)
	}
	return result
}

func summaryGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("一時リポジトリの準備失敗: %v %s", err, output)
	}
}

func summaryWrite(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func summaryCommit(t *testing.T, dir string) {
	t.Helper()
	summaryGit(t, dir, "-c", "user.name=workscope-test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "検証用")
}

func summaryRepo(t *testing.T) string {
	t.Helper()
	dir, hooks := t.TempDir(), t.TempDir()
	summaryGit(t, dir, "init", "-q", "--template="+hooks)
	summaryGit(t, dir, "config", "core.hooksPath", hooks)
	summaryGit(t, dir, "config", "core.autocrlf", "false")
	summaryWrite(t, dir, "input.txt", "開始時の本文")
	summaryGit(t, dir, "add", "input.txt")
	summaryCommit(t, dir)
	return dir
}

func requireSummaryCounts(t *testing.T, result jsonSummary, expected ...int) {
	t.Helper()
	for i, name := range []string{"preexisting_changes", "changes_since_begin", "index_changes", "out_of_scope_changes", "issues"} {
		if count := result.Counts[name]; count == nil || *count != expected[i] {
			t.Fatalf("%s の件数が不正: %+v、期待値=%d", name, result.Counts, expected[i])
		}
	}
	if result.ObservedCounts != nil {
		t.Fatal("完全な照合に下限の件数があります")
	}
}

func requireUnknownSummaryCounts(t *testing.T, result jsonSummary) {
	t.Helper()
	if result.Coverage.Complete || result.Status == "within_scope" || result.ObservedCounts == nil {
		t.Fatalf("不完全な照合の表示が不正: %+v", result)
	}
	for _, name := range []string{"changes_since_begin", "index_changes", "out_of_scope_changes"} {
		if result.Counts[name] != nil {
			t.Fatalf("未確認の総件数を確定しました: %s %+v", name, result.Counts)
		}
	}
	if count := result.Counts["issues"]; count == nil || *count != len(result.Issues) || *count == 0 {
		t.Fatal("問題の件数が不正です")
	}
}

func TestJSONSummaryCountsAndRecordProtection(t *testing.T) {
	for _, task := range []string{"", "日本語 作業"} {
		t.Run(task, func(t *testing.T) {
			dir := summaryRepo(t)
			t.Chdir(dir)
			summaryWrite(t, dir, "input.txt", "開始前の変更")
			begin := []string{"begin", "--allow", "input.txt"}
			check := []string{"check"}
			if task != "" {
				begin = append(begin, "--task", task)
				check = append(check, "--task="+task)
			}
			recorded := runSummary(t, begin, 0)
			if recorded.Status != "recorded" || recorded.BaselineID == "" || recorded.Task != task || !recorded.Coverage.Complete {
				t.Fatalf("記録成功の要約が不正: %+v", recorded)
			}
			requireSummaryCounts(t, recorded, 1, 0, 0, 0, 0)
			unchanged := runSummary(t, check, 0)
			if unchanged.Status != "within_scope" || unchanged.BaselineID != recorded.BaselineID {
				t.Fatalf("変更なしの照合が不正: %+v", unchanged)
			}
			requireSummaryCounts(t, unchanged, 1, 0, 0, 0, 0)
			summaryWrite(t, dir, "input.txt", "開始後の変更")
			summaryWrite(t, dir, "範囲外 空白.txt", "範囲外の変更")
			summaryGit(t, dir, "add", "範囲外 空白.txt")
			protected := map[string][]byte{}
			for _, name := range []string{"input.txt", "範囲外 空白.txt", ".git/index", ".git/HEAD", ".git/config", ".workscope/" + baseline.RecordName(task)} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				protected[name] = data
			}
			checked := runSummary(t, check, 1)
			if checked.Status != "out_of_scope" || checked.BaselineID != recorded.BaselineID || checked.Task != task || checked.Root != recorded.Root {
				t.Fatalf("範囲外の要約が不正: %+v", checked)
			}
			requireSummaryCounts(t, checked, 1, 2, 1, 2, 0)
			var stdout, stderr bytes.Buffer
			if code := cli.Run(append(check, "--json"), &stdout, &stderr); code != 1 {
				t.Fatalf("詳細JSONの照合失敗: %d %s", code, &stdout)
			}
			var detailed baseline.Result
			if err := json.Unmarshal(stdout.Bytes(), &detailed); err != nil {
				t.Fatal(err)
			}
			if len(detailed.ChangesSinceBegin) != 2 || len(detailed.IndexChanges) != 1 || len(detailed.OutOfScopeChanges) != 2 || !reflect.DeepEqual(checked.Coverage, detailed.Coverage) {
				t.Fatalf("詳細と要約の照合範囲・件数が不一致: %+v", detailed)
			}
			rejected := runSummary(t, begin, 2)
			requireUnknownSummaryCounts(t, rejected)
			if rejected.Issues[0].Code != "baseline_exists" || rejected.Counts["preexisting_changes"] != nil {
				t.Fatalf("既存記録の拒否が不正: %+v", rejected)
			}
			if code := cli.Run(append(check, "--json-summary"), failedWriter{}, &stderr); code != 2 || !strings.Contains(stderr.String(), "書き込みに失敗") {
				t.Fatalf("要約の出力失敗を成功扱いしました: %d %s", code, &stderr)
			}
			for name, before := range protected {
				after, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("対象・Git状態・開始記録が変化しました: %s %v", name, err)
				}
			}
		})
	}
}

func TestJSONSummaryHeadChangeKeepsObservedOutOfScopeCounts(t *testing.T) {
	dir := summaryRepo(t)
	t.Chdir(dir)
	runSummary(t, []string{"begin", "--allow", "input.txt"}, 0)
	summaryWrite(t, dir, "outside.txt", "開始後の変更")
	summaryGit(t, dir, "add", "outside.txt")
	summaryCommit(t, dir)
	result := runSummary(t, []string{"check"}, 2)
	requireUnknownSummaryCounts(t, result)
	if count := result.Counts["preexisting_changes"]; count == nil || *count != 0 {
		t.Fatal("読み取れた開始前の件数がありません")
	}
	if result.Issues[0].Code != "head_changed" || result.ObservedCounts["changes_since_begin"] != 1 || result.ObservedCounts["index_changes"] != 1 || result.ObservedCounts["out_of_scope_changes"] != 2 {
		t.Fatalf("HEAD変更時の検出済み件数が不正: %+v", result)
	}
}

func TestJSONSummaryArgumentErrorsNeverWriteState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, args := range [][]string{
		{"begin"}, {"begin", "--allow"}, {"begin", "--allow", "input.txt", "--unknown"},
		{"begin", "--allow", "input.txt", "--json"}, {"begin", "--task="},
		{"check", "--allow", "input.txt"}, {"check", "--replace"}, {"check", "--task="},
		{"check", "--json"}, {"check", "--task", "--json"},
	} {
		result := runSummary(t, args, 64)
		requireUnknownSummaryCounts(t, result)
		if result.Status != "invalid_arguments" || result.Counts["preexisting_changes"] != nil {
			t.Fatalf("引数エラーの要約が不正: %+v", result)
		}
	}
	for _, command := range []string{"begin", "check"} {
		args := []string{command, "--json-summary", "--json"}
		if command == "begin" {
			args = append(args, "--allow", "input.txt")
		}
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != 64 || !json.Valid(stdout.Bytes()) || !strings.Contains(stdout.String(), "併用できません") {
			t.Fatalf("表示指定の逆順の併用を拒否しませんでした: %d %s", code, &stdout)
		}
	}
	children, err := os.ReadDir(dir)
	if err != nil || len(children) != 0 {
		t.Fatalf("引数エラーで状態を作成しました: %v %v", children, err)
	}
}

func TestJSONSummaryMissingAndCorruptRecordsHaveUnknownCounts(t *testing.T) {
	dir := summaryRepo(t)
	t.Chdir(dir)
	for _, args := range [][]string{{"check"}, {"check", "--task=未記録"}} {
		result := runSummary(t, args, 2)
		requireUnknownSummaryCounts(t, result)
		if result.Counts["preexisting_changes"] != nil || result.BaselineID != "" || result.Issues[0].Code != "baseline_missing" {
			t.Fatalf("未記録の件数を確定しました: %+v", result)
		}
	}
	runSummary(t, []string{"begin", "--allow", "input.txt"}, 0)
	name := filepath.Join(dir, ".workscope", "baseline.json")
	if err := os.WriteFile(name, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	result := runSummary(t, []string{"check"}, 2)
	requireUnknownSummaryCounts(t, result)
	if result.Counts["preexisting_changes"] != nil || result.BaselineID != "" || result.Issues[0].Code != "foreign_state" {
		t.Fatalf("破損記録の要約が不正: %+v", result)
	}
	if data, err := os.ReadFile(name); err != nil || string(data) != "{" {
		t.Fatal("破損記録を書き換えました")
	}
}

func TestJSONSummaryGitDiagnosticsRespectVerbose(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	for _, command := range []string{"begin", "check"} {
		for _, verbose := range []bool{false, true} {
			args := []string{command}
			if command == "begin" {
				args = append(args, "--allow", "input.txt")
			}
			if verbose {
				args = append(args, "--verbose")
			}
			result := runSummary(t, args, 2)
			requireUnknownSummaryCounts(t, result)
			if result.Issues[0].Code != "git_not_found" || (result.Issues[0].Details != "") != verbose || result.Counts["preexisting_changes"] != nil {
				t.Fatalf("要約のGit診断が不正: %+v", result)
			}
		}
	}
}
