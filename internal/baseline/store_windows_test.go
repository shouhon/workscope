package baseline

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestWindowsPublishUsesCreatedFileHandle(t *testing.T) {
	dir := t.TempDir()
	const temporary, name = "temporary.json", "baseline.json"
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file, err := root.OpenFile(temporary, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("作成した記録"); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename(temporary, "original.tmp"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "other.json", "保護する別の内容")
	if err := os.Symlink("other.json", filepath.Join(dir, temporary)); err != nil {
		t.Skipf("リンクを作成できません: %v", err)
	}
	if err := publishNewRecord(root, file, temporary, name); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string]string{name: "作成した記録", "other.json": "保護する別の内容"} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(data) != expected {
			t.Fatalf("作成した記録以外を移動しました: %s %s %v", path, data, err)
		}
	}
}

func TestWindowsUnreadableEmptyStateIsNotInitialized(t *testing.T) {
	dir := repo(t)
	state := filepath.Join(dir, stateName)
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(state)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	result, code := Begin(dir, []string{"tracked.txt"}, false, "")
	if err := syscall.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	requireFailure(t, result, code, "state_unreadable")
	if code != 2 || result.Coverage.Complete {
		t.Fatalf("読取不能を成功扱いしました: %d %+v", code, result)
	}
	children, err := os.ReadDir(state)
	if err != nil || len(children) != 0 {
		t.Fatalf("読取不能の保存先を変更しました: %v %v", children, err)
	}
}

func TestWindowsLinkedEmptyStateIsNotInitialized(t *testing.T) {
	for _, kind := range []string{"symlink", "junction"} {
		t.Run(kind, func(t *testing.T) {
			dir, outside := repo(t), t.TempDir()
			state := filepath.Join(dir, stateName)
			if kind == "symlink" {
				if err := os.Symlink(outside, state); err != nil {
					t.Skipf("リンクを作成できません: %v", err)
				}
			} else {
				quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
				command := "New-Item -ItemType Junction -Path " + quote(state) + " -Target " + quote(outside) + " -ErrorAction Stop | Out-Null"
				if output, err := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput(); err != nil {
					t.Skipf("ジャンクションを作成できません: %v %s", err, output)
				}
			}
			result, code := Begin(dir, []string{"tracked.txt"}, true, "")
			requireFailure(t, result, code, "foreign_state")
			children, err := os.ReadDir(outside)
			if err != nil || len(children) != 0 {
				t.Fatalf("外部の保存先へ書き込みました: %v %v", children, err)
			}
		})
	}
}

func TestWindowsListUnreadableRecordIsUnknown(t *testing.T) {
	dir := repo(t)
	if result, code := Begin(dir, []string{"future"}, false, ""); code != 0 {
		t.Fatalf("準備失敗: %d %+v", code, result)
	}
	before := taskData(t, dir, "")
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, stateName, recordName))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	result, code := List(dir, Options{})
	closeErr := syscall.CloseHandle(handle)
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if code != 2 || result.Status != "unknown" || len(result.Issues) != 1 || result.Issues[0].Code != "state_unreadable" || len(result.Records) != 0 {
		t.Fatalf("読取不能を正常な0件にしました: %d %+v", code, result)
	}
	if !bytes.Equal(before, taskData(t, dir, "")) {
		t.Fatal("読取不能の一覧が記録を変更しました")
	}
}

func TestWindowsFailedReplacementPreservesBaseline(t *testing.T) {
	for _, task := range []string{"", "日本語 作業"} {
		t.Run(task, func(t *testing.T) {
			dir := repo(t)
			result, code := Begin(dir, []string{"."}, false, task)
			if code != 0 {
				t.Fatalf("%+v", result)
			}
			before := taskData(t, dir, task)
			name, err := syscall.UTF16PtrFromString(filepath.Join(dir, stateName, RecordName(task)))
			if err != nil {
				t.Fatal(err)
			}
			// 読み書きは許すが削除共有を許さず、Windowsの置換失敗を再現する。
			handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.CloseHandle(handle)
			write(t, dir, "tracked.txt", "取り直し時の本文")
			result, code = Begin(dir, []string{"."}, true, task)
			requireFailure(t, result, code, "save_failed")
			if !bytes.Equal(before, taskData(t, dir, task)) {
				t.Fatal("保存失敗で以前の開始記録が失われました")
			}
			children, _ := os.ReadDir(filepath.Join(dir, stateName))
			if len(children) != 1 || children[0].Name() != RecordName(task) {
				t.Fatalf("保存失敗後に一時ファイルが残りました: %v", children)
			}
		})
	}
}

