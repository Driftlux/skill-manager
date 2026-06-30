package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Status string
type Source string

const (
	StatusEnabled  Status = "enabled"
	StatusDisabled Status = "disabled"
	StatusInvalid  Status = "invalid"

	SourceUser   Source = "user"
	SourceSystem Source = "system"
	SourcePlugin Source = "plugin"
)

type ConfigEntry struct {
	Name    string `json:"name,omitempty"`
	Path    string `json:"path,omitempty"`
	Enabled bool   `json:"enabled"`
}

type Skill struct {
	Name           string       `json:"name"`
	Title          string       `json:"title"`
	Description    string       `json:"description,omitempty"`
	Source         Source       `json:"source"`
	Status         Status       `json:"status"`
	Path           string       `json:"path"`
	SkillFilePath  string       `json:"skillFilePath"`
	HasSkillFile   bool         `json:"hasSkillFile"`
	ConfigEntry    *ConfigEntry `json:"configEntry"`
	LegacyDisabled bool         `json:"legacyDisabled,omitempty"`
	PluginID       string       `json:"pluginId,omitempty"`
}

type PluginSkill struct {
	Name                 string `json:"name"`
	Title                string `json:"title"`
	Description          string `json:"description,omitempty"`
	Path                 string `json:"path"`
	EnabledByPlugin      bool   `json:"enabledByPlugin"`
	IndividuallyDisabled bool   `json:"individuallyDisabled"`
}

type Plugin struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Marketplace string        `json:"marketplace"`
	Enabled     bool          `json:"enabled"`
	ConfigPath  string        `json:"configPath"`
	CachePaths  []string      `json:"cachePaths"`
	Skills      []PluginSkill `json:"skills"`
}

type Options struct {
	UserSkillsRoot      string
	SystemSkillsRoot    string
	LegacyDisabledRoot  string
	TrashRoot           string
	PluginCacheRoot     string
	ConfigPath          string
	BackupTimestampFunc func() string
}

type Manager struct {
	UserSkillsRoot      string
	SystemSkillsRoot    string
	LegacyDisabledRoot  string
	TrashRoot           string
	PluginCacheRoot     string
	ConfigPath          string
	backupTimestampFunc func() string
}

func NewManager(options Options) *Manager {
	return &Manager{
		UserSkillsRoot:      filepath.Clean(options.UserSkillsRoot),
		SystemSkillsRoot:    filepath.Clean(options.SystemSkillsRoot),
		LegacyDisabledRoot:  filepath.Clean(options.LegacyDisabledRoot),
		TrashRoot:           filepath.Clean(options.TrashRoot),
		PluginCacheRoot:     filepath.Clean(options.PluginCacheRoot),
		ConfigPath:          filepath.Clean(options.ConfigPath),
		backupTimestampFunc: options.BackupTimestampFunc,
	}
}

func (m *Manager) timestamp() string {
	if m.backupTimestampFunc != nil {
		return m.backupTimestampFunc()
	}
	return time.Now().Format("20060102T150405")
}

