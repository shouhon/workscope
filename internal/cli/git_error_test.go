package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

func TestMain(m *testing.M) {
	// テスト用Gitとして起動された場合だけ、指定した操作を失敗させる。
	if target := os.Getenv("WORKSCOPE_TEST_GIT_OPERATION"); target != "" {
		args := os.Args[1:]
		if len(args) >= 7 && strings.Join(args[7:], " ") == target {
			fmt.Fprintln(os.Stderr, "fatal: cannot open '/private/secret': Permission denied; https://user:password@example.invalid/?token=private\x1b[2J")
			os.Exit(1)
		}
		command := exec.Command(os.Getenv("WORKSCOPE_TEST_REAL_GIT"), args...)
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := command.Run(); err != nil {
			if exited, ok := err.(*exec.ExitError); ok {
				os.Exit(exited.ExitCode())
			}
			os.Exit(127)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGitUnavailableOutputAndExitCodes(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	helperDir := t.TempDir()
	helperName := "git"
	if runtime.GOOS == "windows" {
		helperName += ".exe"
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperDir, helperName), helper, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	for _, startupFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("起動失敗=%t", startupFailure), func(t *testing.T) {
			if startupFailure {
				t.Setenv("PATH", t.TempDir())
			} else {
				t.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
				t.Setenv("WORKSCOPE_TEST_GIT_OPERATION", "rev-parse --show-toplevel")
				t.Setenv("WORKSCOPE_TEST_REAL_GIT", gitPath)
			}
			for _, command := range []string{"begin", "check"} {
				for _, jsonOutput := range []bool{false, true} {
					args := []string{command}
					if command == "begin" {
						args = append(args, "--allow", "new.txt")
					}
					if jsonOutput {
						args = append(args, "--json")
					}
					var stdout, stderr bytes.Buffer
					if code := cli.Run(args, &stdout, &stderr); code != 2 {
						t.Fatalf("%v の判定不能が終了コード2ではありません: %d", args, code)
					}
					var text string
					if jsonOutput {
						var result baseline.Result
						decoder := json.NewDecoder(&stdout)
						if err := decoder.Decode(&result); err != nil {
							t.Fatal(err)
						}
						if decoder.Decode(new(any)) != io.EOF || stderr.Len() != 0 || result.Status != "unknown" || result.Command != command || result.Coverage.Complete || len(result.Issues) != 1 || result.Issues[0].Code != "git_unavailable" {
							t.Fatalf("%v のJSON契約が不正: %+v / %s", args, result, &stderr)
						}
						text = result.Issues[0].Message
					} else if command == "begin" {
						if stdout.Len() != 0 {
							t.Fatal("beginの失敗時に標準出力があります")
						}
						text = stderr.String()
					} else {
						if stderr.Len() != 0 {
							t.Fatal("checkの結果以外に診断を出力しています")
						}
						text = stdout.String()
					}
					wants := []string{"失敗したGit操作: git rev-parse --show-toplevel", "標準エラー（伏字済み）:"}
					if startupFailure {
						wants = append(wants, "起動エラー: PATH上にGitの実行ファイルが見つかりません。", "Gitの終了コード: 未取得（起動失敗）", "標準エラー（伏字済み）: （空）")
					} else {
						wants = append(wants, "起動エラー: なし", "Gitの終了コード: 1", "permission denied")
					}
					for _, want := range wants {
						if !strings.Contains(text, want) {
							t.Errorf("%v の診断に %q がありません: %s", args, want, text)
						}
					}
					for _, secret := range []string{"private", "password", "token=", "\x1b"} {
						if strings.Contains(text, secret) {
							t.Errorf("%v の診断に伏せていない値があります", args)
						}
					}
				}
			}
			if _, err := os.Stat(".workscope"); !os.IsNotExist(err) {
				t.Fatal("Gitの失敗時に状態保存先が作られました")
			}
		})
	}
	t.Run("HEAD確認途中の失敗", func(t *testing.T) {
		if output, err := exec.Command(gitPath, "init", "-q").CombinedOutput(); err != nil {
			t.Fatalf("一時リポジトリを作れません: %v %s", err, output)
		}
		var stdout, stderr bytes.Buffer
		if code := cli.Run([]string{"begin", "--allow", "new.txt", "--json"}, &stdout, &stderr); code != 0 {
			t.Fatalf("コミット前の開始記録失敗: %d %s / %s", code, &stdout, &stderr)
		}
		before, err := os.ReadFile(".workscope/baseline.json")
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		t.Setenv("WORKSCOPE_TEST_GIT_OPERATION", "symbolic-ref -q HEAD")
		t.Setenv("WORKSCOPE_TEST_REAL_GIT", gitPath)
		for _, args := range [][]string{{"begin", "--replace", "--allow", "new.txt", "--json"}, {"check", "--json"}} {
			stdout.Reset()
			stderr.Reset()
			code := cli.Run(args, &stdout, &stderr)
			var result baseline.Result
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 2 || result.Status != "unknown" || len(result.Issues) != 1 || result.Issues[0].Code != "head_unreadable" || stderr.Len() != 0 {
				t.Fatalf("HEAD確認失敗の出力契約が不正: %d %s / %s / %v", code, &stdout, &stderr, err)
			}
			for _, want := range []string{"git rev-parse --verify HEAD", "git symbolic-ref -q HEAD", "permission denied", "Gitの終了コード: 1"} {
				if !strings.Contains(result.Issues[0].Message, want) {
					t.Errorf("HEAD確認の診断に %q がありません", want)
				}
			}
			after, err := os.ReadFile(".workscope/baseline.json")
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("Gitの失敗時に既存の開始記録が変化しました")
			}
		}
	})
}
