package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
			for _, text := range []string{"使い方", "workscope check [--json]", "--task <名前>", "サブモジュール", "/.workscope/", "1=範囲外の変更あり"} {
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

func TestSubcommandHelpDoesNotNeedGitOrWriteState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", t.TempDir())
	for _, command := range []string{"begin", "check"} {
		for _, flags := range [][]string{{"--help"}, {"-h"}, {"--json", "--help"}} {
			args := append([]string{command}, flags...)
			var stdout, stderr bytes.Buffer
			if code := cli.Run(args, &stdout, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("%v のヘルプ表示失敗: %d %s", args, code, &stderr)
			}
			for _, want := range []string{"使い方:", "引数:", "使用例:", "--task <名前>", "終了コード:"} {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("%v のヘルプに %q がありません", args, want)
				}
			}
			if command == "begin" && !strings.Contains(stdout.String(), "必須。Gitルートからの相対パス") {
				t.Fatal("必須引数の説明がありません")
			}
		}
	}
	children, err := os.ReadDir(dir)
	if err != nil || len(children) != 0 {
		t.Fatalf("ヘルプ表示で状態を作成しました: %v %v", children, err)
	}
}

func validProgress(text string) bool {
	if text == "" {
		return true
	}
	line := regexp.MustCompile(`^進捗: (Git待ち: git [A-Za-z0-9 =<>_./伏字-]+|処理の準備|Git作業場所の確認|許容範囲の確認|開始記録の確認|ファイル走査|許容範囲の再確認|開始記録の保存|開始後の変更の照合|開始記録の再確認|処理終了) \| 確認済みファイル: [0-9]+件 \| 取得: [0-2]/2回目 \| 経過: [0-9]+\.[0-9]秒$`)
	for _, value := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if !line.MatchString(value) {
			return false
		}
	}
	return true
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
	if result.Status != "recorded" || result.Command != "begin" || result.Tool != "workscope" || result.SchemaVersion != 1 || result.BaselineID == "" || result.ChangesSinceBegin == nil || result.IndexChanges == nil || result.OutOfScopeChanges == nil || result.Issues == nil || !validProgress(stderr.String()) {
		t.Fatalf("出力契約が不正: %+v / %s", result, &stderr)
	}
	name := filepath.Join(dir, ".workscope", "baseline.json")
	before, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"--version"}, {"begin", "--help"}, {"check", "-h"}, {"check", "--json"}, {"begin", "--allow", "日本語 空白.txt", "--json"}} {
		stdout.Reset()
		stderr.Reset()
		code := cli.Run(args, &stdout, &stderr)
		if args[0] == "check" && args[1] == "--json" && (code != 0 || !strings.Contains(stdout.String(), `"status":"within_scope"`)) {
			t.Fatalf("照合成功の契約が不正: %d %s", code, &stdout)
		}
		if args[0] == "begin" && args[1] != "--help" && (code != 2 || !strings.Contains(stdout.String(), `"status":"unknown"`)) {
			t.Fatalf("既存記録の失敗契約が不正: %d %s", code, &stdout)
		}
		after, err := os.ReadFile(name)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("%v が開始記録を更新しました", args)
		}
	}
}

