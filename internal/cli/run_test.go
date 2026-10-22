package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
	"workscope/internal/version"
)

func TestHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := cli.Run([]string{arg}, &stdout, &stderr); code != 0 {
				t.Fatalf("終了コード = %d、期待値 = 0", code)
			}
			for _, text := range []string{"使い方", "workscope check [--json]", "サブモジュール", "/.workscope/", "1=範囲外の変更あり"} {
				if !strings.Contains(stdout.String(), text) {
					t.Errorf("ヘルプに %q がありません", text)
				}
			}
			if stderr.Len() != 0 {
				t.Errorf("標準エラーが空ではありません: %s", &stderr)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("終了コード = %d、期待値 = 0", code)
	}
	if want := "workscope " + version.Current + "\n"; stdout.String() != want {
		t.Errorf("版表示 = %q、期待値 = %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("標準エラーが空ではありません: %s", &stderr)
	}
}

func TestUnsupportedArgumentsNeverReportSuccess(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "引数なし"},
		{name: "未知の引数", args: []string{"--unknown"}},
		{name: "余分な引数", args: []string{"--version", "extra"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := cli.Run(tc.args, &stdout, &stderr); code != 64 {
				t.Errorf("終了コード = %d、期待値 = 64", code)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "workscope --help") {
				t.Errorf("誤った引数の案内が不正です。標準出力 = %q、標準エラー = %q", &stdout, &stderr)
			}
		})
	}
}

func TestBeginInvalidArgumentsJSON(t *testing.T) {
	for _, args := range [][]string{{"begin", "--json"}, {"begin", "--allow", "--json"}, {"begin", "--json", "--unknown"}} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != 64 {
			t.Errorf("終了コード = %d", code)
		}
		if !strings.Contains(stdout.String(), `"status":"invalid_arguments"`) || stderr.Len() != 0 {
			t.Errorf("JSONの引数エラーが不正です: %s / %s", &stdout, &stderr)
		}
	}
}

func TestBeginJSONAndReadOnlyCommands(t *testing.T) {
	dir := t.TempDir()
	if output, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	if err := os.WriteFile(filepath.Join(dir, "日本語 空白.txt"), []byte("検証用の本文"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"begin", "--allow", "日本語 空白.txt", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
	var result baseline.Result
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("JSONが1件ではありません: %v", err)
	}
	if result.Status != "recorded" || result.Command != "begin" || result.Tool != "workscope" || result.SchemaVersion != 1 || result.BaselineID == "" || result.ChangesSinceBegin == nil || result.IndexChanges == nil || result.OutOfScopeChanges == nil || result.Issues == nil || stderr.Len() != 0 {
		t.Fatalf("出力契約が不正: %+v / %s", result, &stderr)
	}
	name := filepath.Join(dir, ".workscope", "baseline.json")
	before, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"--version"}, {"check", "--json"}, {"begin", "--allow", "日本語 空白.txt", "--json"}} {
		stdout.Reset()
		stderr.Reset()
		code := cli.Run(args, &stdout, &stderr)
		if args[0] == "check" && (code != 0 || !strings.Contains(stdout.String(), `"status":"within_scope"`)) {
			t.Fatalf("照合成功の契約が不正: %d %s", code, &stdout)
		}
		if args[0] == "begin" && (code != 2 || !strings.Contains(stdout.String(), `"status":"unknown"`)) {
			t.Fatalf("既存記録の失敗契約が不正: %d %s", code, &stdout)
		}
		after, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("%v が開始記録を更新しました", args)
		}
	}
}

func TestCheckJSONAndTextOutputContracts(t *testing.T) {
	dir := t.TempDir()
	if output, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	t.Chdir(dir)
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"begin", "--allow", "src", "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("開始記録失敗: %d %s %s", code, &stdout, &stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "範囲外 空白.txt"), []byte("本文"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, jsonOutput := range []bool{true, false} {
		stdout.Reset()
		stderr.Reset()
		args := []string{"check"}
		if jsonOutput {
			args = append(args, "--json")
		}
		if code := cli.Run(args, &stdout, &stderr); code != 1 || stderr.Len() != 0 {
			t.Fatalf("範囲外の出力失敗: %d %s / %s", code, &stdout, &stderr)
		}
		if jsonOutput {
			var result baseline.Result
			decoder := json.NewDecoder(&stdout)
			if err := decoder.Decode(&result); err != nil {
				t.Fatal(err)
			}
			if err := decoder.Decode(new(any)); err != io.EOF {
				t.Fatalf("JSONが1件ではありません: %v", err)
			}
			if result.Command != "check" || result.Status != "out_of_scope" || result.SchemaVersion != 1 || result.BaselineID == "" || len(result.ChangesSinceBegin) != 1 || len(result.OutOfScopeChanges) != 1 || result.PreexistingChanges == nil || result.IndexChanges == nil || result.Issues == nil {
				t.Fatalf("照合のJSON契約が不正: %+v", result)
			}
		} else {
			for _, text := range []string{"開始前からあった変更: 0件", "開始後の変更: 1件", "インデックスの変化: 0件", "範囲外の変更: 1件", "判定不能の理由: 0件", "範囲外 空白.txt", "対象外:"} {
				if !strings.Contains(stdout.String(), text) {
					t.Errorf("日本語表示に %q がありません: %s", text, &stdout)
				}
			}
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run([]string{"check", "--json"}, failedWriter{}, &stderr); code != 2 {
		t.Fatalf("照合のJSON出力失敗を成功扱いしました: %d", code)
	}
}

func TestCheckJSONUnknownAndInvalidArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, tc := range []struct {
		args   []string
		code   int
		status string
	}{
		{[]string{"check", "--json"}, 2, "unknown"},
		{[]string{"check", "--allow", "src", "--json"}, 64, "invalid_arguments"},
		{[]string{"check", "--json", "--replace"}, 64, "invalid_arguments"},
	} {
		var stdout, stderr bytes.Buffer
		code := cli.Run(tc.args, &stdout, &stderr)
		var result baseline.Result
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != tc.code || result.Status != tc.status || result.Command != "check" || result.Coverage.Complete || len(result.Issues) == 0 || stderr.Len() != 0 {
			t.Fatalf("失敗時のJSON契約が不正: %v %d %s / %s / %v", tc.args, code, &stdout, &stderr, err)
		}
	}
}

func TestBeginOutsideGitReturnsUnknownJSON(t *testing.T) {
	t.Chdir(t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"begin", "--allow", "new.txt", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("終了コード = %d", code)
	}
	var result baseline.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Status != "unknown" || stderr.Len() != 0 {
		t.Fatalf("判定不能のJSONが不正: %s / %s / %v", &stdout, &stderr, err)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) {
	return 0, errors.New("書き込み不能")
}

func TestOutputFailureNeverReportsSuccess(t *testing.T) {
	var stderr bytes.Buffer
	if code := cli.Run([]string{"--version"}, failedWriter{}, &stderr); code != 2 {
		t.Errorf("終了コード = %d、期待値 = 2", code)
	}
	if !strings.Contains(stderr.String(), "書き込みに失敗") {
		t.Errorf("出力失敗の案内がありません: %s", &stderr)
	}
}
