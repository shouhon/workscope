package baseline

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestGitDiagnosticRedactsUntrustedStderr(t *testing.T) {
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"空", "", "（空）"},
		{"権限", "fatal: cannot open '/private/secret': Permission denied\r\n", "permission denied; cannot open <その他は伏字>"},
		{"所有者", "fatal: detected dubious ownership in repository at 'C:/Users/private'", "detected dubious ownership <その他は伏字>"},
		{"Git外", "fatal: not a git repository (or any of the parent directories): .git", "not a git repository <その他は伏字>"},
		{"未知", "secret=value https://user:password@example.invalid/?token=private\x1b[2J\x00\xff", "<伏字: 既知の診断語句なし>"},
		{"診断語句と秘密値", "Permission denied\nAuthorization: Bearer private\nC:\\Users\\private\\secret\nhttps://user:password@example.invalid", "permission denied <その他は伏字>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeGitStderr(tc.raw); got != tc.want {
				t.Fatalf("標準エラーの伏字が不正: %q", got)
			}
		})
	}
}

func TestGitStartupDiagnosticPreservesOSCauseWithoutPath(t *testing.T) {
	cause := &os.PathError{Op: "fork/exec", Path: "C:/private/secret/git.exe", Err: syscall.EACCES}
	err := &gitError{cause: cause, operation: "git rev-parse --show-toplevel", stderr: safeGitStderr("")}
	text := err.Error()
	for _, want := range []string{"起動エラー:", "OSエラー番号:", "Gitの終了コード: 未取得（起動失敗）", "標準エラー（伏字済み）: （空）"} {
		if !strings.Contains(text, want) {
			t.Errorf("起動エラーに %q がありません: %s", want, text)
		}
	}
	if strings.Contains(text, "private") || !errors.Is(err, syscall.EACCES) {
		t.Fatal("パスを伏せた原因の保持が不正です")
	}
	var original *os.PathError
	if !errors.As(err, &original) || original != cause {
		t.Fatal("元の起動エラーを参照できません")
	}
}

func TestGitFailureReportsOperationAndExitStatus(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	t.Setenv("LANGUAGE", "C")
	_, err := git(t.TempDir(), nil, "rev-parse", "--show-toplevel")
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() == 0 {
		t.Fatalf("Gitの終了状態を保持していません: %v", err)
	}
	text := describeError(failGit("git_unavailable", "Gitルートを特定できません。", err)).Message
	for _, want := range []string{"失敗したGit操作: git rev-parse --show-toplevel", "起動エラー: なし", "Gitの終了コード:", "not a git repository"} {
		if !strings.Contains(text, want) {
			t.Errorf("診断に %q がありません: %s", want, text)
		}
	}
}

func TestGitOperationDoesNotExposeReferenceNames(t *testing.T) {
	if got := gitOperation([]string{"show-ref", "--verify", "--quiet", "refs/heads/private"}); got != "git show-ref --verify --quiet <伏字>" {
		t.Fatalf("参照名が伏字になっていません: %s", got)
	}
}

func TestGitContextRedactsPathsAndUnknownEnvironmentNames(t *testing.T) {
	c := gitContext{executable: filepath.Join(t.TempDir(), "private-token", "git.exe"), directory: filepath.Join(t.TempDir(), "private-project")}
	for _, key := range []string{"git_config_value_0", "GIT_CONFIG_GLOBAL", "GIT_DIR", "GIT_PRIVATE_CREDENTIAL_NAME"} {
		c.omit(key)
	}
	text := c.String()
	for _, want := range []string{diagnosticPath(c.executable), diagnosticPath(c.directory), "その他のGIT_*: 1件を除外", "読込結果は未確認"} {
		if !strings.Contains(text, want) {
			t.Errorf("実行情報に %q がありません", want)
		}
	}
	for _, secret := range []string{"private-token", "private-project", "GIT_PRIVATE_CREDENTIAL_NAME"} {
		if strings.Contains(text, secret) {
			t.Error("実行情報に秘密が含まれています")
		}
	}
	if !c.commandConfig || !c.configLocation || !c.repository || diagnosticPath("") != "未特定" {
		t.Fatal("除外した設定群またはGit未特定の扱いが不正です")
	}
}

func TestGitErrorClassificationDistinguishesOwnershipAndPermissions(t *testing.T) {
	for _, tc := range []struct {
		err      error
		stderr   string
		category string
	}{
		{errors.New("Git失敗"), "fatal: detected dubious ownership; Permission denied", "ownership"},
		{os.ErrPermission, "", "permission"},
		{errors.New("Git失敗"), "fatal: Access is denied", "permission"},
		{exec.ErrNotFound, "", "not_found"},
		{errors.New("Git失敗"), "fatal: not a git repository", "not_repository"},
		{errors.New("Git失敗"), "fatal: bad config line 1", "configuration"},
		{exec.ErrDot, "", "startup"},
	} {
		if got := classifyGitError(tc.err, tc.stderr); !strings.HasPrefix(got, tc.category+"（") {
			t.Errorf("分類=%s、期待=%s", got, tc.category)
		}
	}
}
