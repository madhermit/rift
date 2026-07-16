package git

import (
	"strings"
	"testing"
)

// TestShellReadSurfacesStderr checks that a failed shelled read carries git's
// stderr (via gitErr) instead of a bare "exit status 128".
func TestShellReadSurfacesStderr(t *testing.T) {
	repo := setupTestRepo(t)

	_, err := repo.DiffAgainstRef("definitely-not-a-ref")
	if err == nil {
		t.Fatal("expected an error for a bad ref, got nil")
	}
	if !strings.Contains(err.Error(), "definitely-not-a-ref") {
		t.Errorf("error %q does not surface git's stderr (missing the bad ref)", err)
	}
}

func TestDiffTargets(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantBase   string
		wantTarget string
		wantErr    bool
	}{
		{"zero args is a working-tree diff (no base)", []string{}, "", "", false},
		{"one arg is base", []string{"main"}, "main", "", false},
		{"two args are base and target", []string{"abc", "def"}, "abc", "def", false},
		{"three args is error", []string{"a", "b", "c"}, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, target, err := DiffTargets(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DiffTargets() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
			if target != tt.wantTarget {
				t.Errorf("target = %q, want %q", target, tt.wantTarget)
			}
		})
	}
}

func TestChangedFiles_Unstaged(t *testing.T) {
	repo := setupTestRepo(t)

	writeFile(t, repo.root, "README.md", "# modified\n")

	files, err := repo.ChangedFiles(false)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 changed file, got %d: %v", len(files), files)
	}
	if files[0].Path != "README.md" || files[0].Status != "Modified" {
		t.Errorf("got {%q, %q}, want {\"README.md\", \"Modified\"}", files[0].Path, files[0].Status)
	}
}

func TestChangedFiles_Staged(t *testing.T) {
	repo := setupTestRepo(t)

	writeFile(t, repo.root, "new.txt", "new content\n")
	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	if _, err := wt.Add("new.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	files, err := repo.ChangedFiles(true)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 staged file, got %d: %v", len(files), files)
	}
	if files[0].Path != "new.txt" || files[0].Status != "Added" {
		t.Errorf("got {%q, %q}, want {\"new.txt\", \"Added\"}", files[0].Path, files[0].Status)
	}
}

