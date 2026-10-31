package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

func TestTrustRepositoryRejectsInvalidArgumentsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	if output, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	child := filepath.Join(dir, "配下")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "通常ファイル")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for _, command := range []string{"begin", "check"} {
		for _, flags := range [][]string{
			{"--trust-repository"}, {"--trust-repository", "--json"},
			{"--trust-repository="}, {"--trust-repository", ""},
			{"--trust-repository", "relative"}, {"--trust-repository", "*"},
			{"--trust-repository", filepath.ToSlash(dir) + "/*"},
			{"--trust-repository", dir + "\n"},
			{"--trust-repository", filepath.Join(dir, "missing")},
			{"--trust-repository", file}, {"--trust-repository", child},
			{"--trust-repository", t.TempDir()},
			{"--trust-repository", dir, "--trust-repository=" + dir},
		} {
			args := append([]string{command, "--json"}, flags...)
			if command == "begin" {
				args = append(args, "--allow", "future")
			}
			var stdout, stderr bytes.Buffer
			code := cli.Run(args, &stdout, &stderr)
			var result baseline.Result
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 64 || result.Status != "invalid_arguments" || result.Coverage.Complete || len(result.Issues) != 1 || stderr.Len() != 0 {
				t.Fatalf("%v の拒否が不正: %d %s / %s / %v", args, code, &stdout, &stderr, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".workscope")); !os.IsNotExist(err) {
		t.Fatal("不正な信頼指定で開始記録を作成しました")
	}
}

func TestTrustRepositoryDiagnosticsAndOutputModesPreserveState(t *testing.T) {
	dir := t.TempDir()
	if output, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, output)
	}
	file := filepath.Join(dir, "通常ファイル")
	if err := os.WriteFile(file, []byte("変更しない本文"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, code := baseline.Begin(dir, []string{"future"}, false, ""); code != 0 {
		t.Fatalf("準備失敗: %d %+v", code, result)
	}
	protected := map[string][]byte{}
	for _, name := range []string{file, filepath.Join(dir, ".git", "config"), filepath.Join(dir, ".git", "HEAD"), filepath.Join(dir, ".workscope", "baseline.json")} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		protected[name] = data
	}
	t.Chdir(dir)
	for _, tc := range []struct{ name, trust, code, message, stage string }{
		{"形式不正", "relative", "invalid_trust_repository", "形式が不正", "引数の形式確認"},
		{"不存在", filepath.Join(dir, "非公開の不存在パス"), "trust_repository_not_found", "参照先が存在しない", "指定先の実体解決"},
		{"通常ファイル", file, "trust_repository_not_directory", "ディレクトリではありません", "指定先の種類確認"},
		{"別ルート", t.TempDir(), "trust_repository_mismatch", "実体が一致しません", "Gitルートとの実体照合"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, command := range []string{"begin", "check"} {
				for _, mode := range []string{"", "--json", "--json-summary"} {
					for _, verbose := range []bool{false, true} {
						args := []string{command, "--trust-repository=" + tc.trust}
						if command == "begin" {
							args = append(args, "--allow", "future", "--replace")
						}
						if mode != "" {
							args = append(args, mode)
						}
						if verbose {
							args = append(args, "--verbose")
						}
						var stdout, stderr bytes.Buffer
						if code := cli.Run(args, &stdout, &stderr); code != 64 {
							t.Fatalf("拒否の終了コードが不正: %d %s / %s", code, &stdout, &stderr)
						}
						if mode != "" {
							var result baseline.Result
							if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Status != "invalid_arguments" || result.Coverage.Complete || len(result.Issues) != 1 || result.Issues[0].Code != tc.code || !validProgress(stderr.String()) {
								t.Fatalf("JSONの診断が不正: %s / %s / %v", &stdout, &stderr, err)
							}
							if (result.Issues[0].Details != "") != verbose {
								t.Fatal("詳細の表示条件が不正です")
							}
						}
						output := stdout.String() + stderr.String()
						if !strings.Contains(output, tc.message) || strings.Contains(output, "失敗した確認段階") != verbose || (verbose && !strings.Contains(output, tc.stage)) {
							t.Fatalf("要約または詳細が不正です: %s", output)
						}
						if strings.Contains(output, dir) || strings.Contains(output, filepath.ToSlash(dir)) || strings.Contains(output, "非公開の不存在パス") {
							t.Fatal("診断に生のパスが漏れました")
						}
					}
				}
			}
		})
	}
	for name, before := range protected {
		if after, err := os.ReadFile(name); err != nil || !bytes.Equal(before, after) {
			t.Fatal("拒否した実行が既存状態を変更しました")
		}
	}
	children, err := os.ReadDir(filepath.Join(dir, ".workscope"))
	if err != nil || len(children) != 1 || children[0].Name() != "baseline.json" {
		t.Fatal("拒否した実行が状態保存先を変更しました")
	}
}

func TestTrustRepositorySupportsWorktreeAndLimitsExistingWildcard(t *testing.T) {
	gitPath, helperDir := installTestGit(t)
	normalHome := t.TempDir()
	t.Setenv("HOME", normalHome)
	t.Setenv("USERPROFILE", normalHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(normalHome, "xdg"))
	globalPath := filepath.Join(normalHome, ".gitconfig")
	global := []byte("[safe]\n\tdirectory = *\n")
	if err := os.WriteFile(globalPath, global, 0600); err != nil {
		t.Fatal(err)
	}
	primary := t.TempDir()
	fixtureGit := func(args ...string) string {
		t.Helper()
		output, err := exec.Command(gitPath, append([]string{"-C", primary}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("一時リポジトリの準備失敗: %v %s", err, output)
		}
		return string(bytes.TrimSpace(output))
	}
	fixtureGit("init", "-q", "--template="+t.TempDir())
	fixtureGit("-c", "user.name=workscope-test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "開始", "--allow-empty")
	worktree := filepath.Join(t.TempDir(), "日本語 作業ツリー")
	fixtureGit("worktree", "add", "--detach", "-q", worktree)
	child := filepath.Join(worktree, "配下")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	localPath := filepath.Join(primary, ".git", "config")
	local, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", helperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WORKSCOPE_TEST_REAL_GIT", gitPath)
	t.Setenv("WORKSCOPE_TEST_GIT_OPERATION", "所有権の再現")
	t.Setenv("WORKSCOPE_TEST_FORCE_OWNER", "1")
	t.Chdir(child)
	for _, args := range [][]string{
		{"begin", "--allow", "future", "--trust-repository", worktree, "--json"},
		{"check", "--trust-repository=" + worktree, "--json"},
	} {
		var stdout, stderr bytes.Buffer
		if code := cli.Run(args, &stdout, &stderr); code != 0 || !validProgress(stderr.String()) {
			t.Fatalf("worktreeの信頼指定失敗: %d %s / %s", code, &stdout, &stderr)
		}
	}
	recordPath := filepath.Join(worktree, ".workscope", "baseline.json")
	record, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	// 共通の管理領域を持っていても、別の作業ツリーへ信頼を広げない。
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{"check", "--trust-repository", primary, "--json"}, &stdout, &stderr)
	var result baseline.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 2 || len(result.Issues) != 1 || result.Issues[0].Code != "git_ownership_rejected" {
		t.Fatalf("既存の全件信頼をこの実行に持ち込みました: %d %s / %v", code, &stdout, err)
	}
	for name, before := range map[string][]byte{globalPath: global, localPath: local, recordPath: record} {
		if after, err := os.ReadFile(name); err != nil || !bytes.Equal(before, after) {
			t.Fatalf("信頼指定が設定または開始記録を変更しました: %s", name)
		}
	}
}