func TestWindowsScopeExtensionFailuresPreserveBaseline(t *testing.T) {
	for _, kind := range []string{"replacement", "record_unreadable", "file_unreadable"} {
		t.Run(kind, func(t *testing.T) {
			dir := extensionRepo(t)
			if result, code := Begin(dir, []string{"tracked.txt"}, false, ""); code != 0 {
				t.Fatalf("準備失敗: %d %+v", code, result)
			}
			before := taskData(t, dir, "")
			path := filepath.Join(dir, stateName, recordName)
			var share uint32
			if kind == "replacement" {
				share = syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE
			}
			if kind == "file_unreadable" {
				path = filepath.Join(dir, "outside.txt")
			}
			name, err := syscall.UTF16PtrFromString(path)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, share, nil, syscall.OPEN_EXISTING, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			result, code := BeginWithOptions(dir, []string{"second.txt"}, false, "", Options{Extend: true, Reason: "追加"})
			if err := syscall.CloseHandle(handle); err != nil {
				t.Fatal(err)
			}
			if code != 2 || result.Status != "unknown" || result.Coverage.Complete || len(result.Issues) == 0 {
				t.Fatalf("読取不能か保存失敗を成功扱い: %d %+v", code, result)
			}
			if kind == "replacement" {
				requireFailure(t, result, code, "save_failed")
			}
			children, err := os.ReadDir(filepath.Join(dir, stateName))
			if err != nil || len(children) != 1 || !bytes.Equal(before, taskData(t, dir, "")) {
				t.Fatalf("失敗後に記録が変わったか一時ファイルが残存: %v %v", children, err)
			}
		})
	}
}

func TestWindowsUnreadableFileNeverRecordsSuccess(t *testing.T) {
	dir := repo(t)
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	result, code := Begin(dir, []string{"."}, false, "")
	if code != 2 || result.Status != "unknown" {
		t.Fatalf("読めない対象を成功扱いしました: %d %+v", code, result)
	}
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("読めない対象で開始記録を保存しました")
	}
}

func TestWindowsCheckKeepsOutOfScopeChangesWithUnreadableFile(t *testing.T) {
	dir := repo(t)
	write(t, dir, "outside.txt", "開始時")
	beginForCheck(t, dir, "future")
	write(t, dir, "outside.txt", "範囲外の更新")
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, "tracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	result, code := Check(dir, "")
	if code != 2 || result.Status != "unknown" || result.Coverage.Complete || len(result.Issues) != 1 || result.Issues[0].Path != "tracked.txt" || len(result.ChangesSinceBegin) != 1 || result.ChangesSinceBegin[0].Path != "outside.txt" || len(result.OutOfScopeChanges) != 1 {
		t.Fatalf("読取不能と範囲外の併存が不正: %d %+v", code, result)
	}
}

func TestWindowsCheckAllowHandlesCaseAndSeparators(t *testing.T) {
	dir := repo(t)
	write(t, dir, "Src/日本語 空白.txt", "開始時")
	beginForCheck(t, dir, `src\日本語 空白.txt`, `FUTURE\New`)
	write(t, dir, "Src/日本語 空白.txt", "更新")
	write(t, dir, "future/new/追加.txt", "未作成のディレクトリ配下")
	result, code := Check(dir, "")
	if code != 0 || len(result.ChangesSinceBegin) != 2 || len(result.OutOfScopeChanges) != 0 {
		t.Fatalf("大小文字・区切りの照合失敗: %d %+v", code, result)
	}
}

func TestWindowsAllowUsesActualNames(t *testing.T) {
	dir := repo(t)
	write(t, dir, "Src/日本語.txt", "本文")
	r, err := openRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.fs.Close()
	allowed, err := r.normalizeAllow([]string{"src", "SRC/", `Src\日本語.txt`})
	if err != nil || len(allowed) != 2 || allowed[0].Path != "Src" || allowed[1].Path != "Src/日本語.txt" {
		t.Fatalf("同一パスの正規化が不正: %+v %v", allowed, err)
	}
}

func TestWindowsAllowDoesNotConfuseHardLinkNames(t *testing.T) {
	dir := repo(t)
	if err := os.Link(filepath.Join(dir, "tracked.txt"), filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	r, err := openRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.fs.Close()
	allowed, err := r.normalizeAllow([]string{"TRACKED.TXT", "tracked.txt", "a.txt"})
	if err != nil || len(allowed) != 2 || allowed[0].Path != "a.txt" || allowed[1].Path != "tracked.txt" {
		t.Fatalf("別名を同一パスへまとめました: %+v %v", allowed, err)
	}
}

func TestWindowsFutureAllowDeduplicatesCase(t *testing.T) {
	dir := repo(t)
	r, err := openRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.fs.Close()
	allowed, err := r.normalizeAllow([]string{"future.txt", "FUTURE.TXT", `future\new.txt`, "FUTURE/new.txt"})
	if err != nil || len(allowed) != 2 {
		t.Fatalf("未作成の同一パスが重複しています: %+v %v", allowed, err)
	}
}

func TestWindowsJunctionIsNeverTraversed(t *testing.T) {
	dir := repo(t)
	outside := t.TempDir()
	write(t, outside, "outside-only.txt", "外部の本文")
	junction := filepath.Join(dir, "junction")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	// パスはPowerShellの文字列として引用し、シェル命令として解釈させない。
	command := "New-Item -ItemType Junction -Path " + quote(junction) + " -Target " + quote(outside) + " -ErrorAction Stop | Out-Null"
	if output, err := exec.Command("pwsh", "-NoProfile", "-NonInteractive", "-Command", command).CombinedOutput(); err != nil {
		t.Skipf("ジャンクションを作成できません: %v %s", err, output)
	}
	result, code := Begin(dir, []string{"."}, false, "")
	requireFailure(t, result, code, "unsupported_kind")
	if result.Coverage.Complete {
		t.Fatal("ジャンクションがある状態を完全な記録と扱いました")
	}
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("外部へつながるジャンクションがある状態で記録しました")
	}
}
