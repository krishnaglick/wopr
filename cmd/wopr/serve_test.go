package main

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alexrudloff/wopr/internal/codingagent"
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

	// A sibling sharing the entry's prefix must not be treated as inside it.
	baseDir := filepath.Join(base, "base")
	sibling := filepath.Join(base, "baseX")
	for _, d := range []string{baseDir, sibling} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := resolveServeRoot(sibling, []string{baseDir}); err == nil {
		t.Fatal("prefix sibling succeeded, want error")
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

	// The default root is os.Getwd(). A cwd that reaches the outside through a
	// symlink inside an allowed entry must be rejected: the root is resolved
	// before the containment check, like the entries.
	allowed := t.TempDir()
	outside := t.TempDir()
	escapeLink := filepath.Join(allowed, "escape")
	testenv.Symlink(t, outside, escapeLink)
	t.Chdir(escapeLink)
	if _, err = resolveServeRoot("", []string{allowed}); err == nil {
		t.Fatal("symlinked cwd escaping its allowed entry succeeded, want error")
	}
}

func TestGlobalServeAllowedRootsFailsClosedOnMalformedGlobalSettings(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		agentDir := t.TempDir()
		sm := codingagent.NewSettingsManagerWithProjectTrust(t.TempDir(), agentDir, false)
		roots, err := globalServeAllowedRoots(sm)
		if roots != nil || err != nil {
			t.Fatalf("globalServeAllowedRoots() = %v, %v; want nil, nil", roots, err)
		}
	})

	t.Run("parsed-empty", func(t *testing.T) {
		agentDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), []byte(`{"server":{"port":7777}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		sm := codingagent.NewSettingsManagerWithProjectTrust(t.TempDir(), agentDir, false)
		roots, err := globalServeAllowedRoots(sm)
		if roots != nil || err != nil {
			t.Fatalf("globalServeAllowedRoots() = %v, %v; want nil, nil", roots, err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		agentDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		sm := codingagent.NewSettingsManagerWithProjectTrust(t.TempDir(), agentDir, false)
		if _, err := globalServeAllowedRoots(sm); err == nil {
			t.Fatal("globalServeAllowedRoots() error = nil for malformed settings, want error")
		}
	})
}

func TestRunServeCommandRefusesOnMalformedGlobalSettings(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	tmp := t.TempDir()
	t.Setenv("WOPR_HOME", tmp)
	t.Setenv(codingagent.ENV_AGENT_DIR, "")
	agentDir := filepath.Join(tmp, "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })
	code := runServeCommand([]string{"serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port)})
	os.Stderr = old
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	stderr, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if code != 1 {
		t.Fatalf("runServeCommand() = %d, want 1; stderr: %s", code, stderr)
	}
	if !strings.Contains(string(stderr), "global settings:") {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, "global settings:")
	}
}
