package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"skill-manager/internal/skills"
)

type apiServer struct {
	manager   *skills.Manager
	staticDir string
}

func NewRouter(manager *skills.Manager, staticDir string) http.Handler {
	server := &apiServer{manager: manager, staticDir: staticDir}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.EscapedPath()
		switch {
		case path == "/api/skills":
			server.handleSkills(w, r)
		case strings.HasPrefix(path, "/api/skills/"):
			server.handleSkillAction(w, r)
		case path == "/api/plugins":
			server.handlePlugins(w, r)
		case strings.HasPrefix(path, "/api/plugins/"):
			server.handlePluginAction(w, r)
		default:
			server.handleStatic(w, r)
		}
	})
}

func (s *apiServer) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.manager.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *apiServer) handleSkillAction(w http.ResponseWriter, r *http.Request) {
	name, action, ok := parseSkillRoute(r.URL.EscapedPath())
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid skill API route")
		return
	}

	var err error
	switch {
	case r.Method == http.MethodPost && action == "enable":
		err = s.manager.EnableSkill(name)
	case r.Method == http.MethodPost && action == "disable":
		err = s.manager.DisableSkill(name)
	case r.Method == http.MethodPost && action == "migrate-legacy-disabled":
		err = s.manager.MigrateLegacyDisabledSkill(name)
	case r.Method == http.MethodDelete && action == "":
		err = s.manager.DeleteSkill(name)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	items, err := s.manager.ListPlugins()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *apiServer) handlePluginAction(w http.ResponseWriter, r *http.Request) {
	id, action, ok := parsePluginRoute(r.URL.EscapedPath())
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid plugin API route")
		return
	}
	var err error
	switch {
	case r.Method == http.MethodPost && action == "enable":
		err = s.manager.SetPluginEnabled(id, true)
	case r.Method == http.MethodPost && action == "disable":
		err = s.manager.SetPluginEnabled(id, false)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *apiServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	if s.staticDir == "" {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
}

func parseSkillRoute(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/api/skills/")
	if rest == path || rest == "" {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		name, err := url.PathUnescape(parts[0])
		return name, "", err == nil
	}
	if len(parts) == 2 {
		name, err := url.PathUnescape(parts[0])
		return name, parts[1], err == nil
	}
	return "", "", false
}

func parsePluginRoute(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/api/plugins/")
	if rest == path || rest == "" {
		return "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	id, err := url.PathUnescape(parts[0])
	return id, parts[1], err == nil
}

func statusForError(err error) int {
	message := err.Error()
	switch {
	case strings.Contains(message, "invalid skill name") || strings.Contains(message, "invalid plugin id") || strings.Contains(message, "outside allowed root") || strings.Contains(message, "ambiguous"):
		return http.StatusBadRequest
	case strings.Contains(message, "does not exist"):
		return http.StatusNotFound
	case strings.Contains(message, "target already exists"):
		return http.StatusConflict
	default:
		if errors.Is(err, os.ErrPermission) {
			return http.StatusForbidden
		}
		return http.StatusInternalServerError
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
