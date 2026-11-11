package baseline

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// 指定は開始記録だけで検証する。現在の読取不能や種類変更は照合側で判定する。
func (r *repository) includeIgnoredPaths(names []string, previous []Entry) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	known := map[string]string{}
	for _, entry := range previous {
		if entry.Kind == "file" || entry.Kind == "symlink" {
			known[pathKey(entry.Path)] = entry.Path
		}
	}
	selected := map[string]bool{}
	for _, name := range names {
		if name == "" || !utf8.ValidString(name) || strings.ContainsAny(name, "\x00*?[]:") || !filepath.IsLocal(filepath.FromSlash(name)) {
			return nil, invalid("--include-ignored には開始記録にあるファイルのルート相対パスを指定してください。ワイルドカードやルート外は指定できません。", name)
		}
		name = filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
		if r.protected(name) {
			return nil, invalid("Gitの管理領域と状態保存先は --include-ignored に指定できません。", name)
		}
		canonical, ok := known[pathKey(name)]
		if !ok {
			return nil, invalid("--include-ignored は選択した開始記録にハッシュのあるファイルかリンクを1件ずつ指定してください。ディレクトリ全体や未記録の対象は指定できません。", name)
		}
		selected[canonical] = true
	}
	paths := make([]string, 0, len(selected))
	for name := range selected {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return paths, nil
}
