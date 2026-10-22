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

func TestWindowsFailedReplacementPreservesBaseline(t *testing.T) {
	dir := repo(t)
	result, code := Begin(dir, []string{"."}, false)
	if code != 0 {
		t.Fatalf("%+v", result)
	}
	_, before := readRecord(t, dir)
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, stateName, recordName))
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
	result, code = Begin(dir, []string{"."}, true)
	requireFailure(t, result, code, "save_failed")
	_, after := readRecord(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatal("保存失敗で以前の開始記録が失われました")
	}
	children, _ := os.ReadDir(filepath.Join(dir, stateName))
	if len(children) != 1 || children[0].Name() != recordName {
		t.Fatalf("保存失敗後に一時ファイルが残りました: %v", children)
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
	result, code := Begin(dir, []string{"."}, false)
	if code != 2 || result.Status != "unknown" {
		t.Fatalf("読めない対象を成功扱いしました: %d %+v", code, result)
	}
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("読めない対象で開始記録を保存しました")
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
	result, code := Begin(dir, []string{"."}, false)
	requireFailure(t, result, code, "unsupported_kind")
	if result.Coverage.Complete {
		t.Fatal("ジャンクションがある状態を完全な記録と扱いました")
	}
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("外部へつながるジャンクションがある状態で記録しました")
	}
}
