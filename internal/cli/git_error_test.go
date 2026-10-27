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
	"time"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

func TestMain(m *testing.M) {
	// テスト用Gitとして起動された場合だけ、指定した操作を失敗させる。
	if target := os.Getenv("WORKSCOPE_TEST_GIT_OPERATION"); target != "" {
		args := os.Args[1:]
		start := 0
	globalArguments:
		for start < len(args) {
			switch args[start] {
			case "--no-optional-locks":
				start++
			case "-C", "-c":
				start += 2
			default:
				break globalArguments
			}
		}
		if start <= len(args) && strings.Join(args[start:], " ") == target {
			if delay := os.Getenv("WORKSCOPE_TEST_GIT_DELAY"); delay != "" {
				duration, err := time.ParseDuration(delay)
				if err != nil {
					os.Exit(127)
				}
				time.Sleep(duration)
			} else {
				fmt.Fprintln(os.Stderr, "fatal: cannot open '/private/secret': Permission denied; https://user:password@example.invalid/?token=private\x1b[2J")
				os.Exit(1)
			}
		}
		command := exec.Command(os.Getenv("WORKSCOPE_TEST_REAL_GIT"), args...)
		if os.Getenv("WORKSCOPE_TEST_FORCE_OWNER") != "" {
			command.Env = append(os.Environ(), "GIT_TEST_ASSUME_DIFFERENT_OWNER=1", "GIT_CONFIG_NOSYSTEM=1")
		}
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

func installTestGit(t *testing.T) (string, string) {
	t.Helper()
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
	return gitPath, helperDir
}

func TestGitUnavailableOutputAndExitCodes(t *testing.T) {
	gitPath, helperDir := installTestGit(t)
	t.Chdir(t.TempDir())
	for _, failure := range []string{"permission", "not_found", "startup"} {
		t.Run(failure, func(t *testing.T) {
			if failure == "not_found" || failure == "startup" {
				emptyPath := t.TempDir()
				if failure == "startup" {
					name := "git"
					if runtime.GOOS == "windows" {
						name += ".exe"
					}
					if err := os.WriteFile(filepath.Join(emptyPath, name), []byte("実行ファイルではない"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				t.Setenv("PATH", emptyPath)
			} else {
				t.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
				t.Setenv("WORKSCOPE_TEST_GIT_OPERATION", "rev-parse --show-toplevel")
				t.Setenv("WORKSCOPE_TEST_REAL_GIT", gitPath)
			}
			for _, command := range []string{"begin", "check"} {
				for _, jsonOutput := range []bool{false, true} {
					for _, verbose := range []bool{false, true} {
						args := []string{command}
						if command == "begin" {
							args = append(args, "--allow", "new.txt")
						}
						if jsonOutput {
							args = append(args, "--json")
						}
						if verbose {
							args = append(args, "--verbose")
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
							wantCode := map[string]string{"permission": "git_permission_denied", "not_found": "git_not_found", "startup": "git_start_failed"}[failure]
							if decoder.Decode(new(any)) != io.EOF || !validProgress(stderr.String()) || result.Status != "unknown" || result.Command != command || result.Coverage.Complete || len(result.Issues) != 1 || result.Issues[0].Code != wantCode || (result.Issues[0].Details != "") != verbose {
								t.Fatalf("%v のJSON契約が不正: %+v / %s", args, result, &stderr)
							}
							text = result.Issues[0].Message + result.Issues[0].Details
						} else if command == "begin" {
							if stdout.Len() != 0 {
								t.Fatal("beginの失敗時に標準出力があります")
							}
							text = stderr.String()
						} else {
							if !validProgress(stderr.String()) {
								t.Fatal("checkの結果以外に診断を出力しています")
							}
							text = stdout.String()
						}
						wants := []string{"失敗したGit操作: git rev-parse --show-toplevel", "原因:", "次の対処:"}
						if verbose {
							wants = append(wants, "標準エラー（伏字済み）:", "パスSHA-256:", "親のGit環境変数:")
							if failure == "not_found" {
								wants = append(wants, "起動エラー: PATH上にGitの実行ファイルが見つかりません。", "Gitの終了コード: 未取得（起動失敗）", "標準エラー（伏字済み）: （空）")
							} else if failure == "startup" {
								wants = append(wants, "Gitの終了コード: 未取得（起動失敗）")
							} else {
								wants = append(wants, "起動エラー: なし", "Gitの終了コード: 1", "permission denied")
							}
						} else if strings.Contains(text, "パスSHA-256") || strings.Contains(text, "GIT_CONFIG_COUNT") || strings.Contains(text, "標準エラー（伏字済み）") {
							t.Fatal("通常表示に詳細が混ざっています")
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
		for _, args := range [][]string{{"begin", "--replace", "--allow", "new.txt", "--json", "--verbose"}, {"check", "--json", "--verbose"}} {
			stdout.Reset()
			stderr.Reset()
			code := cli.Run(args, &stdout, &stderr)
			var result baseline.Result
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 2 || result.Status != "unknown" || len(result.Issues) != 1 || result.Issues[0].Code != "git_permission_denied" || !validProgress(stderr.String()) {
				t.Fatalf("HEAD確認失敗の出力契約が不正: %d %s / %s / %v", code, &stdout, &stderr, err)
			}
			for _, want := range []string{"git rev-parse --verify HEAD", "git symbolic-ref -q HEAD", "permission denied", "Gitの終了コード: 1"} {
				if !strings.Contains(result.Issues[0].Details, want) {
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

func TestOwnershipDiagnosticExplainsDirectGitDifference(t *testing.T) {
	gitPath, helperDir := installTestGit(t)
	// 所有権拒否と復旧を、利用者の設定から隔離して確認する。
	normalHome := t.TempDir()
	t.Setenv("HOME", normalHome)
	t.Setenv("USERPROFILE", normalHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(normalHome, "xdg"))
	dir := t.TempDir()
	if output, err := exec.Command(gitPath, "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("一時リポジトリを作れません: %v %s", err, output)
	}
	if result, code := baseline.Begin(dir, []string{"new.txt"}, false, ""); code != 0 {
		t.Fatalf("開始記録失敗: %+v", result)
	}
	recordPath := filepath.Join(dir, ".workscope", "baseline.json")
	before, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "empty.gitconfig")
	if err := os.WriteFile(config, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "safe.directory")
	t.Setenv("GIT_CONFIG_VALUE_0", filepath.ToSlash(dir))
	direct := exec.Command(gitPath, "-C", dir, "rev-parse", "--show-toplevel")
	direct.Env = append(os.Environ(), "GIT_TEST_ASSUME_DIFFERENT_OWNER=1")
	if output, err := direct.CombinedOutput(); err != nil {
		t.Fatalf("環境変数で対象だけを信頼した直接実行に失敗: %v %s", err, output)
	}
	t.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKSCOPE_TEST_GIT_OPERATION", "所有権の再現")
	t.Setenv("WORKSCOPE_TEST_REAL_GIT", gitPath)
	t.Setenv("WORKSCOPE_TEST_FORCE_OWNER", "1")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANGUAGE", "C")
	t.Chdir(dir)
	localConfig, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"begin", "--replace", "--allow", "new.txt", "--json"}, {"check", "--json"}} {
		for _, verbose := range []bool{false, true} {
			callArgs := append([]string{}, args...)
			if verbose {
				callArgs = append(callArgs, "--verbose")
			}
			var stdout, stderr bytes.Buffer
			if code := cli.Run(callArgs, &stdout, &stderr); code != 2 || !validProgress(stderr.String()) {
				t.Fatalf("所有権の失敗契約が不正: %d %s / %s", code, &stdout, &stderr)
			}
			var result baseline.Result
			decoder := json.NewDecoder(&stdout)
			if err := decoder.Decode(&result); err != nil || decoder.Decode(new(any)) != io.EOF || result.Status != "unknown" || len(result.Issues) != 1 || result.Issues[0].Code != "git_ownership_rejected" {
				t.Fatalf("所有権失敗のJSONが不正: %+v %v", result, err)
			}
			message := result.Issues[0].Message
			for _, want := range []string{"失敗したGit操作:", "原因: Gitがリポジトリの所有権確認で拒否", "次の対処:", "--trust-repository <絶対パス>", "この実行だけ"} {
				if !strings.Contains(message, want) {
					t.Errorf("所有権の診断に %q がありません: %s", want, message)
				}
			}
			if strings.Contains(message, "パスSHA-256") || strings.Contains(message, "--global") || (result.Issues[0].Details != "") != verbose {
				t.Fatal("診断の要約と詳細が分離されていません")
			}
			if verbose {
				for _, want := range []string{"エラー分類: ownership", "Gitの実行ファイル（PATHで選択）:", "作業場所（git -C）:", "パスSHA-256:", "GIT_CONFIG_COUNT/KEY_*/VALUE_*/PARAMETERS: あり（引き継がない）", "GIT_CONFIG/GLOBAL/SYSTEM/NOSYSTEM: あり（引き継がない）"} {
					if !strings.Contains(result.Issues[0].Details, want) {
						t.Errorf("所有権の詳細に %q がありません", want)
					}
				}
			}
			message += result.Issues[0].Details
			for _, secret := range []string{dir, filepath.ToSlash(dir), config, helperDir, "safe.directory=*", "GIT_CONFIG_VALUE_0="} {
				if strings.Contains(message, secret) {
					t.Error("診断に伏せていないパスや危険な設定が含まれます")
				}
			}
			after, err := os.ReadFile(recordPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("所有権エラー時に開始記録が変更されました")
			}
		}
	}
	for _, args := range [][]string{{"check", "--json"}, {"begin", "--replace", "--allow", "new.txt", "--json"}} {
		var stdout, stderr bytes.Buffer
		args = append(args, "--trust-repository", dir)
		if code := cli.Run(args, &stdout, &stderr); code != 0 || !validProgress(stderr.String()) {
			t.Fatalf("一時的な信頼指定で復旧できません: %d %s / %s", code, &stdout, &stderr)
		}
		if args[0] == "check" {
			after, err := os.ReadFile(recordPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("一時的な信頼指定のcheckが開始記録を変更しました")
			}
		}
	}
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"check", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatal("信頼指定が別の実行へ残りました")
	}
	if _, err := os.Stat(filepath.Join(normalHome, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatal("恒久的なGit設定が作られました")
	}
	if after, err := os.ReadFile(filepath.Join(dir, ".git", "config")); err != nil || !bytes.Equal(localConfig, after) {
		t.Fatal("リポジトリの設定が変わりました")
	}
	if alternate, err := os.ReadFile(config); err != nil || len(alternate) != 0 {
		t.Fatal("上書き用の別設定ファイルへ登録してしまいました")
	}
	if os.Getenv("GIT_CONFIG_GLOBAL") != config || os.Getenv("GIT_CONFIG_VALUE_0") != filepath.ToSlash(dir) {
		t.Fatal("復旧手順が親の環境変数を変更しました")
	}
}

func TestSlowGitReportsProgressSeparatelyFromJSON(t *testing.T) {
	gitPath, helperDir := installTestGit(t)
	dir := t.TempDir()
	if output, err := exec.Command(gitPath, "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("一時リポジトリを作れません: %v %s", err, output)
	}
	t.Chdir(dir)
	t.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKSCOPE_TEST_REAL_GIT", gitPath)
	t.Setenv("WORKSCOPE_TEST_GIT_OPERATION", "rev-parse --show-toplevel")
	t.Setenv("WORKSCOPE_TEST_GIT_DELAY", "1300ms")
	for _, args := range [][]string{{"begin", "--allow", "new.txt", "--json"}, {"check", "--json"}} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != 0 {
			t.Fatalf("%v の実行失敗: %d %s / %s", args, code, &stdout, &stderr)
		}
		var result baseline.Result
		decoder := json.NewDecoder(&stdout)
		if err := decoder.Decode(&result); err != nil || decoder.Decode(new(any)) != io.EOF {
			t.Fatalf("進捗がJSONへ混入しました: %v", err)
		}
		if !validProgress(stderr.String()) {
			t.Fatalf("進捗の形式が不正: %s", &stderr)
		}
		for _, want := range []string{"Git待ち: git rev-parse --show-toplevel", "確認済みファイル:", "経過:", "処理終了"} {
			if !strings.Contains(stderr.String(), want) {
				t.Errorf("進捗に %q がありません: %s", want, &stderr)
			}
		}
	}
}