func TestEmptyStateDiagnosticsPreserveDirectory(t *testing.T) {
	dir := t.TempDir()
	fixtureGit := func(args ...string) {
		t.Helper()
		output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("一時リポジトリの準備失敗: %v %s", err, output)
		}
	}
	fixtureGit("init", "-q", "--template="+t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "input.txt"), []byte("検証用の本文"), 0600); err != nil {
		t.Fatal(err)
	}
	fixtureGit("add", "input.txt")
	fixtureGit("-c", "user.name=workscope-test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath="+t.TempDir(), "commit", "-qm", "検証用")
	state := filepath.Join(dir, ".workscope")
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	protected := map[string][]byte{}
	for _, name := range []string{"input.txt", ".git/index", ".git/HEAD", ".git/config"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		protected[name] = data
	}
	t.Chdir(dir)
	for _, task := range []string{"", "repro"} {
		for _, command := range []struct {
			name string
			args []string
		}{
			{"begin", []string{"begin", "--allow", "input.txt"}},
			{"replace", []string{"begin", "--replace", "--allow", "input.txt"}},
			{"check", []string{"check"}},
		} {
			for _, output := range []struct {
				name string
				args []string
				json bool
			}{
				{"text", nil, false},
				{"json", []string{"--json"}, true},
				{"verbose", []string{"--verbose"}, false},
				{"json_verbose", []string{"--json", "--verbose"}, true},
			} {
				t.Run(command.name+"/"+task+"/"+output.name, func(t *testing.T) {
					args := append(append([]string{}, command.args...), output.args...)
					if task != "" {
						args = append(args, "--task", task)
					}
					var stdout, stderr bytes.Buffer
					if code := cli.Run(args, &stdout, &stderr); code != 2 {
						t.Fatalf("空の保存先の拒否が不正: %d %s / %s", code, &stdout, &stderr)
					}
					message := stdout.String()
					if !output.json && command.args[0] == "begin" {
						if stdout.Len() != 0 {
							t.Fatalf("拒否したbeginの標準出力が空ではありません: %s", &stdout)
						}
						message = stderr.String()
					} else if !validProgress(stderr.String()) {
						t.Fatalf("標準エラーが不正です: %s", &stderr)
					}
					if output.json {
						var result baseline.Result
						decoder := json.NewDecoder(&stdout)
						if err := decoder.Decode(&result); err != nil {
							t.Fatal(err)
						}
						if err := decoder.Decode(new(any)); err != io.EOF {
							t.Fatalf("JSONが1件ではありません: %v", err)
						}
						if result.Status != "unknown" || result.Command != command.args[0] || result.Task != task || result.Coverage.Complete || len(result.Issues) != 1 || result.Issues[0].Code != "foreign_state" || result.Issues[0].Path != ".workscope" {
							t.Fatalf("空の保存先のJSON契約が不正: %+v", result)
						}
						message = result.Issues[0].Message
					}
					for _, want := range []string{"既存の .workscope は空", "保存先は変更していません", "通常の空ディレクトリ", "別名へ退避", "次の編集前に begin", "空の状態保存先から復旧する"} {
						if !strings.Contains(message, want) {
							t.Fatalf("診断に %q がありません: %s", want, message)
						}
					}
					children, err := os.ReadDir(state)
					if err != nil || len(children) != 0 {
						t.Fatalf("拒否した保存先が変化しました: %v %v", children, err)
					}
					for name, before := range protected {
						after, err := os.ReadFile(filepath.Join(dir, name))
						if err != nil || !bytes.Equal(before, after) {
							t.Fatalf("対象ファイルかGit状態が変化しました: %s %v", name, err)
						}
					}
				})
			}
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
		if code := cli.Run(args, &stdout, &stderr); code != 1 || !validProgress(stderr.String()) {
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
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != tc.code || result.Status != tc.status || result.Command != "check" || result.Coverage.Complete || len(result.Issues) == 0 || !validProgress(stderr.String()) {
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
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Status != "unknown" || !validProgress(stderr.String()) {
		t.Fatalf("判定不能のJSONが不正: %s / %s / %v", &stdout, &stderr, err)
	}
}

func TestTaskArgumentsJSONAndText(t *testing.T) {
	dir := t.TempDir()
	if output, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	t.Chdir(dir)
	const task = "日本語 作業"
	var id string
	for _, args := range [][]string{
		{"begin", "--task", task, "--allow=future", "--json"},
		{"check", "--task=" + task, "--json"},
	} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != 0 || !validProgress(stderr.String()) {
			t.Fatalf("タスクの実行失敗: %d %s / %s", code, &stdout, &stderr)
		}
		var result baseline.Result
		decoder := json.NewDecoder(&stdout)
		if err := decoder.Decode(&result); err != nil {
			t.Fatal(err)
		}
		if err := decoder.Decode(new(any)); err != io.EOF || result.Task != task || result.BaselineID == "" {
			t.Fatalf("タスクのJSON契約が不正: %+v %v", result, err)
		}
		if args[0] == "begin" {
			id = result.BaselineID
		} else if result.BaselineID != id {
			t.Fatal("別のタスクを照合しました")
		}
	}
	for _, args := range [][]string{
		{"begin", "--task=表示用", "--allow", "future"},
		{"check", "--task", "表示用"},
	} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != 0 || !validProgress(stderr.String()) || !strings.Contains(stdout.String(), `タスク: "表示用"`) {
			t.Fatalf("タスクの日本語表示が不正: %d %s / %s", code, &stdout, &stderr)
		}
		if args[0] == "begin" && !strings.Contains(stdout.String(), "/.workscope/"+baseline.RecordName("表示用")) {
			t.Fatal("実際の記録先を表示しませんでした")
		}
	}
}

func TestInvalidTaskArgumentsNeverWriteState(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, command := range []string{"begin", "check"} {
		for _, args := range [][]string{
			{"--task"}, {"--task", "--json"}, {"--task="}, {"--task", ""},
			{"--task", "A", "--task=B"}, {"--task=A", "--task", "B"},
			{"--task", "../outside"}, {"--task", " 前後"},
		} {
			full := append([]string{command}, args...)
			if command == "begin" {
				full = append(full, "--allow", "future")
			}
			full = append(full, "--json")
			var stdout, stderr bytes.Buffer
			code := cli.Run(full, &stdout, &stderr)
			var result baseline.Result
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 64 || result.Command != command || result.Status != "invalid_arguments" || result.Coverage.Complete || len(result.Issues) != 1 || stderr.Len() != 0 {
				t.Fatalf("タスクの引数エラーが不正: %v %d %s / %s / %v", full, code, &stdout, &stderr, err)
			}
		}
	}
	children, err := os.ReadDir(dir)
	if err != nil || len(children) != 0 {
		t.Fatalf("不正なタスク名でファイルを作成しました: %v %v", children, err)
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