// TestChangedFiles_StagedRename pins that a staged move is one Renamed record,
// not a delete + an add — matching what `git status` reports. The stage screen
// keeps the split view via StatusFiles; this is the diff listing's contract.
func TestChangedFiles_StagedRename(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	content := "package svc\n\nfunc Alpha() int { return 1 }\nfunc Beta() int { return 2 }\n"
	writeFile(t, repo.root, "old/svc.go", content)
	if _, err := wt.Add("old/svc.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	testCommit(t, wt, "add old/svc.go")

	// Stage a pure move: identical content, so git pairs the paths at 100%.
	writeFile(t, repo.root, "new/svc.go", content)
	if _, err := wt.Remove("old/svc.go"); err != nil {
		t.Fatalf("git rm: %v", err)
	}
	if _, err := wt.Add("new/svc.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	files, err := repo.ChangedFiles(true)
	if err != nil {
		t.Fatalf("ChangedFiles(true) error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 rename record, got %d: %+v", len(files), files)
	}
	if got := files[0]; got != (ChangedFile{Path: "new/svc.go", OldPath: "old/svc.go", Status: "Renamed"}) {
		t.Errorf("staged rename = %+v, want new/svc.go ← old/svc.go Renamed", got)
	}
}

func TestCompactRenamePath(t *testing.T) {
	tests := []struct {
		name       string
		oldP, newP string
		want       string
	}{
		{"inserted dir segment", "web/app/pages/our-prenup/x.vue", "web/app/pages/prenup/our-prenup/x.vue", "web/app/pages/{ → prenup}/our-prenup/x.vue"},
		{"changed middle dir", "a/b/one/x.go", "a/b/two/x.go", "a/b/{one → two}/x.go"},
		{"same dir renamed file", "pkg/old.go", "pkg/new.go", "pkg/{old.go → new.go}"},
		{"nothing in common", "a/b.go", "c/d.go", "{a/b.go → c/d.go}"},
		{"top-level file rename", "old.go", "new.go", "{old.go → new.go}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := ChangedFile{Path: tc.newP, OldPath: tc.oldP, Status: "Renamed"}
			if got := f.DisplayPath(); got != tc.want {
				t.Errorf("DisplayPath() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDiffBetweenCommits(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	head, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("get head: %v", err)
	}
	baseHash := head.Hash().String()

	writeFile(t, repo.root, "added.go", "package main\n")
	writeFile(t, repo.root, "README.md", "# changed\n") // was "# test repo\n"
	if _, err := wt.Add("added.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	targetHash := testCommit(t, wt, "second commit")

	files, err := repo.DiffBetweenCommits(baseHash, targetHash.String())
	if err != nil {
		t.Fatalf("DiffBetweenCommits() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 changed files, got %d: %v", len(files), files)
	}

	byPath := map[string]ChangedFile{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	if got := byPath["added.go"]; got.Status != "Added" || got.Added != 1 || got.Deleted != 0 {
		t.Errorf("added.go = %+v, want Added +1 -0", got)
	}
	// README.md: one line replaced by another = +1/-1.
	if got := byPath["README.md"]; got.Status != "Modified" || got.Added != 1 || got.Deleted != 1 {
		t.Errorf("README.md = %+v, want Modified +1 -1", got)
	}
}

// TestDiffBetweenCommits_EmptyTree covers the EmptyTree base: a root commit (via
// diff-tree --root) and a non-root target (which must diff against the empty tree,
// not the parent — so every file in the target shows as a full addition).
func TestDiffBetweenCommits_EmptyTree(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	rootHash, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("get head: %v", err)
	}

	writeFile(t, repo.root, "second.go", "package main\n")
	if _, err := wt.Add("second.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	secondHash := testCommit(t, wt, "second commit")

	t.Run("root commit lists its files as additions", func(t *testing.T) {
		files, err := repo.DiffBetweenCommits(EmptyTree, rootHash.Hash().String())
		if err != nil {
			t.Fatalf("DiffBetweenCommits() error = %v", err)
		}
		if len(files) != 1 || files[0].Path != "README.md" || files[0].Status != "Added" {
			t.Fatalf("root diff = %+v, want [README.md Added]", files)
		}
	})

	t.Run("non-root target diffs against empty tree, not parent", func(t *testing.T) {
		files, err := repo.DiffBetweenCommits(EmptyTree, secondHash.String())
		if err != nil {
			t.Fatalf("DiffBetweenCommits() error = %v", err)
		}
		byPath := map[string]string{}
		for _, f := range files {
			byPath[f.Path] = f.Status
		}
		// Against the parent it would be just second.go; against the empty tree
		// the whole tree (README.md + second.go) is an addition.
		if byPath["README.md"] != "Added" || byPath["second.go"] != "Added" {
			t.Errorf("empty-tree diff = %+v, want README.md and second.go both Added", byPath)
		}
	})
}

// TestDiffAgainstRef covers the single-ref scope: the working tree compared to a
// ref, surfacing both committed and uncommitted changes since it, with stats.
// Non-ASCII and spaced paths must come through verbatim (the -z path).
func TestDiffAgainstRef(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	base, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("get head: %v", err)
	}

	// A committed change since base (invisible to a worktree-vs-HEAD listing).
	writeFile(t, repo.root, "héllo.txt", "committed\n")
	if _, err := wt.Add("héllo.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	testCommit(t, wt, "add héllo")
	// An uncommitted change on top.
	writeFile(t, repo.root, "sp ace.txt", "unstaged\n")
	if _, err := wt.Add("sp ace.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	files, err := repo.DiffAgainstRef(base.Hash().String())
	if err != nil {
		t.Fatalf("DiffAgainstRef() error = %v", err)
	}
	byPath := map[string]ChangedFile{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	if got := byPath["héllo.txt"]; got.Status != "Added" || got.Added != 1 {
		t.Errorf("héllo.txt = %+v, want Added +1 (committed change since ref)", got)
	}
	if got := byPath["sp ace.txt"]; got.Status != "Added" || got.Added != 1 {
		t.Errorf("sp ace.txt = %+v, want Added +1 (uncommitted change)", got)
	}
}

func TestParseNameStatusZ(t *testing.T) {
	// STATUS\0PATH pairs, non-ASCII and spaced paths verbatim (no C-quoting).
	out := "M\x00internal/diff/diff.go\x00A\x00héllo.txt\x00D\x00sp ace.txt\x00"
	got := parseNameStatusZ(out)
	want := []ChangedFile{
		{Path: "internal/diff/diff.go", Status: "Modified"},
		{Path: "héllo.txt", Status: "Added"},
		{Path: "sp ace.txt", Status: "Deleted"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseNumstatZ(t *testing.T) {
	// added\tdeleted\tpath records; binary reports "-" (→ 0); path verbatim.
	out := "3\t1\tinternal/diff/diff.go\x00-\t-\timage.png\x0010\t0\tsp ace.txt\x00"
	got := parseNumstatZ(out)
	want := map[string][2]int{
		"internal/diff/diff.go": {3, 1},
		"image.png":             {0, 0}, // binary
		"sp ace.txt":            {10, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for path, w := range want {
		if got[path] != w {
			t.Errorf("%q = %v, want %v", path, got[path], w)
		}
	}
}

func TestParseNameStatusZ_Rename(t *testing.T) {
	// A rename record carries three fields: R<score>\0OLD\0NEW. Following
	// records parse normally.
	out := "R087\x00old dir/a.go\x00new dir/b.go\x00M\x00c.go\x00"
	got := parseNameStatusZ(out)
	want := []ChangedFile{
		{Path: "new dir/b.go", OldPath: "old dir/a.go", Status: "Renamed"},
		{Path: "c.go", Status: "Modified"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseNumstatZ_Rename(t *testing.T) {
	// A rename record leaves the inline path empty and appends OLD and NEW as
	// separate NUL fields; stats key on the new path.
	out := "3\t1\t\x00old/a.go\x00new/b.go\x005\t0\tc.go\x00"
	got := parseNumstatZ(out)
	want := map[string][2]int{
		"new/b.go": {3, 1},
		"c.go":     {5, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for path, w := range want {
		if got[path] != w {
			t.Errorf("%q = %v, want %v", path, got[path], w)
		}
	}
}

// TestDiffBetweenCommits_Rename covers rename detection end to end: a renamed
// file with a small edit reports one record with both paths and the edit's
// stats, not a full delete + add.
func TestDiffBetweenCommits_Rename(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}

	content := "package main\n\nfunc a() {}\nfunc b() {}\nfunc c() {}\nfunc d() {}\nfunc e() {}\nfunc f() {}\nfunc g() {}\nfunc h() {}\n"
	writeFile(t, repo.root, "old_name.go", content)
	if _, err := wt.Add("old_name.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	base := testCommit(t, wt, "add old_name")

	// Rename with a one-line tweak: similar enough for -M to pair the paths.
	writeFile(t, repo.root, "new_name.go", strings.Replace(content, "func h() {}", "func i() {}", 1))
	if _, err := wt.Remove("old_name.go"); err != nil {
		t.Fatalf("git rm: %v", err)
	}
	if _, err := wt.Add("new_name.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	target := testCommit(t, wt, "rename old_name to new_name")

	files, err := repo.DiffBetweenCommits(base.String(), target.String())
	if err != nil {
		t.Fatalf("DiffBetweenCommits() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 rename record, got %d: %+v", len(files), files)
	}
	got := files[0]
	want := ChangedFile{Path: "new_name.go", OldPath: "old_name.go", Status: "Renamed", Added: 1, Deleted: 1}
	if got != want {
		t.Errorf("rename = %+v, want %+v", got, want)
	}
}

// TestListChanged_StagedWinsOverBase pins the precedence contract: staged
// beats base, matching the per-file preview (buildGitDiffArgs), so the TUI's
// `s` toggle in a base-ref scope lists the same side the previews render.
func TestListChanged_StagedWinsOverBase(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	head, err := repo.repo.Head()
	if err != nil {
		t.Fatalf("get head: %v", err)
	}
	base := head.Hash().String()

	// One committed-since-base change and one staged change.
	writeFile(t, repo.root, "committed.go", "package main\n")
	if _, err := wt.Add("committed.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	testCommit(t, wt, "committed change")
	writeFile(t, repo.root, "staged.go", "package main\n")
	if _, err := wt.Add("staged.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	files, err := repo.ListChanged(true, base, "")
	if err != nil {
		t.Fatalf("ListChanged() error = %v", err)
	}
	if len(files) != 1 || files[0].Path != "staged.go" {
		t.Errorf("staged+base should list the staged side only, got %+v", files)
	}
}

func TestStagedBlobHashes(t *testing.T) {
	repo := setupTestRepo(t)

	wt, err := repo.repo.Worktree()
	if err != nil {
		t.Fatalf("get worktree: %v", err)
	}
	writeFile(t, repo.root, "f.go", "package main\n")
	if _, err := wt.Add("f.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	staged := repo.StagedBlobHashes([]string{"f.go"})
	if staged["f.go"] == "" {
		t.Fatalf("expected an index hash for f.go, got %v", staged)
	}

	// Editing the worktree without re-adding must not change the index hash —
	// that's the property the staged watch fingerprint relies on.
	writeFile(t, repo.root, "f.go", "package main\n\nfunc edited() {}\n")
	after := repo.StagedBlobHashes([]string{"f.go"})
	if after["f.go"] != staged["f.go"] {
		t.Errorf("index hash changed on a worktree-only edit: %q -> %q", staged["f.go"], after["f.go"])
	}
	if wtHash := repo.BlobHashes([]string{"f.go"})["f.go"]; wtHash == after["f.go"] {
		t.Errorf("worktree and index hashes should differ after the edit, both %q", wtHash)
	}

	// Re-staging picks up the new content.
	if _, err := wt.Add("f.go"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	restaged := repo.StagedBlobHashes([]string{"f.go"})
	if restaged["f.go"] == staged["f.go"] {
		t.Errorf("index hash should change after re-staging, still %q", staged["f.go"])
	}
}
