package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lckrugel/tower-of-doom/internal/game/engine"
)

func TestRunArgumentExitCodesAndDiagnostics(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want int
		msg  string
	}{
		{[]string{"unexpected"}, 2, `unexpected argument "unexpected"`},
		{[]string{"--mods="}, 2, "--mods needs a directory"},
		{[]string{"--mods", "/missing"}, 2, "mods directory"},
		{[]string{"--bad"}, 2, "usage: tower-of-doom"},
	} {
		var out, stderr bytes.Buffer
		if got := run(tt.args, &out, &stderr, func(string) string { return "" }, func(*engine.Engine) error { return nil }); got != tt.want {
			t.Errorf("run(%v) = %d, want %d", tt.args, got, tt.want)
		}
		if !strings.Contains(stderr.String(), tt.msg) {
			t.Errorf("run(%v) stderr %q lacks %q", tt.args, stderr.String(), tt.msg)
		}
	}
}

func TestRunModPrecedenceAndPathsDiagnostics(t *testing.T) {
	root := t.TempDir()
	flagDir, envDir := filepath.Join(root, "flag"), filepath.Join(root, "env")
	for _, dir := range []string{flagDir, envDir} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range assetFiles {
			data, err := os.ReadFile(filepath.Join("..", "..", "internal", "game", "assets", name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	getenv := func(key string) string {
		if key == "TOD_MODS" {
			return envDir
		}
		return ""
	}
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--paths"}, envDir},
		{[]string{"--paths", "--mods", flagDir}, flagDir},
	} {
		var out, stderr bytes.Buffer
		if code := run(tt.args, &out, &stderr, getenv, func(*engine.Engine) error { t.Fatal("paths must not start TUI"); return nil }); code != 0 {
			t.Fatalf("run(%v) = %d; stderr=%s", tt.args, code, stderr.String())
		}
		if !strings.Contains(out.String(), "items: external ("+tt.want+")") {
			t.Errorf("provenance did not honor precedence %q: %s", tt.want, out.String())
		}
	}
}

func TestRunPropagatesTUIFailure(t *testing.T) {
	wantErr := errors.New("terminal unavailable")
	var out, stderr bytes.Buffer
	home := t.TempDir()
	getenv := func(key string) string {
		if key == "HOME" {
			return home
		}
		return ""
	}
	code := run(nil, &out, &stderr, getenv, func(*engine.Engine) error { return wantErr })
	if code != 1 || !strings.Contains(stderr.String(), wantErr.Error()) {
		t.Fatalf("run code=%d stderr=%q, want 1 and TUI diagnostic", code, stderr.String())
	}
}