func (m *Manager) ListSkills() ([]Skill, error) {
	config, err := m.readConfig()
	if err != nil {
		return nil, err
	}
	plugins, err := m.pluginsFromConfig(config)
	if err != nil {
		return nil, err
	}

	var result []Skill
	user, err := m.listSkillRoot(m.UserSkillsRoot, SourceUser, "", config, true)
	if err != nil {
		return nil, err
	}
	system, err := m.listSkillRoot(m.SystemSkillsRoot, SourceSystem, "", config, false)
	if err != nil {
		return nil, err
	}
	legacy, err := m.listSkillRoot(m.LegacyDisabledRoot, SourceUser, "", config, true)
	if err != nil {
		return nil, err
	}
	for i := range legacy {
		legacy[i].LegacyDisabled = true
		if legacy[i].Status != StatusInvalid {
			legacy[i].Status = StatusDisabled
		}
	}
	result = append(result, user...)
	result = append(result, system...)
	result = append(result, legacy...)

	for _, plugin := range plugins {
		for _, cachePath := range plugin.CachePaths {
			pluginSkills, err := m.listPluginSkills(cachePath, plugin.ID, plugin.Enabled, config)
			if err != nil {
				return nil, err
			}
			result = append(result, pluginSkills...)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (m *Manager) DisableSkill(name string) error {
	skill, err := m.findActionableSkill(name)
	if err != nil {
		return err
	}
	if !skill.HasSkillFile {
		return fmt.Errorf("skill %q has no SKILL.md", name)
	}
	return m.setSkillEnabled(skill, false)
}

func (m *Manager) EnableSkill(name string) error {
	skill, err := m.findActionableSkill(name)
	if err != nil {
		return err
	}
	if !skill.HasSkillFile {
		return fmt.Errorf("skill %q has no SKILL.md", name)
	}
	return m.setSkillEnabled(skill, true)
}

func (m *Manager) DeleteSkill(name string) error {
	skill, err := m.findActionableSkill(name)
	if err != nil {
		return err
	}
	if skill.Source != SourceUser || skill.LegacyDisabled {
		return fmt.Errorf("delete is only allowed for installed user skills")
	}
	if err := m.validatePathInRoot(skill.Path, m.UserSkillsRoot); err != nil {
		return err
	}

	config, err := m.readConfig()
	if err != nil {
		return err
	}
	updated := config.removeSkillEntry(skill.SkillFilePath, skill.Name)
	if err := m.writeConfig(updated.String()); err != nil {
		return err
	}

	target := filepath.Join(m.TrashRoot, m.timestamp()+"-"+skill.Name)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("trash target already exists: %s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Rename(skill.Path, target); err != nil {
		return fmt.Errorf("soft delete %s to %s: %w", skill.Path, target, err)
	}
	return nil
}

func (m *Manager) MigrateLegacyDisabledSkill(name string) error {
	source, err := m.skillDirPath(m.LegacyDisabledRoot, name, false)
	if err != nil {
		return err
	}
	target, err := m.skillDirPath(m.UserSkillsRoot, name, false)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("target already exists, refusing to overwrite: %s", target)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	if err := os.Rename(source, target); err != nil {
		return err
	}
	skill := Skill{Name: name, Path: target, SkillFilePath: filepath.Join(target, "SKILL.md")}
	return m.setSkillEnabled(skill, false)
}

func (m *Manager) ListPlugins() ([]Plugin, error) {
	config, err := m.readConfig()
	if err != nil {
		return nil, err
	}
	return m.pluginsFromConfig(config)
}

func (m *Manager) SetPluginEnabled(id string, enabled bool) error {
	if !validPluginID(id) {
		return fmt.Errorf("invalid plugin id %q", id)
	}
	config, err := m.readConfig()
	if err != nil {
		return err
	}
	updated, ok := config.setPluginEnabled(id, enabled)
	if !ok {
		return fmt.Errorf("plugin config %q does not exist", id)
	}
	return m.writeConfig(updated.String())
}

func (m *Manager) setSkillEnabled(skill Skill, enabled bool) error {
	if err := m.validateSkillFilePath(skill.SkillFilePath); err != nil {
		return err
	}
	config, err := m.readConfig()
	if err != nil {
		return err
	}
	updated := config.setSkillEnabled(skill.SkillFilePath, skill.Name, enabled)
	return m.writeConfig(updated.String())
}

func (m *Manager) findActionableSkill(name string) (Skill, error) {
	if err := validateName(name); err != nil {
		return Skill{}, err
	}
	items, err := m.ListSkills()
	if err != nil {
		return Skill{}, err
	}
	var matches []Skill
	for _, item := range items {
		if item.Name == name && !item.LegacyDisabled {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return Skill{}, fmt.Errorf("skill %q does not exist", name)
	}
	if len(matches) > 1 {
		return Skill{}, fmt.Errorf("skill name %q is ambiguous", name)
	}
	return matches[0], nil
}

func (m *Manager) listSkillRoot(root string, source Source, pluginID string, config *configFile, allowMissingSkillFile bool) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", root, err)
	}
	var result []Skill
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".system" {
			continue
		}
		dir, err := m.skillDirPath(root, entry.Name(), source == SourceSystem)
		if err != nil {
			continue
		}
		skillFile := filepath.Join(dir, "SKILL.md")
		title, description, hasSkillFile := readSkillMetadata(skillFile, entry.Name())
		status := StatusEnabled
		configEntry := config.matchSkill(skillFile, entry.Name())
		if configEntry != nil && !configEntry.Enabled {
			status = StatusDisabled
		}
		if !hasSkillFile && allowMissingSkillFile {
			status = StatusInvalid
		}
		if !hasSkillFile && !allowMissingSkillFile {
			continue
		}
		result = append(result, Skill{
			Name:          entry.Name(),
			Title:         title,
			Description:   description,
			Source:        source,
			Status:        status,
			Path:          dir,
			SkillFilePath: skillFile,
			HasSkillFile:  hasSkillFile,
			ConfigEntry:   configEntry,
			PluginID:      pluginID,
		})
	}
	return result, nil
}

func (m *Manager) listPluginSkills(cachePath, pluginID string, pluginEnabled bool, config *configFile) ([]Skill, error) {
	skillsRoot := filepath.Join(cachePath, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(skillsRoot, entry.Name())
		skillFile := filepath.Join(dir, "SKILL.md")
		title, description, hasSkillFile := readSkillMetadata(skillFile, entry.Name())
		if !hasSkillFile {
			continue
		}
		configEntry := config.matchSkill(skillFile, pluginID+":"+entry.Name())
		status := StatusEnabled
		if !pluginEnabled || (configEntry != nil && !configEntry.Enabled) {
			status = StatusDisabled
		}
		result = append(result, Skill{
			Name:          pluginID + ":" + entry.Name(),
			Title:         title,
			Description:   description,
			Source:        SourcePlugin,
			Status:        status,
			Path:          dir,
			SkillFilePath: skillFile,
			HasSkillFile:  true,
			ConfigEntry:   configEntry,
			PluginID:      pluginID,
		})
	}
	return result, nil
}

func (m *Manager) pluginsFromConfig(config *configFile) ([]Plugin, error) {
	var plugins []Plugin
	for _, block := range config.pluginBlocks {
		name, marketplace, _ := strings.Cut(block.ID, "@")
		plugin := Plugin{
			ID:          block.ID,
			Name:        name,
			Marketplace: marketplace,
			Enabled:     block.Enabled,
			ConfigPath:  m.ConfigPath,
			CachePaths:  m.findPluginCachePaths(name, marketplace),
		}
		for _, cachePath := range plugin.CachePaths {
			skills, err := m.pluginSkillsForPlugin(cachePath, plugin.Enabled, config)
			if err != nil {
				return nil, err
			}
			plugin.Skills = append(plugin.Skills, skills...)
		}
		plugins = append(plugins, plugin)
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].ID < plugins[j].ID })
	return plugins, nil
}

