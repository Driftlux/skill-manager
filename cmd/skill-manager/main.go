package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"skill-manager/internal/server"
	"skill-manager/internal/skills"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	addr := flag.String("addr", "127.0.0.1:5174", "HTTP listen address")
	userSkillsRoot := flag.String("skills-root", filepath.Join(home, ".codex", "skills"), "user Codex skills root")
	systemSkillsRoot := flag.String("system-skills-root", filepath.Join(home, ".codex", "skills", ".system"), "system Codex skills root")
	legacyDisabledRoot := flag.String("legacy-disabled-root", filepath.Join(home, ".codex", "skills.disabled"), "legacy disabled skills root")
	trashRoot := flag.String("trash-root", filepath.Join(home, ".codex", "skills.trash"), "soft-delete trash root")
	pluginCacheRoot := flag.String("plugin-cache-root", filepath.Join(home, ".codex", "plugins", "cache"), "Codex plugin cache root")
	configPath := flag.String("config", filepath.Join(home, ".codex", "config.toml"), "Codex config.toml path")
	staticDir := flag.String("static-dir", filepath.Join("web", "dist"), "static frontend directory")
	flag.Parse()

	manager := skills.NewManager(skills.Options{
		UserSkillsRoot:     *userSkillsRoot,
		SystemSkillsRoot:   *systemSkillsRoot,
		LegacyDisabledRoot: *legacyDisabledRoot,
		TrashRoot:          *trashRoot,
		PluginCacheRoot:    *pluginCacheRoot,
		ConfigPath:         *configPath,
	})
	handler := server.NewRouter(manager, *staticDir)

	log.Printf("skill-manager listening on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
