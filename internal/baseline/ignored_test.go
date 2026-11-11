package baseline

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestIncludeIgnoredComparesKnownFilesWithoutUpdatingBaseline(t *testing.T) {
	for _, kind := range []string{"unchanged", "modified", "outside", "deleted", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			name := "logs/既知 空白.log"
			write(t, dir, ".gitignore", "/.workscope/\n")
			write(t, dir, name, "開始時")
			allow := []string{".gitignore"}
			if kind != "outside" {
				allow = append(allow, name)
			}
			record := beginForCheck(t, dir, allow...)
			_, baselineBefore := readRecord(t, dir)
			write(t, dir, ".gitignore", "/.workscope/\n/logs/\n")
			write(t, dir, "logs/new.secret", "開始時にない無視対象")
			switch kind {
			case "modified", "outside":
				write(t, dir, name, "開始後に変更")
			case "deleted", "directory", "symlink":
				if err := os.Remove(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
					t.Fatal(err)
				}
				if kind == "directory" {
					write(t, dir, name+"/new.secret", "種類変更で生じた配下は読まない")
				}
				if kind == "symlink" {
					outside := t.TempDir()
					write(t, outside, "外部.txt", "リンク先の内容は読まない")
					if err := os.Symlink(filepath.Join(outside, "外部.txt"), filepath.Join(dir, filepath.FromSlash(name))); err != nil {
						t.Skipf("この環境ではリンクを作成できません: %v", err)
					}
				}
			}
			testGit(t, dir, "add", ".gitignore")
			indexBefore, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
			configBefore, _ := os.ReadFile(filepath.Join(dir, ".git", "config"))
			result, code := CheckWithOptions(dir, "", Options{IncludeIgnored: []string{name}})
			wantCode, wantStatus := 0, "within_scope"
			if kind == "outside" {
				wantCode, wantStatus = 1, "out_of_scope"
			}
			if code != wantCode || result.Status != wantStatus || !result.Coverage.Complete || len(result.Issues) != 0 || result.BaselineID != record.ID || !reflect.DeepEqual(result.Coverage.IgnoredBaselinePaths, []string{name}) {
				t.Fatalf("明示対象の照合が不正: %d %+v", code, result)
			}
			wantChanges := 2
			if kind == "unchanged" {
				wantChanges = 1
			}
			if len(result.ChangesSinceBegin) != wantChanges || len(result.IndexChanges) != 1 || result.IndexChanges[0].Path != ".gitignore" || len(result.OutOfScopeChanges) != wantCode {
				t.Fatalf("本文・インデックス・範囲外の差分が不正: %+v", result)
			}
			if kind != "unchanged" {
				change := result.ChangesSinceBegin[1]
				wantKind := "modified"
				if kind == "deleted" {
					wantKind = "deleted"
				} else if kind == "directory" || kind == "symlink" {
					wantKind = "type_changed"
				}
				if change.Path != name || change.Kind != wantKind || change.BeforeSHA256 == change.AfterSHA256 {
					t.Fatalf("無視対象の変更を検出できません: %+v", change)
				}
			}
			_, baselineAfter := readRecord(t, dir)
			indexAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
			configAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "config"))
			if !bytes.Equal(baselineBefore, baselineAfter) || !bytes.Equal(indexBefore, indexAfter) || !bytes.Equal(configBefore, configAfter) {
				t.Fatal("照合が開始記録・インデックス・設定を変更しました")
			}
			// 明示指定は保存されず、次回は再び既定の保護が働く。
			ordinary, ordinaryCode := Check(dir, "")
			requireFailure(t, ordinary, ordinaryCode, "coverage_changed")
		})
	}
}

func TestIncludeIgnoredDoesNotIncludeOtherKnownFiles(t *testing.T) {
	dir := repo(t)
	write(t, dir, "logs/first", "開始時")
	write(t, dir, "logs/second", "開始時")
	beginForCheck(t, dir, ".gitignore", "logs/first")
	write(t, dir, ".gitignore", "/logs/\n")
	write(t, dir, "logs/second", "範囲外の変更")
	result, code := CheckWithOptions(dir, "", Options{IncludeIgnored: []string{"logs/first"}})
	requireFailure(t, result, code, "coverage_changed")
	if len(result.Issues) != 1 || result.Issues[0].Path != "logs/second" || len(result.ChangesSinceBegin) != 1 {
		t.Fatalf("指定していない既知ファイルを照合しました: %d %+v", code, result)
	}
	result, code = CheckWithOptions(dir, "", Options{IncludeIgnored: []string{"logs/second", "./logs/first", "logs/first"}})
	if code != 1 || len(result.OutOfScopeChanges) != 1 || !reflect.DeepEqual(result.Coverage.IgnoredBaselinePaths, []string{"logs/first", "logs/second"}) {
		t.Fatalf("複数指定の範囲外変更または重複排除が不正: %d %+v", code, result)
	}
}