func (m *Manager) findPluginCachePaths(name, marketplace string) []string {
	pluginRoot := filepath.Join(m.PluginCacheRoot, marketplace, name)
	versions, err := os.ReadDir(pluginRoot)
	if err != nil {
		return nil
	}
	var paths []string
	for _, version := range versions {
		if version.IsDir() {
			paths = append(paths, filepath.Join(pluginRoot, version.Name()))
		}
	}
	sort.Strings(paths)
	return paths
}

func (m *Manager) pluginSkillsForPlugin(cachePath string, pluginEnabled bool, config *configFile) ([]PluginSkill, error) {
	skillsRoot := filepath.Join(cachePath, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []PluginSkill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(skillsRoot, entry.Name(), "SKILL.md")
		title, description, ok := readSkillMetadata(skillFile, entry.Name())
		if !ok {
			continue
		}
		configEntry := config.matchSkill(skillFile, entry.Name())
		result = append(result, PluginSkill{
			Name:                 entry.Name(),
			Title:                title,
			Description:          description,
			Path:                 skillFile,
			EnabledByPlugin:      pluginEnabled,
			IndividuallyDisabled: configEntry != nil && !configEntry.Enabled,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (m *Manager) readConfig() (*configFile, error) {
	data, err := os.ReadFile(m.ConfigPath)
	if err != nil {
		return nil, err
	}
	return parseConfig(string(data)), nil
}

func (m *Manager) writeConfig(content string) error {
	original, err := os.ReadFile(m.ConfigPath)
	if err != nil {
		return err
	}
	backupPath := m.ConfigPath + ".bak." + m.timestamp()
	if err := os.WriteFile(backupPath, original, 0o600); err != nil {
		return err
	}
	return os.WriteFile(m.ConfigPath, []byte(content), 0o600)
}

func (m *Manager) validateSkillFilePath(path string) error {
	for _, root := range []string{m.UserSkillsRoot, m.SystemSkillsRoot, m.PluginCacheRoot} {
		if err := m.validatePathInRoot(path, root); err == nil {
			return nil
		}
	}
	return fmt.Errorf("skill path is outside allowed roots: %s", path)
}

func (m *Manager) validatePathInRoot(path, root string) error {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path %s is outside allowed root %s", path, root)
	}
	return nil
}

func (m *Manager) skillDirPath(root, name string, allowSystemName bool) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	if name == ".system" && !allowSystemName {
		return "", fmt.Errorf("invalid skill name %q", name)
	}
	path := filepath.Clean(filepath.Join(root, name))
	if err := m.validatePathInRoot(path, root); err != nil {
		return "", err
	}
	return path, nil
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || filepath.Clean(name) != name {
		return fmt.Errorf("invalid skill name %q", name)
	}
	return nil
}

func validPluginID(id string) bool {
	if id == "" || strings.ContainsAny(id, `/\`) || !strings.Contains(id, "@") {
		return false
	}
	return filepath.Clean(id) == id
}

func readSkillMetadata(skillFilePath, fallback string) (string, string, bool) {
	data, err := os.ReadFile(skillFilePath)
	if err != nil {
		return fallback, "", false
	}
	fields := parseFrontmatter(string(data))
	title := strings.TrimSpace(fields["name"])
	if title == "" {
		title = fallback
	}
	return title, strings.TrimSpace(fields["description"]), true
}

func parseFrontmatter(content string) map[string]string {
	fields := map[string]string{}
	content = strings.TrimPrefix(content, "\uFEFF")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fields
	}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return fields
}

type configFile struct {
	lines        []string
	skillBlocks []skillBlock
	pluginBlocks []pluginBlock
}

type skillBlock struct {
	Start   int
	End     int
	Path    string
	Name    string
	Enabled bool
}

type pluginBlock struct {
	Start   int
	End     int
	ID      string
	Enabled bool
}

func parseConfig(content string) *configFile {
	config := &configFile{lines: splitLines(content)}
	for i := 0; i < len(config.lines); i++ {
		trimmed := strings.TrimSpace(strings.TrimRight(config.lines[i], "\n"))
		switch {
		case trimmed == "[[skills.config]]":
			block := skillBlock{Start: i, End: findBlockEnd(config.lines, i+1), Enabled: true}
			for j := i + 1; j < block.End; j++ {
				key, value, ok := parseAssignment(config.lines[j])
				if !ok {
					continue
				}
				switch key {
				case "path":
					block.Path = value
				case "name":
					block.Name = value
				case "enabled":
					block.Enabled = value == "true"
				}
			}
			config.skillBlocks = append(config.skillBlocks, block)
		case strings.HasPrefix(trimmed, `[plugins."`) && strings.HasSuffix(trimmed, `"]`):
			id := strings.TrimSuffix(strings.TrimPrefix(trimmed, `[plugins."`), `"]`)
			block := pluginBlock{Start: i, End: findBlockEnd(config.lines, i+1), ID: id, Enabled: true}
			for j := i + 1; j < block.End; j++ {
				key, value, ok := parseAssignment(config.lines[j])
				if ok && key == "enabled" {
					block.Enabled = value == "true"
				}
			}
			config.pluginBlocks = append(config.pluginBlocks, block)
		}
	}
	return config
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	parts := strings.SplitAfter(content, "\n")
	if !strings.HasSuffix(content, "\n") && len(parts) > 0 {
		return parts
	}
	return parts
}

func findBlockEnd(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") {
			return i
		}
	}
	return len(lines)
}

func parseAssignment(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	key, value, ok := strings.Cut(trimmed, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, `"`) {
		unquoted, err := strconv.Unquote(value)
		if err == nil {
			value = unquoted
		}
	}
	return key, value, true
}

func (c *configFile) String() string {
	return strings.Join(c.lines, "")
}

func (c *configFile) matchSkill(path, name string) *ConfigEntry {
	cleanPath := filepath.Clean(path)
	for _, block := range c.skillBlocks {
		if block.Path != "" && filepath.Clean(block.Path) == cleanPath {
			return &ConfigEntry{Name: block.Name, Path: block.Path, Enabled: block.Enabled}
		}
		if block.Name != "" && (block.Name == name || strings.HasSuffix(block.Name, ":"+name)) {
			return &ConfigEntry{Name: block.Name, Path: block.Path, Enabled: block.Enabled}
		}
	}
	return nil
}

func (c *configFile) setSkillEnabled(path, name string, enabled bool) *configFile {
	copied := c.clone()
	cleanPath := filepath.Clean(path)
	for _, block := range copied.skillBlocks {
		if (block.Path != "" && filepath.Clean(block.Path) == cleanPath) || (block.Name != "" && (block.Name == name || strings.HasSuffix(block.Name, ":"+name))) {
			copied.setEnabledLine(block.Start+1, block.End, enabled)
			return copied
		}
	}
	if enabled {
		return copied
	}
	if len(copied.lines) > 0 && !strings.HasSuffix(copied.lines[len(copied.lines)-1], "\n") {
		copied.lines[len(copied.lines)-1] += "\n"
	}
	copied.lines = append(copied.lines,
		"\n[[skills.config]]\n",
		fmt.Sprintf("path = %q\n", cleanPath),
		"enabled = false\n",
	)
	return parseConfig(copied.String())
}

func (c *configFile) removeSkillEntry(path, name string) *configFile {
	copied := c.clone()
	cleanPath := filepath.Clean(path)
	for i := len(copied.skillBlocks) - 1; i >= 0; i-- {
		block := copied.skillBlocks[i]
		if (block.Path != "" && filepath.Clean(block.Path) == cleanPath) || (block.Name != "" && (block.Name == name || strings.HasSuffix(block.Name, ":"+name))) {
			copied.lines = append(copied.lines[:block.Start], copied.lines[block.End:]...)
			return parseConfig(copied.String())
		}
	}
	return copied
}

func (c *configFile) setPluginEnabled(id string, enabled bool) (*configFile, bool) {
	copied := c.clone()
	for _, block := range copied.pluginBlocks {
		if block.ID == id {
			copied.setEnabledLine(block.Start+1, block.End, enabled)
			return copied, true
		}
	}
	return copied, false
}

func (c *configFile) setEnabledLine(start, end int, enabled bool) {
	value := fmt.Sprintf("enabled = %t\n", enabled)
	for i := start; i < end; i++ {
		key, _, ok := parseAssignment(c.lines[i])
		if ok && key == "enabled" {
			prefix := c.lines[i][:strings.Index(c.lines[i], "enabled")]
			c.lines[i] = prefix + value
			return
		}
	}
	insertAt := end
	c.lines = append(c.lines[:insertAt], append([]string{value}, c.lines[insertAt:]...)...)
}

func (c *configFile) clone() *configFile {
	lines := append([]string(nil), c.lines...)
	return parseConfig(strings.Join(lines, ""))
}
