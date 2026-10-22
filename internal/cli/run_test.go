package cli_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"workscope/internal/cli"
	"workscope/internal/version"
)

func TestHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := cli.Run([]string{arg}, &stdout, &stderr); code != 0 {
				t.Fatalf("終了コード = %d、期待値 = 0", code)
			}
			for _, text := range []string{"使い方", "現在は未実装", "サブモジュール", "/.workscope/"} {
				if !strings.Contains(stdout.String(), text) {
					t.Errorf("ヘルプに %q がありません", text)
				}
			}
			if stderr.Len() != 0 {
				t.Errorf("標準エラーが空ではありません: %s", &stderr)
			}
		})
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"--version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("終了コード = %d、期待値 = 0", code)
	}
	if want := "workscope " + version.Current + "\n"; stdout.String() != want {
		t.Errorf("版表示 = %q、期待値 = %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Errorf("標準エラーが空ではありません: %s", &stderr)
	}
}

func TestUnsupportedArgumentsNeverReportSuccess(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "引数なし"},
		{name: "未知の引数", args: []string{"--unknown"}},
		{name: "余分な引数", args: []string{"--version", "extra"}},
		{name: "未実装の記録", args: []string{"begin", "--allow", "src"}},
		{name: "未実装のJSON照合", args: []string{"check", "--json"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := cli.Run(tc.args, &stdout, &stderr); code != 64 {
				t.Errorf("終了コード = %d、期待値 = 64", code)
			}
			if stdout.Len() != 0 || !strings.Contains(stderr.String(), "workscope --help") {
				t.Errorf("誤った引数の案内が不正です。標準出力 = %q、標準エラー = %q", &stdout, &stderr)
			}
		})
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) {
	return 0, errors.New("書き込み不能")
}

func TestOutputFailureNeverReportsSuccess(t *testing.T) {
	var stderr bytes.Buffer
	if code := cli.Run([]string{"--version"}, failedWriter{}, &stderr); code != 2 {
		t.Errorf("終了コード = %d、期待値 = 2", code)
	}
	if !strings.Contains(stderr.String(), "書き込みに失敗") {
		t.Errorf("出力失敗の案内がありません: %s", &stderr)
	}
}
