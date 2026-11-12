package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestRunConsumablesOnlyMods(t *testing.T) {
	for _, source := range []string{"flag", "env", "default"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			env := map[string]string{"HOME": root}
			dir := filepath.Join(root, "mod")
			var args []string
			switch source {
			case "flag":
				args = []string{"--mods", dir}
			case "env":
				env["TOD_MODS"] = dir
			case "default":
				switch runtime.GOOS {
				case "windows":
					env["APPDATA"] = filepath.Join(root, "appdata")
					dir = filepath.Join(env["APPDATA"], "tower-of-doom", "mods")
				case "darwin":
					dir = filepath.Join(root, "Library", "Application Support", "tower-of-doom", "mods")
				default:
					env["XDG_DATA_HOME"] = filepath.Join(root, "data")
					dir = filepath.Join(env["XDG_DATA_HOME"], "tower-of-doom", "mods")
				}
			}
			getenv := func(key string) string { return env[key] }
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, "consumables.json")
			write := func(data string) {
				t.Helper()
				if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// A literal, single-file fixture must not inherit the CLI detection list.
			write(`[{"id":"mod_tonic","name":"Mod Tonic","heal_percent":37,"price":13}]`)
			var out, stderr bytes.Buffer
			if code := run(append([]string{"--paths"}, args...), &out, &stderr, getenv, func(*engine.Engine) error {
				t.Fatal("--paths must not start TUI")
				return nil
			}); code != 0 {
				t.Fatalf("--paths code=%d stderr=%s", code, stderr.String())
			}
			if !strings.Contains(out.String(), "consumables: external ("+dir+")") {
				t.Errorf("missing external consumables provenance: %s", out.String())
			}
			for _, name := range []string{"monsters", "items", "moves", "statuses", "sections"} {
				if !strings.Contains(out.String(), name+": embedded") {
					t.Errorf("missing embedded %s provenance: %s", name, out.String())
				}
			}
			out.Reset()
			stderr.Reset()
			started := false
			if code := run(args, &out, &stderr, getenv, func(e *engine.Engine) error {
				started = true
				asset, ok := e.ConsumableByID("mod_tonic")
				if !ok || asset.Name != "Mod Tonic" || asset.HealPercent != 37 || asset.Price != 13 {
					t.Errorf("startup consumable = %+v, found=%v", asset, ok)
				}
				return nil
			}); code != 0 || !started {
				t.Errorf("startup code=%d started=%v stderr=%s", code, started, stderr.String())
			}
			write(`[{"id":"mod_tonic","name":"Mod Tonic","heal_percent":0,"price":13}]`)
			for _, mode := range [][]string{args, append([]string{"--paths"}, args...)} {
				stderr.Reset()
				if code := run(mode, &out, &stderr, getenv, func(*engine.Engine) error {
					t.Fatal("invalid content must not start TUI")
					return nil
				}); code != 1 || !strings.Contains(stderr.String(), "invalid game content:") {
					t.Errorf("invalid consumable: code=%d stderr=%s", code, stderr.String())
				}
			}
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			out.Reset()
			stderr.Reset()
			code := run(append([]string{"--paths"}, args...), &out, &stderr, getenv, func(*engine.Engine) error {
				t.Fatal("--paths must not start TUI")
				return nil
			})
			if source == "flag" {
				if code != 2 || !strings.Contains(stderr.String(), "holds no asset files") {
					t.Errorf("empty explicit mod: code=%d stderr=%s", code, stderr.String())
				}
			} else if code != 0 || !strings.Contains(out.String(), "assets: embedded") {
				t.Errorf("empty discovered mod: code=%d out=%s stderr=%s", code, out.String(), stderr.String())
			}
		})
	}
}
