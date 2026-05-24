package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"strm/internal/db"
	"strm/internal/openlist"
	"strm/internal/scanner"
	"strm/internal/scheduler"
)

type Server struct {
	store    *db.Store
	manager  *scheduler.Manager
	sessions *sessionManager
	mux      *http.ServeMux
}

func New(store *db.Store, manager *scheduler.Manager) *Server {
	s := &Server{
		store:    store,
		manager:  manager,
		sessions: newSessionManager(),
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	s.mux.HandleFunc("POST /api/auth/login", s.login)
	s.mux.HandleFunc("GET /api/auth/session", s.session)
	s.mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.logout))
	s.mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.me))
	s.mux.HandleFunc("GET /api/status", s.requireAuth(s.status))
	s.mux.HandleFunc("GET /api/settings/openlist", s.requireAuth(s.getOpenListSettings))
	s.mux.HandleFunc("PUT /api/settings/openlist", s.requireAuth(s.saveOpenListSettings))
	s.mux.HandleFunc("POST /api/settings/openlist/test", s.requireAuth(s.testOpenListSettings))
	s.mux.HandleFunc("GET /api/tasks", s.requireAuth(s.listTasks))
	s.mux.HandleFunc("POST /api/tasks", s.requireAuth(s.createTask))
	s.mux.HandleFunc("GET /api/tasks/{id}", s.requireAuth(s.getTask))
	s.mux.HandleFunc("PUT /api/tasks/{id}", s.requireAuth(s.updateTask))
	s.mux.HandleFunc("DELETE /api/tasks/{id}", s.requireAuth(s.deleteTask))
	s.mux.HandleFunc("POST /api/tasks/{id}/run", s.requireAuth(s.runTask))
	s.mux.HandleFunc("POST /api/tasks/{id}/stop", s.requireAuth(s.stopTask))
	s.mux.HandleFunc("GET /api/runs", s.requireAuth(s.listRuns))
	s.mux.HandleFunc("GET /api/runs/{id}", s.requireAuth(s.getRun))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.sessions.get(r); !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := s.store.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err := s.sessions.create(w, user.ID, user.Username); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"username": user.Username})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if sess, ok := s.sessions.get(r); ok {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": sess.Username})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.sessions.clear(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	sess, _ := s.sessions.get(r)
	writeJSON(w, http.StatusOK, map[string]any{"username": sess.Username})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"active_runs": s.manager.ActiveRuns()})
}

func (s *Server) getOpenListSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetOpenListSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings.PasswordSet = settings.PasswordHash != ""
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) saveOpenListSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseURL         string `json:"base_url"`
		DownloadBaseURL string `json:"download_base_url"`
		Username        string `json:"username"`
		Password        string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	current, err := s.store.GetOpenListSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	current.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	current.DownloadBaseURL = strings.TrimRight(strings.TrimSpace(req.DownloadBaseURL), "/")
	current.Username = strings.TrimSpace(req.Username)
	if req.Password != "" {
		current.PasswordHash = openlist.HashPassword(req.Password)
	}
	if err := s.store.SaveOpenListSettings(r.Context(), current); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	current.PasswordSet = current.PasswordHash != ""
	writeJSON(w, http.StatusOK, current)
}

func (s *Server) testOpenListSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.GetOpenListSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if settings.BaseURL == "" || settings.Username == "" || settings.PasswordHash == "" {
		writeError(w, http.StatusBadRequest, "openlist settings are incomplete")
		return
	}
	client := openlist.NewClient(settings.BaseURL, nil)
	if err := client.LoginHash(r.Context(), settings.Username, settings.PasswordHash); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.store.ListTasks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	task := scanner.DefaultTask()
	if !decodeJSON(w, r, &task) {
		return
	}
	task, err := scanner.NormalizeTask(task)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err = s.store.CreateTask(r.Context(), task)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = s.manager.Reload(r.Context())
	writeJSON(w, http.StatusCreated, task)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	task, err := s.store.GetTask(r.Context(), pathID(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	task := scanner.DefaultTask()
	if !decodeJSON(w, r, &task) {
		return
	}
	task.ID = pathID(r)
	var err error
	task, err = scanner.NormalizeTask(task)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err = s.store.UpdateTask(r.Context(), task)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	_ = s.manager.Reload(r.Context())
	writeJSON(w, http.StatusOK, task)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id := pathID(r)
	if s.manager.StopRun(id) {
		writeError(w, http.StatusConflict, "task is running")
		return
	}
	if err := s.store.DeleteTask(r.Context(), id); err != nil {
		writeStoreError(w, err)
		return
	}
	_ = s.manager.Reload(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runTask(w http.ResponseWriter, r *http.Request) {
	run, err := s.manager.StartRun(r.Context(), pathID(r), "manual")
	if err != nil {
		if errors.Is(err, scheduler.ErrTaskRunning) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, run)
}

func (s *Server) stopTask(w http.ResponseWriter, r *http.Request) {
	ok := s.manager.StopRun(pathID(r))
	writeJSON(w, http.StatusOK, map[string]any{"stopped": ok})
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	runs, err := s.store.ListRuns(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	run, events, err := s.store.GetRun(r.Context(), pathID(r))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "events": events})
}

func pathID(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, db.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
