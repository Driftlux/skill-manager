package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePluginSkill(t *testing.T, cacheRoot, marketplace, plugin, version, skill, body string) string {
	t.Helper()
	dir := filepath.Join(cacheRoot, marketplace, plugin, version, "skills", skill)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newTestManager(t *testing.T, config string) *Manager {
	t.Helper()
	base := t.TempDir()
	configPath := filepath.Join(base, "config.toml")
	if err := os.WriteFile(configPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewManager(Options{
		UserSkillsRoot:      filepath.Join(base, "skills"),
		SystemSkillsRoot:    filepath.Join(base, "skills", ".system"),
		LegacyDisabledRoot:  filepath.Join(base, "skills.disabled"),
		TrashRoot:           filepath.Join(base, "skills.trash"),
		PluginCacheRoot:     filepath.Join(base, "plugins", "cache"),
		ConfigPath:          configPath,
		BackupTimestampFunc: func() string { return "20260630T101112" },
	})
}

func TestListSkillsUsesConfigStatusAndSources(t *testing.T) {
	manager := newTestManager(t, `
[plugins."demo@market"]
enabled = true

[[skills.config]]
path = "`+filepath.Join("ROOT", "skills", "ui-design", "SKILL.md")+`"
enabled = false
`)
	userSkillPath := filepath.Join(manager.UserSkillsRoot, "ui-design", "SKILL.md")
	config := strings.ReplaceAll(mustRead(t, manager.ConfigPath), filepath.Join("ROOT", "skills", "ui-design", "SKILL.md"), userSkillPath)
	if err := os.WriteFile(manager.ConfigPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, manager.UserSkillsRoot, "ui-design", "---\nname: UI Design\ndescription: User skill\n---\n")
	writeSkill(t, manager.SystemSkillsRoot, "imagegen", "---\nname: imagegen\ndescription: System skill\n---\n")
	pluginSkillPath := writePluginSkill(t, manager.PluginCacheRoot, "market", "demo", "1.0.0", "make", "---\nname: make\ndescription: Plugin skill\n---\n")

	items, err := manager.ListSkills()
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string]Skill{}
	for _, item := range items {
		byPath[item.SkillFilePath] = item
	}
	if byPath[userSkillPath].Status != StatusDisabled || byPath[userSkillPath].Source != SourceUser {
		t.Fatalf("expected user skill disabled from config, got %#v", byPath[userSkillPath])
	}
	if byPath[filepath.Join(manager.SystemSkillsRoot, "imagegen", "SKILL.md")].Source != SourceSystem {
		t.Fatalf("expected system skill source, got %#v", byPath[filepath.Join(manager.SystemSkillsRoot, "imagegen", "SKILL.md")])
	}
	if byPath[pluginSkillPath].Source != SourcePlugin || byPath[pluginSkillPath].Status != StatusEnabled {
		t.Fatalf("expected plugin skill enabled by plugin, got %#v", byPath[pluginSkillPath])
	}
}

func TestDisableAndEnableSkillOnlyEditConfigAndCreateBackup(t *testing.T) {
	manager := newTestManager(t, "model = \"x\"\n")
	writeSkill(t, manager.UserSkillsRoot, "demo", "---\nname: demo\n---\n")
	skillDir := filepath.Join(manager.UserSkillsRoot, "demo")

	if err := manager.DisableSkill("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(skillDir); err != nil {
		t.Fatalf("skill directory moved or removed: %v", err)
	}
	config := mustRead(t, manager.ConfigPath)
	if !strings.Contains(config, `path = "`+filepath.Join(skillDir, "SKILL.md")+`"`) || !strings.Contains(config, "enabled = false") {
		t.Fatalf("disable did not append disabled config:\n%s", config)
	}
	if backups := mustGlob(t, manager.ConfigPath+".bak.*"); len(backups) != 1 {
		t.Fatalf("expected one backup after disable, got %v", backups)
	}

	if err := manager.EnableSkill("demo"); err != nil {
		t.Fatal(err)
	}
	config = mustRead(t, manager.ConfigPath)
	if !strings.Contains(config, "enabled = true") || strings.Contains(config, "skills.disabled") {
		t.Fatalf("enable should only edit config, got:\n%s", config)
	}
	if _, err := os.Stat(skillDir); err != nil {
		t.Fatalf("skill directory should stay in place: %v", err)
	}
}

func TestDeleteUserSkillSoftDeletesAndCleansConfig(t *testing.T) {
	manager := newTestManager(t, "")
	writeSkill(t, manager.UserSkillsRoot, "demo", "---\nname: demo\n---\n")
	if err := manager.DisableSkill("demo"); err != nil {
		t.Fatal(err)
	}

	if err := manager.DeleteSkill("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(manager.UserSkillsRoot, "demo")); !os.IsNotExist(err) {
		t.Fatalf("expected user skill moved out of active root, got %v", err)
	}
	trashMatches := mustGlob(t, filepath.Join(manager.TrashRoot, "20260630T101112-demo"))
	if len(trashMatches) != 1 {
		t.Fatalf("expected soft-deleted skill in trash, got %v", trashMatches)
	}
	if strings.Contains(mustRead(t, manager.ConfigPath), "demo/SKILL.md") {
		t.Fatalf("expected skill config entry cleaned, got:\n%s", mustRead(t, manager.ConfigPath))
	}
}

func TestListAndTogglePlugins(t *testing.T) {
	manager := newTestManager(t, `
[plugins."demo@market"]
enabled = true

[plugins."other@market"]
enabled = false
`)
	writePluginSkill(t, manager.PluginCacheRoot, "market", "demo", "1.0.0", "make", "---\nname: make\n---\n")
	writePluginSkill(t, manager.PluginCacheRoot, "market", "other", "2.0.0", "draw", "---\nname: draw\n---\n")

	plugins, err := manager.ListPlugins()
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 2 || plugins[0].ID != "demo@market" || len(plugins[0].Skills) != 1 {
		t.Fatalf("unexpected plugins: %#v", plugins)
	}

	if err := manager.SetPluginEnabled("demo@market", false); err != nil {
		t.Fatal(err)
	}
	config := mustRead(t, manager.ConfigPath)
	if !strings.Contains(config, `[plugins."demo@market"]`+"\nenabled = false") {
		t.Fatalf("expected target plugin disabled, got:\n%s", config)
	}
	if !strings.Contains(config, `[plugins."other@market"]`+"\nenabled = false") {
		t.Fatalf("other plugin block should remain unchanged, got:\n%s", config)
	}
	if err := manager.SetPluginEnabled("missing@market", true); err == nil {
		t.Fatal("expected missing plugin config to return error")
	}
}

func TestMigrateLegacyDisabledSkillMovesBackAndWritesDisabledConfig(t *testing.T) {
	manager := newTestManager(t, "")
	writeSkill(t, manager.LegacyDisabledRoot, "legacy", "---\nname: legacy\n---\n")

	if err := manager.MigrateLegacyDisabledSkill("legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(manager.UserSkillsRoot, "legacy", "SKILL.md")); err != nil {
		t.Fatalf("expected legacy skill restored to user root: %v", err)
	}
	config := mustRead(t, manager.ConfigPath)
	if !strings.Contains(config, filepath.Join(manager.UserSkillsRoot, "legacy", "SKILL.md")) || !strings.Contains(config, "enabled = false") {
		t.Fatalf("expected restored skill to be disabled in config, got:\n%s", config)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return matches
}
