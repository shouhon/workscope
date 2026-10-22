package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

type snapshot struct {
	Head       string
	Index      []IndexEntry
	Files      []Entry
	Changes    []GitChange
	Submodules []string
}

func (r *repository) readEntry(name string) (Entry, error) {
	entry := Entry{Path: name, Kind: "missing"}
	if err := r.safeParents(name); err != nil {
		return entry, err
	}
	before, err := r.fs.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return entry, nil
	}
	if err != nil {
		return entry, fail("unreadable_path", "対象を読み取れません。", name)
	}
	switch {
	case before.Mode()&os.ModeSymlink != 0:
		entry.Kind = "symlink"
		entry.Target, err = r.fs.Readlink(name)
		if err != nil {
			return entry, fail("unreadable_link", "リンク先の文字列を読み取れません。", name)
		}
		if !utf8.ValidString(entry.Target) {
			return entry, fail("unsupported_encoding", "リンク先の文字列をUTF-8で記録できません。", name)
		}
		hash := sha256.Sum256([]byte(entry.Target))
		entry.SHA256 = hex.EncodeToString(hash[:])
	case before.IsDir():
		entry.Kind = "directory"
	case before.Mode().IsRegular():
		entry.Kind = "file"
		file, err := r.fs.Open(name)
		if err != nil {
			return entry, fail("unreadable_path", "ファイルを開けません。", name)
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
			file.Close()
			return entry, fail("snapshot_changed", "読み取り中に対象が変化しました。", name)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		after, statErr := file.Stat()
		closeErr := file.Close()
		if copyErr != nil || statErr != nil || closeErr != nil {
			return entry, fail("unreadable_path", "ファイル全体を読み取れません。", name)
		}
		if before.Size() != after.Size() || before.ModTime() != after.ModTime() {
			return entry, fail("snapshot_changed", "読み取り中にファイルが変化しました。", name)
		}
		entry.SHA256 = hex.EncodeToString(hash.Sum(nil))
	default:
		return entry, fail("unsupported_kind", "通常のファイル・ディレクトリ・リンク以外は記録できません。", name)
	}
	after, err := r.fs.Lstat(name)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.ModTime() != after.ModTime() {
		return entry, fail("snapshot_changed", "読み取り中に対象が変化しました。", name)
	}
	return entry, nil
}

func (r *repository) collect() (snapshot, error) {
	s := snapshot{Files: []Entry{}, Changes: []GitChange{}, Submodules: []string{}}
	var err error
	s.Head, err = r.head()
	if err != nil {
		return s, err
	}
	s.Index, err = r.index()
	if err != nil {
		return s, err
	}
	tracked := map[string]bool{}
	for _, entry := range s.Index {
		if r.protected(entry.Path) {
			continue
		}
		if entry.Mode == "160000" {
			if len(s.Submodules) == 0 || s.Submodules[len(s.Submodules)-1] != entry.Path {
				s.Submodules = append(s.Submodules, entry.Path)
			}
			continue
		}
		if err := r.safeParents(entry.Path); err != nil {
			return s, err
		}
		tracked[entry.Path] = true
	}
	if len(s.Submodules) != 0 {
		return s, fail("unsupported_submodule", "サブモジュールは判定対象外のため、完全な開始記録を作れません。", s.Submodules[0])
	}
	entries := map[string]Entry{}
	trackedNames := make([]string, 0, len(tracked))
	for name := range tracked {
		trackedNames = append(trackedNames, name)
	}
	sort.Strings(trackedNames)
	for _, name := range trackedNames {
		entry, err := r.readEntry(name)
		if err != nil {
			return s, err
		}
		entries[name] = entry
	}
	var walk func(string) error
	walk = func(dir string) error {
		if err := r.safeParents(filepath.Join(dir, "_")); err != nil {
			return err
		}
		file, err := r.fs.Open(dir)
		if err != nil {
			return fail("unreadable_directory", "ディレクトリを開けません。", dir)
		}
		children, readErr := file.ReadDir(-1)
		file.Close()
		if readErr != nil {
			return fail("unreadable_directory", "ディレクトリを読み取れません。", dir)
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		names := []string{}
		for _, child := range children {
			name := filepath.ToSlash(filepath.Join(dir, child.Name()))
			if r.protected(name) {
				continue
			}
			info, err := r.fs.Lstat(name)
			if err != nil {
				return fail("snapshot_changed", "列挙中に対象を確認できなくなりました。", name)
			}
			if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				name += "/"
			}
			names = append(names, name)
		}
		ignored, err := r.ignored(names)
		if err != nil {
			return err
		}
		for _, name := range names {
			if ignored[name] {
				continue
			}
			if strings.HasSuffix(name, "/") {
				if err := walk(strings.TrimSuffix(name, "/")); err != nil {
					return err
				}
				continue
			}
			if _, exists := entries[name]; exists {
				continue
			}
			entry, err := r.readEntry(name)
			if err != nil {
				return err
			}
			if entry.Kind == "missing" {
				return fail("snapshot_changed", "列挙中に対象が削除されました。", name)
			}
			entries[name] = entry
			if !tracked[name] {
				s.Changes = append(s.Changes, GitChange{name, "??"})
			}
		}
		return nil
	}
	if err := walk("."); err != nil {
		return s, err
	}
	// 未追跡ファイルは上でリンクをたどらず列挙済み。Gitには追跡済みだけを問い合わせる。
	data, err := git(r.root, nil, "status", "--porcelain=v1", "-z", "--untracked-files=no", "--ignore-submodules=all", "--no-renames")
	if err != nil {
		return s, fail("status_unreadable", "Gitの変更状態を読み取れません。", "")
	}
	changes, err := parseStatus(data)
	if err != nil {
		return s, err
	}
	for _, change := range changes {
		if !r.protected(change.Path) {
			s.Changes = append(s.Changes, change)
		}
	}
	for _, entry := range entries {
		s.Files = append(s.Files, entry)
	}
	sort.Slice(s.Files, func(i, j int) bool { return s.Files[i].Path < s.Files[j].Path })
	sort.Slice(s.Changes, func(i, j int) bool { return s.Changes[i].Path < s.Changes[j].Path })
	filtered := []IndexEntry{}
	for _, entry := range s.Index {
		if !r.protected(entry.Path) {
			filtered = append(filtered, entry)
		}
	}
	// 取得中のHEAD・インデックスの変化は、完全な再取得と合わせて検出する。
	head, err := r.head()
	if err != nil {
		return s, err
	}
	index, err := r.index()
	if err != nil {
		return s, err
	}
	if head != s.Head || !reflect.DeepEqual(index, s.Index) {
		return s, fail("snapshot_changed", "取得中にHEADまたはインデックスが変化しました。", "")
	}
	s.Index = filtered
	return s, nil
}

func (r *repository) stableSnapshot() (snapshot, error) {
	first, err := r.collect()
	if err != nil {
		return first, err
	}
	second, err := r.collect()
	if err != nil {
		return second, err
	}
	if !reflect.DeepEqual(first, second) {
		return second, fail("snapshot_changed", "記録中に対象が変化しました。変更が止まってから再実行してください。", "")
	}
	return second, nil
}

func describeError(err error) Issue {
	var f *failure
	if errors.As(err, &f) {
		return f.issue
	}
	return Issue{Code: "record_failed", Message: fmt.Sprintf("開始記録を作成できません: %v", err)}
}
