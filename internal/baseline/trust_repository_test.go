package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTrustRepositoryIdentityWithOpenDirectory(t *testing.T) {
	dir := repo(t)
	// ほかの読取ハンドルがある状態でも、同一ルートを実体情報で照合する。
	opened, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	options := Options{TrustRepository: dir}
	result, code := BeginWithOptions(dir, []string{"future"}, false, "", options)
	if code != 0 || result.Status != "recorded" {
		t.Fatalf("参照中の同一ルートを誤って拒否しました: %d %+v", code, result)
	}
	result, code = CheckWithOptions(dir, "", options)
	if code != 0 || result.Status != "within_scope" {
		t.Fatalf("参照中の同一ルートを照合できません: %d %+v", code, result)
	}
}

func TestTrustRepositoryIdentityDoesNotReopenPath(t *testing.T) {
	dir := t.TempDir()
	first, err := trustRepositoryIdentity(dir, "指定先")
	if err != nil {
		t.Fatal(err)
	}
	second, err := trustRepositoryIdentity(dir, "Gitルート")
	if err != nil {
		t.Fatal(err)
	}
	// 自分で作った空ディレクトリだけを消し、比較時の遅延参照を検出する。
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(first, second) {
		t.Fatal("取得済みの実体情報を比較せず、消えたパスを再参照しました")
	}
}

func TestTrustRepositoryDisappearanceIsUnknown(t *testing.T) {
	dir := repo(t)
	if result, code := Begin(dir, []string{"future"}, false, ""); code != 0 {
		t.Fatalf("準備失敗: %d %+v", code, result)
	}
	_, before := readRecord(t, dir)
	for _, command := range []string{"begin", "check"} {
		trusted := t.TempDir()
		waiting, removed := false, false
		observer := func(value Progress) {
			if value.Stage == "Git待ち: git rev-parse --show-toplevel" {
				waiting = true
			} else if waiting && !removed && value.Stage == "Git作業場所の確認" {
				// 最初の存在確認後、Gitの終了直後に自分の空の指定先だけを消す。
				if err := os.Remove(trusted); err != nil {
					t.Fatal(err)
				}
				removed = true
			}
		}
		var result Result
		var code int
		if command == "begin" {
			result, code = BeginWithOptions(dir, []string{"future"}, true, "", Options{TrustRepository: trusted}, observer)
		} else {
			result, code = CheckWithOptions(dir, "", Options{TrustRepository: trusted}, observer)
		}
		if !removed || code != 2 || result.Status != "unknown" || result.Coverage.Complete || len(result.Issues) != 1 || result.Issues[0].Code != "trust_repository_unreadable" || !strings.Contains(result.Issues[0].Details, "指定先の実体を開く") {
			t.Fatalf("確認中の消失を誤分類しました: %s %d %+v", command, code, result)
		}
		_, after := readRecord(t, dir)
		if string(before) != string(after) {
			t.Fatal("実体の確認失敗で既存記録を変更しました")
		}
	}
}

func TestTrustRepositoryReferenceFailuresAreSafeAndClassified(t *testing.T) {
	name := filepath.Join(t.TempDir(), "個人名と秘密値")
	for _, tc := range []struct {
		name, code, cause string
		err               error
		rejectMissing     bool
		invalid           bool
	}{
		{"権限拒否", "trust_repository_unreadable", "アクセス拒否", os.ErrPermission, true, false},
		{"不存在", "trust_repository_not_found", "参照先が存在しない", os.ErrNotExist, true, true},
		{"確認中の消失", "trust_repository_unreadable", "参照先が存在しない", os.ErrNotExist, false, false},
		{"原因不明", "trust_repository_unreadable", "未確認", errors.New("秘密のOSエラー本文"), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, stage := range []string{"指定先の実体解決", "指定先の参照", "指定先の実体を開く", "指定先の実体情報取得", "Gitルートの実体解決", "Gitルートの実体を開く", "Gitルートの実体情報取得", "Gitルートを開く"} {
				err := unreadableTrustRepository(stage, name, &os.PathError{Op: "秘密の操作", Path: name, Err: tc.err}, tc.rejectMissing)
				var f *failure
				if !errors.As(err, &f) || f.invalid != tc.invalid {
					t.Fatalf("引数誤りと判定不能の区別が不正です: %+v", err)
				}
				issue := describeError(err)
				if issue.Code != tc.code || issue.Path != "" || !strings.Contains(issue.Message, tc.cause) || !strings.Contains(issue.Message, "確認してください") || !strings.Contains(issue.Details, stage) || !strings.Contains(issue.Details, tc.cause) || !strings.Contains(issue.Details, diagnosticPath(name)) {
					t.Fatalf("確認段階・原因・対処・パス識別子が不正です: %+v", issue)
				}
				for _, secret := range []string{name, filepath.ToSlash(name), "個人名と秘密値", "秘密の操作", "秘密のOSエラー本文"} {
					if strings.Contains(issue.Message+issue.Details, secret) {
						t.Fatal("診断に生のパスまたはOSエラーが漏れました")
					}
				}
			}
		})
	}
}
