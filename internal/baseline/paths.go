package baseline

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"
)

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func beneath(name, parent string) bool {
	if parent == "." {
		return true
	}
	return samePath(name, parent) || (len(name) > len(parent) && name[len(parent)] == '/' && samePath(name[:len(parent)], parent))
}

func (r *repository) protected(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if samePath(part, ".git") {
			return true
		}
	}
	for _, prefix := range r.excluded {
		if beneath(name, prefix) {
			return true
		}
	}
	return false
}

func (r *repository) normalizeAllow(names []string) ([]Allowed, error) {
	allowed := []Allowed{}
	for _, name := range names {
		if name == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00*?[]") || !filepath.IsLocal(filepath.FromSlash(name)) || strings.Contains(name, ":") {
			return nil, invalid("--allow にはワイルドカードを含まないルート相対パスを指定してください。", name)
		}
		name = filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
		if r.protected(name) {
			return nil, invalid("Gitの管理領域と状態保存先は許容対象に指定できません。", name)
		}
		kind := "future"
		prefix := "."
		parts := strings.Split(name, "/")
		for i, part := range parts {
			candidate := filepath.ToSlash(filepath.Join(prefix, part))
			info, err := r.fs.Lstat(candidate)
			if errors.Is(err, fs.ErrNotExist) {
				prefix = candidate
				continue
			}
			if err != nil {
				return nil, invalid("許容対象のパスを確認できません。", name)
			}
			// 実在するパスの実名を取得する。リンク先やハードリンクの別名へ置き換えない。
			actual, err := actualName(filepath.Join(r.root, filepath.FromSlash(candidate)))
			if err != nil {
				return nil, invalid("許容対象の実名を確認できません。", name)
			}
			if part != "." {
				candidate = filepath.ToSlash(filepath.Join(prefix, actual))
			}
			if r.protected(candidate) {
				return nil, invalid("Gitの管理領域と状態保存先は許容対象に指定できません。", name)
			}
			last := i == len(parts)-1
			if !last && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
				return nil, invalid("リンクやファイルの配下は許容対象に指定できません。", name)
			}
			if last {
				kind = "file"
				if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
					kind = "directory"
				}
			}
			prefix = candidate
		}
		duplicate := false
		for _, previous := range allowed {
			if previous.Path == prefix || (kind == "future" && previous.Kind == "future" && samePath(previous.Path, prefix)) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			allowed = append(allowed, Allowed{prefix, kind})
		}
	}
	sort.Slice(allowed, func(i, j int) bool { return allowed[i].Path < allowed[j].Path })
	return allowed, nil
}

// 親リンクを開かずに、各成分が通常のディレクトリであることを確認する。
func (r *repository) safeParents(name string) error {
	parent := filepath.ToSlash(filepath.Dir(name))
	if parent == "." {
		return nil
	}
	prefix := ""
	for _, part := range strings.Split(parent, "/") {
		prefix = filepath.ToSlash(filepath.Join(prefix, part))
		info, err := r.fs.Lstat(prefix)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fail("unreadable_path", "親ディレクトリを確認できません。", prefix)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fail("unsafe_parent", "親がリンクまたは通常のディレクトリ以外のため、配下を読み取りません。", prefix)
		}
	}
	return nil
}
