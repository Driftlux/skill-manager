package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skill-manager/internal/skills"
)

func testRouter(t *testing.T) (http.Handler, *skills.Manager) {
	t.Helper()
	base := t.TempDir()
	configPath := filepath.Join(base, "config.toml")
	if err := os.WriteFile(configPath, []byte("[plugins.\"demo@market\"]\nenabled = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := skills.NewManager(skills.Options{
		UserSkillsRoot:      filepath.Join(base, "skills"),
		SystemSkillsRoot:    filepath.Join(base, "skills", ".system"),
		LegacyDisabledRoot:  filepath.Join(base, "skills.disabled"),
		TrashRoot:           filepath.Join(base, "skills.trash"),
		PluginCacheRoot:     filepath.Join(base, "plugins", "cache"),
		ConfigPath:          configPath,
		BackupTimestampFunc: func() string { return "20260630T101112" },
	})
	return NewRouter(manager, ""), manager
}

func writeSkill(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAPIListSkills(t *testing.T) {
	router, manager := testRouter(t)
	writeSkill(t, manager.UserSkillsRoot, "demo")

	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}
	if ct := res.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("expected json response, got %q", ct)
	}
	if !strings.Contains(res.Body.String(), `"source":"user"`) {
		t.Fatalf("expected source in response, got %s", res.Body.String())
	}
}

func TestAPIRoutesRejectPathTraversal(t *testing.T) {
	router, _ := testRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/skills/..%2Fdemo/disable", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", res.Code)
	}
}

func TestAPIEnableDisableDeleteAndPlugins(t *testing.T) {
	router, manager := testRouter(t)
	writeSkill(t, manager.UserSkillsRoot, "demo")

	req := httptest.NewRequest(http.MethodPost, "/api/skills/demo/disable", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("disable expected 200, got %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/skills/demo/enable", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("enable expected 200, got %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "demo@market") {
		t.Fatalf("plugins expected 200 with config plugin, got %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/plugins/demo%40market/disable", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("plugin disable expected 200, got %d: %s", res.Code, res.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/skills/demo", nil)
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("delete expected 200, got %d: %s", res.Code, res.Body.String())
	}
}
