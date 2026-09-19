package api

import (
	"context"
	cfg "erupe-ce/config"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"erupe-ce/server/patchtree"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// Config holds the dependencies required to initialize an APIServer.
type Config struct {
	Logger      *zap.Logger
	DB          *sqlx.DB
	ErupeConfig *cfg.Config
}

// APIServer is Erupes Standard API interface
type APIServer struct {
	sync.Mutex
	logger         *zap.Logger
	db             *sqlx.DB
	erupeConfig    *cfg.Config
	userRepo       APIUserRepo
	charRepo       APICharacterRepo
	sessionRepo    APISessionRepo
	eventRepo      APIEventRepo
	httpServer     *http.Server
	startTime      time.Time
	isShuttingDown bool
}

// NewAPIServer creates a new Server type.
func NewAPIServer(config *Config) *APIServer {
	s := &APIServer{
		logger:      config.Logger,
		db:          config.DB,
		erupeConfig: config.ErupeConfig,
		httpServer:  &http.Server{},
	}
	if config.DB != nil {
		s.userRepo = NewAPIUserRepository(config.DB)
		s.charRepo = NewAPICharacterRepository(config.DB)
		s.sessionRepo = NewAPISessionRepository(config.DB)
		s.eventRepo = NewAPIEventRepository(config.DB)
	}
	return s
}

// Start starts the server in a new goroutine.
func (s *APIServer) Start() error {
	s.startTime = time.Now()

	// Set up the routes responsible for serving the launcher HTML, serverlist, unique name check, and JP auth.
	r := mux.NewRouter()

	// Dashboard routes (before catch-all)
	r.HandleFunc("/dashboard", s.Dashboard)
	r.HandleFunc("/api/dashboard/stats", s.DashboardStatsJSON).Methods("GET")

	// Legacy routes. The auth/mutation endpoints below decode a JSON request
	// body, so they must be POST-only: without method enforcement, bare GET
	// probes from internet scanners reach the handler and fail body decoding,
	// producing needless error-level log noise. The v2 equivalents already
	// enforce methods; these now match.
	r.HandleFunc("/launcher", s.Launcher)
	r.HandleFunc("/login", s.Login).Methods("POST")
	r.HandleFunc("/register", s.Register).Methods("POST")
	r.HandleFunc("/character/create", s.CreateCharacter).Methods("POST")
	r.HandleFunc("/character/delete", s.DeleteCharacter).Methods("POST")
	r.HandleFunc("/character/export", s.ExportSave).Methods("POST")
	r.HandleFunc("/api/ss/bbs/upload.php", s.ScreenShot)
	r.HandleFunc("/api/ss/bbs/{id}", s.ScreenShotGet)
	r.HandleFunc("/", s.LandingPage)
	r.HandleFunc("/health", s.Health)
	r.HandleFunc("/version", s.Version)

	// Game-file tree for the original launcher and mhf-outpost, when the
	// operator chose to host it from Erupe rather than a web server.
	if s.erupeConfig.API.PatchTree.Enabled {
		s.mountPatchTree(r)
	}

	// V2 routes (with HTTP method enforcement)
	v2 := r.PathPrefix("/v2").Subrouter()
	v2.HandleFunc("/login", s.Login).Methods("POST")
	v2.HandleFunc("/register", s.Register).Methods("POST")
	v2.HandleFunc("/launcher", s.Launcher).Methods("GET")
	v2.HandleFunc("/version", s.Version).Methods("GET")
	v2.HandleFunc("/health", s.Health).Methods("GET")
	v2.HandleFunc("/server/status", s.ServerStatus).Methods("GET")
	v2.HandleFunc("/server/info", s.ServerInfo).Methods("GET")

	// V2 authenticated routes
	v2Auth := v2.PathPrefix("").Subrouter()
	v2Auth.Use(s.AuthMiddleware)
	v2Auth.HandleFunc("/characters", s.CreateCharacter).Methods("POST")
	v2Auth.HandleFunc("/characters/{id}/delete", s.DeleteCharacter).Methods("POST")
	v2Auth.HandleFunc("/characters/{id}", s.DeleteCharacter).Methods("DELETE")
	v2Auth.HandleFunc("/characters/{id}/export", s.ExportSave).Methods("GET")
	v2Auth.HandleFunc("/characters/{id}/import", s.ImportSave).Methods("POST")

	handler := handlers.CORS(
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)(r)
	s.httpServer.Handler = handlers.LoggingHandler(os.Stdout, handler)
	s.httpServer.Addr = fmt.Sprintf(":%d", s.erupeConfig.API.Port)

	serveError := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil {
			// Send error if any.
			serveError <- err
		}
	}()

	// Get the error from calling ListenAndServe, otherwise assume it's good after 250 milliseconds.
	select {
	case err := <-serveError:
		return err
	case <-time.After(250 * time.Millisecond):
		return nil
	}
}

// Shutdown exits the server gracefully.
func (s *APIServer) Shutdown() {
	s.logger.Debug("Shutting down")

	s.Lock()
	s.isShuttingDown = true
	s.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		// Just warn because we are shutting down the server anyway.
		s.logger.Warn("Got error on httpServer shutdown", zap.Error(err))
	}
}

// mountPatchTree registers the launcher-facing file routes and makes sure
// the manifest reflects the files on disk. Manifest generation reads every
// byte of the tree (about 5 GB for ZZ), so it runs in the background and only
// when a file is newer than the manifest; the manifest route answers 503
// until the first one exists.
func (s *APIServer) mountPatchTree(r *mux.Router) {
	root := s.erupeConfig.API.PatchTree.Root
	r.Handle("/mhf_file.php", patchtree.ManifestHandler(root))
	r.PathPrefix("/" + patchtree.FilesDir + "/").Handler(patchtree.FilesHandler(root))

	stale, err := patchtree.Stale(root)
	if err != nil {
		s.logger.Warn("Patch tree: cannot read root; files will not be served",
			zap.String("root", root), zap.Error(err))
		return
	}
	s.logger.Info("Patch tree: serving game files",
		zap.String("root", root), zap.String("advertised", s.erupeConfig.API.PatchServer))
	if !stale {
		return
	}
	go func() {
		s.logger.Info("Patch tree: manifest missing or out of date, regenerating", zap.String("root", root))
		start := time.Now()
		n, err := patchtree.Generate(root)
		if err != nil {
			s.logger.Error("Patch tree: manifest generation failed", zap.Error(err))
			return
		}
		s.logger.Info("Patch tree: manifest regenerated",
			zap.Int("files", n), zap.Duration("took", time.Since(start).Round(time.Millisecond)))
	}()
}
