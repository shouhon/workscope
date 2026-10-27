package baseline

import (
	"strings"
	"testing"
)

func TestProgressCountsEachSnapshotWithoutIgnoredOrStateFiles(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".gitignore", "ignored/\n")
	write(t, dir, "ignored/private.txt", "走査しない本文")
	write(t, dir, "dir/untracked.txt", "走査する本文")
	for _, command := range []string{"begin", "check"} {
		var events []Progress
		observer := Observer(func(value Progress) { events = append(events, value) })
		var result Result
		var code int
		if command == "begin" {
			result, code = Begin(dir, []string{"dir"}, false, "", observer)
		} else {
			result, code = Check(dir, "", observer)
		}
		if code != 0 {
			t.Fatalf("%s の実行失敗: %+v", command, result)
		}
		maxFiles := map[int]int{}
		gitWait, scanning := false, false
		for _, event := range events {
			gitWait = gitWait || strings.HasPrefix(event.Stage, "Git待ち: git ")
			scanning = scanning || event.Stage == "ファイル走査"
			if event.Files > maxFiles[event.Pass] {
				maxFiles[event.Pass] = event.Files
			}
			if strings.Contains(event.Stage, "private") || strings.Contains(event.Stage, "untracked.txt") {
				t.Fatal("進捗にファイル名が含まれています")
			}
		}
		if !gitWait || !scanning || maxFiles[1] != 3 || maxFiles[2] != 3 {
			t.Fatalf("%s の段階または取得回ごとの件数が不正: Git待ち=%t 走査=%t 件数=%v", command, gitWait, scanning, maxFiles)
		}
	}
}
