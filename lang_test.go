package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var cjk = regexp.MustCompile(`[\x{3400}-\x{9fff}]`)

func TestSystemLanguage(t *testing.T) {
	for _, tc := range []struct {
		preferred []string
		want      string
	}{
		{nil, langZH},
		{[]string{"zh-Hans-CN", "en"}, langZH},
		{[]string{"zh-TW"}, langZH},
		{[]string{"en-US", "zh-Hans"}, langEN},
		{[]string{"ja-JP"}, langEN},
	} {
		if got := systemLanguage(tc.preferred); got != tc.want {
			t.Errorf("systemLanguage(%v) = %q, want %q", tc.preferred, got, tc.want)
		}
	}
}

func TestLPicksTheCurrentLanguage(t *testing.T) {
	defer currentLang.Store(langZH)
	currentLang.Store(langEN)
	if got := L("退出", "Quit"); got != "Quit" {
		t.Errorf("English: got %q", got)
	}
	currentLang.Store(langZH)
	if got := L("退出", "Quit"); got != "退出" {
		t.Errorf("Chinese: got %q", got)
	}
}

func TestTelemetryToolTipReadsInBothLanguages(t *testing.T) {
	defer currentLang.Store(langZH)
	cpu := 12.34
	currentLang.Store(langZH)
	if got, want := telemetryToolTip(&cpu, "warn", 80.25), "orbit · 最近采样\nCPU 12.3% · 内存压力偏高\n启动磁盘可用 80.2 GB"; got != want {
		t.Errorf("zh tooltip = %q, want %q", got, want)
	}
	currentLang.Store(langEN)
	got := telemetryToolTip(nil, "", 80.25)
	if cjk.MatchString(got) || !strings.Contains(got, "CPU Unknown") || !strings.Contains(got, "Memory pressure: Unknown") {
		t.Errorf("en tooltip = %q", got)
	}
}

// Both preferences live in one file, and saving one must not drop the other.
func TestPreferencesKeepBothChoices(t *testing.T) {
	p := preferences{Theme: themeDark, Lang: langEN}
	p.Lang = ""
	if p == (preferences{}) || p.Theme != themeDark {
		t.Fatalf("clearing the language lost the theme: %+v", p)
	}
	p.Theme = ""
	if p != (preferences{}) {
		t.Fatalf("both cleared should be the empty value that removes the file: %+v", p)
	}
}

// The English text of every L(zh, en) call must be English: a Chinese character
// there is a missed translation.
func TestEnglishSideOfLHasNoChinese(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	calls := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); !ok || id.Name != "L" {
				return true
			}
			zh, ok1 := call.Args[0].(*ast.BasicLit)
			en, ok2 := call.Args[1].(*ast.BasicLit)
			if !ok1 || !ok2 {
				return true
			}
			calls++
			zhText, _ := strconv.Unquote(zh.Value)
			enText, _ := strconv.Unquote(en.Value)
			if !cjk.MatchString(zhText) {
				t.Errorf("%s: L(%s, ...) has no Chinese source", name, zh.Value)
			}
			if cjk.MatchString(enText) || enText == "" {
				t.Errorf("%s: L(..., %s) is not English", name, en.Value)
			}
			return true
		})
	}
	if calls < 10 {
		t.Fatalf("found only %d L() calls; the scan no longer matches", calls)
	}
}
