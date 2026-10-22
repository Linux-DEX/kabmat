package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingFileUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KABMAT_CONFIG", filepath.Join(dir, "config"))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Exists || cfg.Theme != "linux-dex" || cfg.Action("board", "c") != "new-card" || cfg.Action("board", "t") != "tags" {
		t.Fatalf("%+v", cfg)
	}
}

func TestParseAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	t.Setenv("KABMAT_CONFIG", path)
	body := "" +
		"theme catppuccin-mocha\n\n" +
		"[tags]\n# a comment\n#spike\n\n" +
		"[keys.board]\nnew-card x\n\n" +
		"[theme.dusk]\naccent #89B4FA\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Exists || cfg.Theme != "catppuccin-mocha" || len(cfg.Tags) != 1 || cfg.Tags[0] != "spike" {
		t.Fatalf("load %+v", cfg)
	}
	if cfg.Action("board", "x") != "new-card" || cfg.Action("board", "c") != "" || cfg.Action("board", "t") != "tags" {
		t.Fatalf("keys x=%s c=%s t=%s", cfg.Action("board", "x"), cfg.Action("board", "c"), cfg.Action("board", "t"))
	}
	if cfg.Themes["dusk"]["accent"] != "#89B4FA" {
		t.Fatal(cfg.Themes)
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	again, err := Load()
	if err != nil || again.Theme != "catppuccin-mocha" || again.Action("board", "x") != "new-card" || again.Themes["dusk"]["accent"] != "#89B4FA" {
		t.Fatalf("round trip %+v %v", again, err)
	}
	if _, err := parse("\n[keys.board]\nnew-card t\n", Default()); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatal(err)
	}
	if cfg.Action("board", " ") != "select" || cfg.Action("menu", "q") != "quit" {
		t.Fatal("space or menu quit")
	}
}

func TestSaveDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	cfg := Default()
	cfg.Path = link
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(secret)
	if err != nil || string(body) != "secret" {
		t.Fatalf("symlink target changed: %q %v", body, err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("config path is still a symlink")
	}
}

func TestRejectsBadConfig(t *testing.T) {
	cases := []string{
		"[nope]\n",
		"theme linux-dex\n[keys.board]\nnope x\n",
		"[keys.board]\nnew-card 1\n",
		"[theme.dusk]\naccent nope\n",
		"[tags]\nbad tag\n",
		"theme ../x\n",
		"[theme.dusk]\naccent 99999\n",
		"[tags]\n../etc\n",
		"[keys.board]\nquit a\nb\n",
	}
	for _, body := range cases {
		if _, err := parse(body, Default()); err == nil {
			t.Fatalf("accepted %q", body)
		}
	}
}
