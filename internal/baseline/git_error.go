package baseline

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type gitError struct {
	cause     error
	operation string
	stderr    string
	category  string
	context   gitContext
}

type gitContext struct {
	executable     string
	directory      string
	commandConfig  bool
	configLocation bool
	repository     bool
	other          int
}

func (c *gitContext) omit(key string) {
	key = strings.ToUpper(key)
	switch {
	case key == "GIT_CONFIG_COUNT", key == "GIT_CONFIG_PARAMETERS", strings.HasPrefix(key, "GIT_CONFIG_KEY_"), strings.HasPrefix(key, "GIT_CONFIG_VALUE_"):
		c.commandConfig = true
	case key == "GIT_CONFIG", key == "GIT_CONFIG_GLOBAL", key == "GIT_CONFIG_SYSTEM", key == "GIT_CONFIG_NOSYSTEM":
		c.configLocation = true
	case key == "GIT_DIR", key == "GIT_WORK_TREE", key == "GIT_COMMON_DIR", key == "GIT_INDEX_FILE", key == "GIT_OBJECT_DIRECTORY", key == "GIT_ALTERNATE_OBJECT_DIRECTORIES":
		c.repository = true
	default:
		c.other++
	}
}

func diagnosticPath(name string) string {
	if name == "" {
		return "未特定"
	}
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "絶対パスを確認できません（値は伏字）"
	}
	// パスにも秘密や個人名を含められるため、照合用の識別子だけを表示する。
	hash := sha256.Sum256([]byte(filepath.ToSlash(absolute)))
	return fmt.Sprintf("<絶対パスは伏字> パスSHA-256: %x", hash)
}

func (c gitContext) String() string {
	present := func(value bool) string {
		if value {
			return "あり（引き継がない）"
		}
		return "なし"
	}
	return fmt.Sprintf("Gitの実行ファイル（PATHで選択）: %s\n  作業場所（git -C）: %s\n  親のGit環境変数: すべて除外（名前と値の任意部分は非表示）\n  GIT_CONFIG_COUNT/KEY_*/VALUE_*/PARAMETERS: %s\n  GIT_CONFIG/GLOBAL/SYSTEM/NOSYSTEM: %s\n  GIT_DIR/WORK_TREE/COMMON_DIR/INDEX_FILEなど: %s\n  その他のGIT_*: %d件を除外\n  通常の設定探索: HOME・USERPROFILE・XDG_CONFIG_HOMEなどは継承（設定ファイルの読込結果は未確認）\n  コマンド内の固定設定: core.fsmonitor=false, core.untrackedCache=false, GIT_OPTIONAL_LOCKS=0, GIT_TERMINAL_PROMPT=0, LC_ALL=C, LANGUAGE=C", diagnosticPath(c.executable), diagnosticPath(c.directory), present(c.commandConfig), present(c.configLocation), present(c.repository), c.other)
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
	category := e.category
	if category == "" {
		category = classifyGitError(e.cause, e.stderr)
	}
	return fmt.Sprintf("失敗したGit操作: %s\n  エラー分類: %s\n  起動エラー: %s\n  Gitの終了コード: %s\n  標準エラー（伏字済み）: %s\n  %s\n  再実行の案内: %s", e.operation, category, startup, exit, e.stderr, e.context.String(), gitRetryAdvice(category))
}

func classifyGitError(err error, stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "detected dubious ownership"), strings.Contains(lower, "unsafe repository"):
		return "ownership（所有権の確認で拒否）"
	case errors.Is(err, exec.ErrNotFound):
		return "not_found（Git未検出）"
	case errors.Is(err, os.ErrPermission), strings.Contains(lower, "permission denied"), strings.Contains(lower, "access denied"), strings.Contains(lower, "access is denied"), strings.Contains(lower, "operation not permitted"), strings.Contains(lower, "アクセスが拒否されました"), strings.Contains(lower, "許可がありません"):
		return "permission（アクセス権限で拒否）"
	case strings.Contains(lower, "not a git repository"):
		return "not_repository（Git作業ツリー外）"
	case strings.Contains(lower, "bad config line"):
		return "configuration（設定の読取失敗）"
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		return "git_failure（Git起動後の失敗）"
	}
	return "startup（Git起動失敗）"
}

func gitRetryAdvice(category string) string {
	common := "直接実行時と同じ利用者・作業場所で、Gitの実行ファイルのパス識別子と設定の渡し方を確認してください。シェルの別名・関数や親のGIT_*設定は引き継ぎません。"
	switch {
	case strings.HasPrefix(category, "ownership"):
		return common + " 所有者と内容を確認して信頼できる共有先だけ、同じ利用者で親のGIT_*をすべて除外した一時的な子環境から、& '<確認したGitの絶対パス>' config --global --add safe.directory '<確認したGitルートの絶対パス>' を明示的に実行し、元のworkscopeコマンドを再実行してください。GIT_CONFIG_GLOBALなどが残ると別の設定先へ保存されるため、利用ガイドの子環境の作成手順を参照してください。パスはプレースホルダーを置き換え、Gitルート1件だけを指定してください。権限昇格は自動で行いません。"
	case strings.HasPrefix(category, "permission"):
		return common + " 対象の読取権限とGitの実行権限を確認し、必要な対象だけを管理者へ相談してから元のコマンドを再実行してください。"
	case strings.HasPrefix(category, "not_found"):
		return "Gitの導入先とPATHを確認し、同じ利用者でGitを起動できる状態にしてから元のコマンドを再実行してください。"
	case strings.HasPrefix(category, "not_repository"):
		return "対象のGit作業ツリー内へ移動して、元のコマンドを再実行してください。"
	default:
		return common + " 分類と伏字済みの診断を確認し、原因を解消してから元のコマンドを再実行してください。"
	}
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
