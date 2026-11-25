package baseline

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkBatchesCheckIgnoreByDepthAndSkipsTrackedFiles(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".gitignore", "ignored_dir/\n*.tmp\n")
	write(t, dir, "ignored_dir/secret.txt", "無視されたディレクトリ配下")
	// 多数のサブディレクトリを作成し、そのうち一部だけに未追跡ファイルやネストした .gitignore を置く。
	for i := 0; i < 12; i++ {
		sub := fmt.Sprintf("pkg/%02d", i)
		write(t, dir, sub+"/tracked.txt", fmt.Sprintf("tracked-%d", i))
	}
	write(t, dir, "pkg/03/.gitignore", "local.log\n!keep.tmp\n")
	testGit(t, dir, "add", ".gitignore", "pkg")
	testGit(t, dir, "commit", "-m", "initial")

	// 未追跡ファイルと無視ファイルを混在させる。
	write(t, dir, "pkg/03/local.log", "ネストしたgitignoreで無視")
	write(t, dir, "pkg/03/keep.tmp", "否定ルールで追跡対象")
	write(t, dir, "pkg/05/untracked.txt", "未追跡")
	write(t, dir, "pkg/05/skip.tmp", "ルートのgitignoreで無視")

	checkIgnoreCalls := map[int]int{}
	prevStage := ""
	observer := Observer(func(p Progress) {
		if p.Stage == "Git待ち: git check-ignore --no-index -z --stdin" && prevStage != p.Stage {
			checkIgnoreCalls[p.Pass]++
		}
		prevStage = p.Stage
	})

	result, code := Begin(dir, []string{"pkg"}, false, "", observer)
	if code != 0 || !result.Coverage.Complete {
		t.Fatalf("begin に失敗しました: code=%d result=%+v", code, result)
	}

	// 深さ: ルート(pkg/, ignored_dir/) -> pkg(00..11/) -> 葉(未追跡がある階層のみ)
	// 1パスあたり最大3回に収まり、14回以上（ディレクトリごと）にはならない。
	for _, pass := range []int{1, 2} {
		if checkIgnoreCalls[pass] == 0 || checkIgnoreCalls[pass] > 3 {
			t.Fatalf("パス %d の check-ignore 起動回数が想定外です: got=%d (want 1..3)", pass, checkIgnoreCalls[pass])
		}
	}

	record, _ := readRecord(t, dir)
	recorded := map[string]bool{}
	for _, entry := range record.Files {
		recorded[entry.Path] = true
	}
	for _, want := range []string{"pkg/00/tracked.txt", "pkg/03/keep.tmp", "pkg/05/untracked.txt"} {
		if !recorded[want] {
			t.Fatalf("記録されるべきパスがありません: %s", want)
		}
	}
	for _, unwanted := range []string{"ignored_dir/secret.txt", "pkg/03/local.log", "pkg/05/skip.tmp"} {
		if recorded[unwanted] {
			t.Fatalf("無視されるべきパスが記録されました: %s", unwanted)
		}
	}
}

func TestWalkSkipsCheckIgnoreForTrackedOnlyLeafDirectories(t *testing.T) {
	dir := repo(t)
	for i := 0; i < 6; i++ {
		write(t, dir, fmt.Sprintf("d%d/a.txt", i), "tracked")
	}
	testGit(t, dir, "add", ".")
	testGit(t, dir, "commit", "-m", "initial")

	checkIgnoreCalls := map[int]int{}
	prevStage := ""
	observer := Observer(func(p Progress) {
		if strings.HasPrefix(p.Stage, "Git待ち: git check-ignore") && prevStage != p.Stage {
			checkIgnoreCalls[p.Pass]++
		}
		prevStage = p.Stage
	})

	result, code := Begin(dir, []string{"d0"}, false, "", observer)
	if code != 0 || !result.Coverage.Complete {
		t.Fatalf("begin に失敗しました: code=%d result=%+v", code, result)
	}

	// ルート直下の d0..d5 の判定の1回だけで済み、葉ディレクトリ（追跡済みファイルのみ）では起動しない。
	for _, pass := range []int{1, 2} {
		if checkIgnoreCalls[pass] != 1 {
			t.Fatalf("パス %d の check-ignore 起動回数が想定外です: got=%d want=1 (dir=%s)", pass, checkIgnoreCalls[pass], filepath.Base(dir))
		}
	}
}
