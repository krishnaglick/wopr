package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alexrudloff/wopr/internal/testenv"
)

func TestResolveServeRoot(t *testing.T) {
	base := t.TempDir()
	sub := filepath.Join(base, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	wantSub, err := filepath.EvalSymlinks(sub)
	if err != nil {
		t.Fatal(err)
	}

	// Root inside an entry.
	if got, err := resolveServeRoot(sub, []string{base}); err != nil || got != wantSub {
		t.Fatalf("inside = %q, %v; want %q", got, err, wantSub)
	}

	// Root outside every entry.
	if _, err := resolveServeRoot(sub, []string{t.TempDir()}); err == nil {
		t.Fatal("root outside every entry succeeded, want error")
	}

	// Empty list is unrestricted.
	if got, err := resolveServeRoot(sub, nil); err != nil || got != wantSub {
		t.Fatalf("empty list = %q, %v; want %q", got, err, wantSub)
	}

	// A symlinked entry is resolved before containment.
	real := t.TempDir()
	wantReal, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	testenv.Symlink(t, real, link)
	if got, err := resolveServeRoot(real, []string{link}); err != nil || got != wantReal {
		t.Fatalf("symlinked entry = %q, %v; want %q", got, err, wantReal)
	}

	// An entry that is not a directory fails closed.
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveServeRoot(sub, []string{file}); err == nil {
		t.Fatal("file entry succeeded, want error")
	}
}
