package baseline

import (
	"errors"
	"os"
	"os/exec"
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
