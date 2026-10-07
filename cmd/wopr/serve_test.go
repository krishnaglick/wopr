package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
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

	t.Run("wrong-typed-roots", func(t *testing.T) {
		agentDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), []byte(`{"server":{"allowedRoots":"x"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		sm := codingagent.NewSettingsManagerWithProjectTrust(t.TempDir(), agentDir, false)
		if _, err := globalServeAllowedRoots(sm); err == nil {
			t.Fatal("globalServeAllowedRoots() error = nil for wrong-typed allowedRoots, want error")
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

func TestRunServeCommandRefusesOnUndecodableServerGuardrail(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(agentDir, "settings.json"), []byte(`{"server":{"allowedRoots":"x"}}`), 0o644); err != nil {
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
	if !strings.Contains(string(stderr), "allowedRoots") && !strings.Contains(string(stderr), "server") {
		t.Fatalf("stderr = %q, want it to name the undecodable server guardrail", stderr)
	}
}

func TestGlobalServeAllowedRootsReadsGlobalLayerOnly(t *testing.T) {
	globalDir := t.TempDir()
	projDir := t.TempDir()
	agentDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(agentDir, "settings.json"),
		[]byte(fmt.Sprintf(`{"server":{"allowedRoots":[%q]}}`, globalDir)), 0o644); err != nil {
		t.Fatal(err)
	}
	projSettingsDir := filepath.Join(projDir, codingagent.CONFIG_DIR_NAME)
	if err := os.MkdirAll(projSettingsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projSettingsDir, "settings.json"),
		[]byte(fmt.Sprintf(`{"server":{"allowedRoots":[%q]}}`, projDir)), 0o644); err != nil {
		t.Fatal(err)
	}

	// The project layer is trusted here, so its broader list reaches the merged
	// view. The global-layer read must ignore it: a project must not widen the
	// serve allowlist.
	t.Run("trusted", func(t *testing.T) {
		sm := codingagent.NewSettingsManagerWithProjectTrust(projDir, agentDir, true)

		merged, err := sm.Get().GetServerAllowedRoots()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(merged, projDir) {
			t.Fatalf("merged GetServerAllowedRoots() = %v; want it to contain project root %q", merged, projDir)
		}

		got, err := globalServeAllowedRoots(sm)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{globalDir}) {
			t.Fatalf("globalServeAllowedRoots() = %v, want only [%q]", got, globalDir)
		}
	})

	// Untrusted mirrors runServeCommand's wiring: the project layer is not read,
	// so both the merged and global views are the global list.
	t.Run("untrusted", func(t *testing.T) {
		sm := codingagent.NewSettingsManagerWithProjectTrust(projDir, agentDir, false)

		merged, err := sm.Get().GetServerAllowedRoots()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(merged, []string{globalDir}) {
			t.Fatalf("merged GetServerAllowedRoots() = %v; want project ignored, [%q]", merged, globalDir)
		}

		got, err := globalServeAllowedRoots(sm)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{globalDir}) {
			t.Fatalf("globalServeAllowedRoots() = %v, want [%q]", got, globalDir)
		}
	})
}

func TestRunServeCommandRefusesRootOutsideGlobalAllowlist(t *testing.T) {
	// Hold a free port so the red-proof path (allowlist check dropped) fails
	// fast at serve.New's net.Listen instead of blocking.
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
	globalDir := t.TempDir()
	projDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(agentDir, "settings.json"),
		[]byte(fmt.Sprintf(`{"server":{"allowedRoots":[%q]}}`, globalDir)), 0o644); err != nil {
		t.Fatal(err)
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = old })
	code := runServeCommand([]string{"serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port), "--dir", projDir})
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
	if !strings.Contains(string(stderr), "is outside every server.allowedRoots entry") {
		t.Fatalf("stderr = %q, want it to contain %q", stderr, "is outside every server.allowedRoots entry")
	}
}