func TestIncludeIgnoredRejectsUnrecordedPathsAndDirectories(t *testing.T) {
	dir := repo(t)
	write(t, dir, "logs/known", "開始時")
	beginForCheck(t, dir, ".gitignore")
	write(t, dir, ".gitignore", "/logs/\n")
	write(t, dir, "logs/new", "未記録")
	_, before := readRecord(t, dir)
	for _, name := range []string{"", ".", "logs", "logs/new", "logs/*", "../known", filepath.Join(dir, "logs", "known"), ".git/HEAD", ".workscope/baseline.json", "logs/known\x00"} {
		result, code := CheckWithOptions(dir, "", Options{IncludeIgnored: []string{"logs/known", name}})
		if code != 64 || result.Status != "invalid_arguments" || result.Coverage.Complete || len(result.ChangesSinceBegin) != 0 {
			t.Fatalf("不正な指定 %q を受け付けました: %d %+v", name, code, result)
		}
	}
	_, after := readRecord(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatal("引数の拒否で開始記録を変更しました")
	}
}

func TestIncludeIgnoredUsesRootRelativeCanonicalPaths(t *testing.T) {
	dir := repo(t)
	name := "Logs/日本語 空白.txt"
	write(t, dir, name, "開始時")
	beginForCheck(t, dir, ".gitignore", name)
	write(t, dir, ".gitignore", "/Logs/\n")
	write(t, dir, name, "開始後")
	alias := "./" + name
	if runtime.GOOS == "windows" {
		alias = strings.ReplaceAll(strings.ToUpper(name), "/", `\`)
	}
	result, code := CheckWithOptions(filepath.Join(dir, "Logs"), "", Options{IncludeIgnored: []string{alias, name}})
	if code != 0 || !reflect.DeepEqual(result.Coverage.IgnoredBaselinePaths, []string{name}) || len(result.ChangesSinceBegin) != 2 || result.ChangesSinceBegin[1].Path != name {
		t.Fatalf("ルート基準の実名へ正規化できません: %d %+v", code, result)
	}
}

func TestIncludeIgnoredDoesNotFollowReplacedParentLink(t *testing.T) {
	dir, outside := repo(t), t.TempDir()
	write(t, dir, "logs/known", "開始時")
	beginForCheck(t, dir, ".gitignore", "logs")
	write(t, dir, ".gitignore", "/logs/\n")
	write(t, outside, "known", "外部の内容")
	if err := os.Remove(filepath.Join(dir, "logs", "known")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "logs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "logs")); err != nil {
		t.Skipf("この環境ではリンクを作成できません: %v", err)
	}
	result, code := CheckWithOptions(dir, "", Options{IncludeIgnored: []string{"logs/known"}})
	requireFailure(t, result, code, "unsafe_parent")
	for _, change := range result.ChangesSinceBegin {
		if strings.HasPrefix(change.Path, "logs/") {
			t.Fatalf("親リンク配下を照合しました: %+v", result)
		}
	}
}

func TestIncludeIgnoredRejectsChangesBetweenSnapshots(t *testing.T) {
	dir := repo(t)
	write(t, dir, "logs/known", "開始時")
	beginForCheck(t, dir, ".gitignore", "logs/known")
	write(t, dir, ".gitignore", "/logs/\n")
	changed := false
	result, code := CheckWithOptions(dir, "", Options{IncludeIgnored: []string{"logs/known"}}, func(p Progress) {
		if !changed && p.Pass == 2 {
			changed = true
			write(t, dir, "logs/known", "取得の間の変更")
		}
	})
	if !changed {
		t.Fatal("取得中の変化を再現できませんでした")
	}
	requireFailure(t, result, code, "snapshot_changed")
}
