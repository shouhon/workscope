package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUnreadableAllowFailuresAreSafeAndClassified(t *testing.T) {
	secretRoot := filepath.Join(t.TempDir(), "個人名と秘密値")
	candidate := "skills/sub/validate.py"
	for _, tc := range []struct {
		name, cause string
		err         error
	}{
		{"権限拒否", "アクセス拒否", os.ErrPermission},
		{"確認中の消失", "確認中に参照先が存在しなくなった", os.ErrNotExist},
		{"原因不明", "未確認（参照エラーを安全に分類できません）", errors.New("秘密のOSエラー本文")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, stage := range []string{"許容対象のパス参照", "許容対象の実名確認"} {
				err := unreadableAllow(stage, candidate, &os.PathError{Op: "秘密の操作", Path: filepath.Join(secretRoot, filepath.FromSlash(candidate)), Err: tc.err})
				var f *failure
				if !errors.As(err, &f) || f.invalid {
					t.Fatalf("参照失敗を引数誤りとして分類しました: %+v", err)
				}
				issue := describeError(err)
				if issue.Code != "unreadable_path" || issue.Path != candidate || !strings.Contains(issue.Message, stage) || !strings.Contains(issue.Message, tc.cause) || !strings.Contains(issue.Details, stage) || !strings.Contains(issue.Details, tc.cause) || !strings.Contains(issue.Details, `"`+candidate+`"`) {
					t.Fatalf("診断の項目が不正です: %+v", issue)
				}
				for _, secret := range []string{secretRoot, filepath.ToSlash(secretRoot), "個人名と秘密値", "秘密の操作", "秘密のOSエラー本文"} {
					if strings.Contains(issue.Message+issue.Details, secret) {
						t.Fatalf("診断に生のパスまたはOSエラーが漏れました: %q in %+v", secret, issue)
					}
				}
			}
		})
	}
}

func TestNormalizeAllowActualNameFailureIsUnknown(t *testing.T) {
	dir := repo(t)
	write(t, dir, "Skills/Sub/validate.py", "内容")
	r, err := openRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.fs.Close()

	// 既存配下の未作成パスと "." が期待どおり正規化されることも確認する。
	allowed, err := r.normalizeAllowWith([]string{".", "Skills/new/deep.txt"}, actualName)
	if err != nil || !reflect.DeepEqual(allowed, []Allowed{{".", "directory"}, {"Skills/new/deep.txt", "future"}}) {
		t.Fatalf("正常系の正規化が不正です: %+v %v", allowed, err)
	}

	for _, tc := range []struct {
		name, cause string
		err         error
	}{
		{"権限拒否", "アクセス拒否", os.ErrPermission},
		{"確認中の消失", "確認中に参照先が存在しなくなった", os.ErrNotExist},
		{"原因不明", "未確認（参照エラーを安全に分類できません）", errors.New("秘密のOSエラー本文")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failing := func(path string) (string, error) {
				if strings.EqualFold(filepath.Base(path), "sub") {
					return "", &os.PathError{Op: "GetLongPathName", Path: path, Err: tc.err}
				}
				return actualName(path)
			}
			_, err := r.normalizeAllowWith([]string{"Skills/Sub/validate.py"}, failing)
			var f *failure
			if !errors.As(err, &f) || f.invalid {
				t.Fatalf("実名確認の失敗が判定不能ではありません: %+v", err)
			}
			issue := describeError(err)
			if issue.Code != "unreadable_path" || issue.Path != "Skills/Sub" || !strings.Contains(issue.Details, "許容対象の実名確認") || !strings.Contains(issue.Details, tc.cause) {
				t.Fatalf("実名確認の失敗診断が不正です: %+v", issue)
			}
			for _, secret := range []string{r.root, filepath.ToSlash(r.root), "GetLongPathName", "秘密のOSエラー本文"} {
				if strings.Contains(issue.Message+issue.Details, secret) {
					t.Fatalf("診断に生のパスまたはOSエラーが漏れました: %q in %+v", secret, issue)
				}
			}
		})
	}
}
