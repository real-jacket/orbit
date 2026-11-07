package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogMeasuresDenseCacheWithoutUnsafeSelection(t *testing.T) {
	home := fixtureHome(t)
	bucket := fixtureDir(t, home, ".cache/uv/archive-v0/package", 4096)
	for index := range 25001 {
		if err := os.WriteFile(filepath.Join(bucket, fmt.Sprintf("entry-%05d", index)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	scanner := &Scanner{home: home, processProbe: func() (string, error) { return "/usr/bin/idle\n", nil }}
	for _, expand := range []bool{false, true} {
		t.Run(fmt.Sprintf("expanded-%v", expand), func(t *testing.T) {
			row := scanner.catalog(CatalogSpec{ID: "dense", Name: "dense cache", Roots: []string{"~/.cache/uv"}, Tier: "review", ExpandChildren: expand}, &catalogClaims{}, nil)
			if row == nil || row["partial"] != false || row["tier"] != "review" {
				t.Fatalf("a measurable dense cache must not disappear or remain incomplete: %v", row)
			}
			if row["checked"] != false || row["subpaths"].([]map[string]any)[0]["checked"] != false {
				t.Fatal("better measurement must not authorize automatic removal")
			}
		})
	}
	if size := scanner.fastDirectorySize(bucket, 16); !size.partial || size.files != 16 {
		t.Fatalf("an explicit file budget must still stop traversal: %+v", size)
	}
}

func TestWholeWhitelistRuleTakesPrecedenceOverChildRule(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Library", "Caches")
	policy := Whitelist{Patterns: []string{filepath.Join(root, "protected-child"), root}}
	protection, protected := policy.Protection(root)
	if !protected || protection.Descendant || protection.Pattern != root {
		t.Fatalf("a whole-root rule must not be hidden by an earlier child: %+v", protection)
	}
	policy.Patterns = policy.Patterns[:1]
	if protection, protected := policy.Protection(root); !protected || !protection.Descendant {
		t.Fatal("a child-only rule must still allow inspection of other children")
	}
}

func TestExcludedCleanRootsStayOutsideCandidatesAndKeepPolicy(t *testing.T) {
	home := fixtureHome(t)
	for _, path := range []string{"Library/Caches/Google", "Library/Logs/App", ".npm/_cacache", "Library/Developer/Xcode/DerivedData/Project"} {
		fixtureDir(t, home, path, 4096)
	}
	writeWhitelist(t, home, "~/Library/Caches/Google/child", "~/Library/Caches & ~/Library/Logs", "~/.npm/_cacache", "~/Library/Developer/Xcode/DerivedData")
	before, err := os.ReadFile(WhitelistPath(home))
	if err != nil {
		t.Fatal(err)
	}
	scanner := &Scanner{home: home}
	rows := scanner.cleanExcludedRoots()
	expected := map[string]bool{"~/Library/Caches": true, "~/Library/Logs": true, "~/.npm/_cacache": true, "~/Library/Developer/Xcode/DerivedData": true}
	if len(rows) != len(expected) {
		t.Fatalf("excluded roots should not double count nested catalog roots: %v", rows)
	}
	for _, row := range rows {
		if !expected[row["path"].(string)] || !strings.Contains(row["reason"].(string), "白名单排除") || row["sizeGb"] != nil {
			t.Fatalf("excluded roots must identify protection without pretending to measure: %v", row)
		}
		delete(expected, row["path"].(string))
	}
	if len(scanner.scanCleanGroups()) != 0 {
		t.Fatal("excluded-root diagnostics must not restore protected candidates")
	}
	data, err := json.Marshal(FullScanPayload{CleanExcludedRoots: rows})
	if err != nil || !strings.Contains(string(data), `"cleanExcludedRoots"`) {
		t.Fatalf("the authoritative scan must carry excluded-root evidence: %s, %v", data, err)
	}
	after, err := os.ReadFile(WhitelistPath(home))
	if err != nil || string(after) != string(before) {
		t.Fatal("describing excluded roots must not change the user's whitelist")
	}
}

func TestExcludedCleanRootsDoNotInventAbsentOrChildOnlyExclusions(t *testing.T) {
	home := fixtureHome(t)
	fixtureDir(t, home, "Library/Caches/Protected", 4096)
	fixtureDir(t, home, "Library/Caches/Review", 4096)
	writeWhitelist(t, home, "~/Library/Caches/Protected", "~/Library/Developer/Xcode/DerivedData")
	if rows := (&Scanner{home: home}).cleanExcludedRoots(); len(rows) != 0 {
		t.Fatalf("a missing root or protected child cannot label a whole cache root excluded: %v", rows)
	}
}
