package cli_test

import (
	"path/filepath"
	"syscall"
	"testing"
)

func TestWindowsJSONSummaryUnreadableFileKeepsUnknownTotals(t *testing.T) {
	dir := summaryRepo(t)
	t.Chdir(dir)
	runSummary(t, []string{"begin", "--allow", "input.txt"}, 0)
	summaryWrite(t, dir, "outside.txt", "検出できる範囲外の変更")
	name, err := syscall.UTF16PtrFromString(filepath.Join(dir, "input.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// 共有を許さず、実際にファイルを読み取れない状態にする。
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)
	result := runSummary(t, []string{"check"}, 2)
	requireUnknownSummaryCounts(t, result)
	if result.Issues[0].Code != "unreadable_path" || result.ObservedCounts["changes_since_begin"] != 1 || result.ObservedCounts["out_of_scope_changes"] != 1 {
		t.Fatalf("読取不能と検出済みの範囲外件数が不正: %+v", result)
	}
}
