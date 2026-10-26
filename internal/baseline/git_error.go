package baseline

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type gitError struct {
	cause     error
	operation string
	stderr    string
}

func (e *gitError) Unwrap() error { return e.cause }

func (e *gitError) Error() string {
	startup, exit := "なし", "未取得（起動失敗）"
	var processError *exec.ExitError
	if errors.As(e.cause, &processError) {
		exit = strconv.Itoa(processError.ExitCode())
		if processError.ExitCode() == -1 {
			exit = "未取得（プロセスが異常終了）"
		}
	} else {
		switch {
		case errors.Is(e.cause, exec.ErrNotFound):
			startup = "PATH上にGitの実行ファイルが見つかりません。"
		case errors.Is(e.cause, exec.ErrDot):
			startup = "現在のディレクトリにあるGitの実行は許可されていません。"
		default:
			// OSのエラー番号と説明には、実行ファイルのパスを含めない。
			var errno syscall.Errno
			if errors.As(e.cause, &errno) {
				startup = fmt.Sprintf("%s（OSエラー番号: %d）", errno.Error(), errno)
			} else if errors.Is(e.cause, os.ErrPermission) {
				startup = "Gitを起動する権限がありません。"
			} else {
				startup = "Gitを起動できません（詳細は伏字）。"
			}
		}
	}
	return fmt.Sprintf("失敗したGit操作: %s\n  起動エラー: %s\n  Gitの終了コード: %s\n  標準エラー（伏字済み）: %s", e.operation, startup, exit, e.stderr)
}

func failGit(code, message string, err error) error {
	return fail(code, message+"\n  "+err.Error(), "")
}

func gitOperation(args []string) string {
	// 作業場所・設定値・参照名など、利用者由来の引数を診断へ転記しない。
	safe := []string{"git"}
	for _, arg := range args {
		switch arg {
		case "rev-parse", "--show-toplevel", "--path-format=absolute", "--absolute-git-dir", "--git-common-dir",
			"--verify", "HEAD", "symbolic-ref", "-q", "show-ref", "--quiet", "ls-files", "--stage", "-z",
			"check-ignore", "--no-index", "--stdin", "status", "--porcelain=v1", "--untracked-files=no",
			"--ignore-submodules=all", "--no-renames":
			safe = append(safe, arg)
		default:
			safe = append(safe, "<伏字>")
		}
	}
	return strings.Join(safe, " ")
}

func safeGitStderr(raw string) string {
	if raw == "" {
		return "（空）"
	}
	// 任意の設定値・パス・URL・認証情報を含み得るため、既知の語句以外は残さない。
	known := []string{
		"permission denied", "access is denied", "operation not permitted", "access denied",
		"not a git repository", "detected dubious ownership", "unsafe repository",
		"cannot change to", "no such file or directory", "unable to read", "unable to open",
		"could not read", "unable to access", "cannot open", "cannot stat", "cannot run",
		"bad config line", "invalid gitfile format", "index file corrupt", "index file smaller than expected",
		"bad object", "ambiguous argument", "unknown revision", "not a valid object name",
		"unknown option", "unknown switch", "unknown subcommand",
		"アクセスが拒否されました", "許可がありません", "そのようなファイルやディレクトリはありません",
	}
	var phrases []string
	lower := strings.ToLower(raw)
	for _, phrase := range known {
		if strings.Contains(lower, phrase) {
			phrases = append(phrases, phrase)
		}
	}
	if len(phrases) == 0 {
		return "<伏字: 既知の診断語句なし>"
	}
	return strings.Join(phrases, "; ") + " <その他は伏字>"
}
