package baseline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

func trustRepositoryIssue(code, message, stage, cause, name string) Issue {
	return Issue{Code: code, Message: message, Details: fmt.Sprintf("失敗した確認段階: %s\n  原因区分: %s\n  確認対象: %s", stage, cause, diagnosticPath(name))}
}

func resolveTrustRepository(name string) (string, error) {
	if !utf8.ValidString(name) || !filepath.IsAbs(name) || strings.ContainsAny(name, "*?[") || strings.ContainsFunc(name, unicode.IsControl) {
		return "", &failure{invalid: true, issue: trustRepositoryIssue("invalid_trust_repository", "--trust-repository の形式が不正です。対象のGitルート1件の絶対パスを指定してください。ワイルドカードや制御文字は使えません。", "引数の形式確認", "形式不正", "")}
	}
	trusted, err := filepath.EvalSymlinks(filepath.Clean(name))
	if err != nil {
		return "", unreadableTrustRepository("指定先の実体解決", name, err, true)
	}
	info, err := os.Stat(trusted)
	if err != nil {
		return "", unreadableTrustRepository("指定先の参照", trusted, err, true)
	}
	if !info.IsDir() {
		return "", &failure{invalid: true, issue: trustRepositoryIssue("trust_repository_not_directory", "--trust-repository の指定先はディレクトリではありません。対象のGitルートのディレクトリを指定してください。", "指定先の種類確認", "ディレクトリ以外", trusted)}
	}
	return trusted, nil
}

func trustRepositoryIdentity(name, target string) (os.FileInfo, error) {
	opened, err := os.Open(name)
	if err != nil {
		return nil, unreadableTrustRepository(target+"の実体を開く", name, err, false)
	}
	defer opened.Close()
	// Windowsのパス名由来のStatではSameFileが実体情報を遅延取得し、
	// その参照失敗もfalseになる。ハンドルから取得して失敗を明示する。
	info, err := opened.Stat()
	if err != nil {
		return nil, unreadableTrustRepository(target+"の実体情報取得", name, err, false)
	}
	return info, nil
}

func unreadableTrustRepository(stage, name string, err error, rejectMissing bool) error {
	code := "trust_repository_unreadable"
	cause := "未確認（参照エラーを安全に分類できません）"
	advice := "指定先の存在、読取・参照権限、実行環境の制限を確認してください。"
	invalid := false
	switch {
	case errors.Is(err, os.ErrPermission):
		cause = "アクセス拒否"
		advice = "指定先と親ディレクトリの読取・参照権限、実行環境の制限を確認してください。"
	case errors.Is(err, os.ErrNotExist):
		cause = "参照先が存在しない"
		if rejectMissing {
			code, invalid = "trust_repository_not_found", true
		}
	}
	// OSエラーの本文やパスは含めず、確認できた原因区分だけを返す。
	message := fmt.Sprintf("--trust-repository の確認に失敗しました（%s、%s）。%s", stage, cause, advice)
	return &failure{invalid: invalid, issue: trustRepositoryIssue(code, message, stage, cause, name)}
}
