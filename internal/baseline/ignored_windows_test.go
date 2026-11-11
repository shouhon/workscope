package baseline

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestWindowsIncludeIgnoredKeepsUnreadableUnknownAndNewFilesExcluded(t *testing.T) {
	dir := repo(t)
	write(t, dir, "logs/known", "開始時")
	beginForCheck(t, dir, ".gitignore", "logs")
	write(t, dir, ".gitignore", "/logs/\n")
	write(t, dir, "tracked.txt", "範囲外の変更")
	write(t, dir, "logs/new.secret", "未記録の無視対象")
	for _, name := range []string{"logs/new.secret", "logs/known"} {
		path, err := syscall.UTF16PtrFromString(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		handle, err := syscall.CreateFile(path, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		result, code := CheckWithOptions(dir, "", Options{IncludeIgnored: []string{"logs/known"}})
		syscall.CloseHandle(handle)
		if name == "logs/new.secret" {
			if code != 1 || !result.Coverage.Complete || len(result.Issues) != 0 {
				t.Fatalf("未記録の無視対象を開こうとしました: %d %+v", code, result)
			}
		} else {
			requireFailure(t, result, code, "unreadable_path")
			if len(result.Issues) != 1 || result.Issues[0].Path != "logs/known" || len(result.OutOfScopeChanges) != 1 || result.OutOfScopeChanges[0].(Change).Path != "tracked.txt" {
				t.Fatalf("読取不能を成功にするか、別の範囲外変更を失いました: %d %+v", code, result)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "logs", "new.secret")); err != nil {
		t.Fatal("未記録の対象を変更しました")
	}
}
