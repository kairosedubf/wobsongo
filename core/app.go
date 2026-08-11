package core

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kairosedubf/wobsongo/auth"
	"github.com/kairosedubf/wobsongo/config"
	"github.com/kairosedubf/wobsongo/data"
	"github.com/kairosedubf/wobsongo/handler"
	"github.com/kairosedubf/wobsongo/ui"
	"github.com/kairosedubf/wobsongo/validation"
	"github.com/kairosedubf/wobsongo/webhandler"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	ReadTimeout  = 5 * time.Second
	WriteTimeout = 90 * time.Second
)

type App struct {
	config        *config.Config
	echoApp       *echo.Echo
	apiGroup      *echo.Group
	documentRepo  data.DocumentRepoer
	mediaProvider data.MediaUploadProvider
	rawStore      data.RawObjectStore
	chunkRepo     data.DocumentChunkRepoer
	knowledgeRepo data.AtomicKnowledgeRepoer
	embedder      data.Embedder
	claimAnalyzer data.ClaimAnalyzer
	claimJudge    data.ClaimJudge
	userRepo      data.UserRepoer
}

// Echo returns the Echo instance of the application.
func (app *App) Echo() *echo.Echo {
	return app.echoApp
}

// Config returns the application configuration.
func (app *App) Config() *config.Config {
	return app.config
}

// Server returns the HTTP server instance configured with the application's Echo instance.
func (app *App) Server() *http.Server {
	return &http.Server{
		Addr:         fmt.Sprintf(":%d", app.config.Port),
		Handler:      app.Echo(),
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
	}
}

// Start starts the HTTP server and listens for incoming requests.
func (app *App) Start() error {
	return app.Server().ListenAndServe()
}

// APIGroup returns the /api Echo group so callers (e.g. a commercial
// extension) can mount additional routes on the same Echo instance without
// touching OSS internals.
func (app *App) APIGroup() *echo.Group {
	return app.apiGroup
}

// AppOption defines a function type for configuring the App with optional dependencies.
type AppOption func(*App)

// WithDocumentRepo sets the document repository for the application.
func WithDocumentRepo(repo data.DocumentRepoer) AppOption {
	return func(a *App) {
		a.documentRepo = repo
	}
}

// WithMediaProvider sets the media upload provider for the application.
func WithMediaProvider(provider data.MediaUploadProvider) AppOption {
	return func(a *App) {
		a.mediaProvider = provider
	}
}

// WithRawObjectStore sets the raw S3 store used for server-side file uploads.
func WithRawObjectStore(s data.RawObjectStore) AppOption {
	return func(a *App) {
		a.rawStore = s
	}
}

// WithChunkRepo sets the document chunk repository for the application.
func WithChunkRepo(repo data.DocumentChunkRepoer) AppOption {
	return func(a *App) {
		a.chunkRepo = repo
	}
}

// WithKnowledgeRepo sets the atomic knowledge repository for the application.
func WithKnowledgeRepo(repo data.AtomicKnowledgeRepoer) AppOption {
	return func(a *App) {
		a.knowledgeRepo = repo
	}
}

// WithEmbedder sets the embedding client used for hybrid-search retrieval.
func WithEmbedder(embedder data.Embedder) AppOption {
	return func(a *App) {
		a.embedder = embedder
	}
}

// WithClaimAnalyzer sets the claim scope/decomposition analyzer for the application.
func WithClaimAnalyzer(analyzer data.ClaimAnalyzer) AppOption {
	return func(a *App) {
		a.claimAnalyzer = analyzer
	}
}

// WithClaimJudge sets the claim judge for the application.
func WithClaimJudge(judge data.ClaimJudge) AppOption {
	return func(a *App) {
		a.claimJudge = judge
	}
}

// WithUserRepo sets the user repository for the application's web layer.
func WithUserRepo(repo data.UserRepoer) AppOption {
	return func(a *App) {
		a.userRepo = repo
	}
}

// NewApp initializes the application with the given Echo instance, version,
// and optional dependencies. Returns a pointer to the app instance
// with singleton behavior.
func NewApp(e *echo.Echo, config *config.Config, optionFuncs ...AppOption) *App {
	e.HideBanner = true

	api := e.Group("/api")

	app := &App{
		config:   config,
		echoApp:  e,
		apiGroup: api,
	}

	handler.UseCustomErrorHandler(app.Echo())
	handler.UseGlobalMiddlewares(app.Echo())

	if err := validation.Register(app.Echo()); err != nil {
		panic(fmt.Errorf("failed to register DTO validator: %w", err))
	}

	// Run option functions to set optional dependencies.
	for _, optionFunc := range optionFuncs {
		optionFunc(app)
	}

	// Initialize repositories and handlers.
	repos := new(handler.Repos)
	repos.DocumentRepo = app.documentRepo
	repos.MediaProvider = app.mediaProvider
	repos.ChunkRepo = app.chunkRepo
	repos.KnowledgeRepo = app.knowledgeRepo
	repos.Embedder = app.embedder
	repos.ClaimAnalyzer = app.claimAnalyzer
	repos.ClaimJudge = app.claimJudge

	handlers := handler.NewHandlers(repos)
	handlers.RegisterRoutes(app.apiGroup)

	// HTML routes, mounted alongside /api on the same Echo instance — token
	// comes from a cookie instead of the Authorization header.
	jwtAuth := auth.New(config.JWTSecret, config.JWTExpiryHours)
	webGroup := e.Group("",
		handler.JWTFromCookieMiddleware(webhandler.AuthCookieName),
		handler.JWTParserMiddleware(jwtAuth),
		middleware.CSRFWithConfig(middleware.CSRFConfig{
			TokenLookup:    "form:_csrf",
			CookieSecure:   config.IsProduction(),
			CookieSameSite: http.SameSiteLaxMode,
		}),
	)
	webhandler.RegisterWebRoutes(
		webGroup,
		&webhandler.WebRepos{
			UserRepo:      app.userRepo,
			DocumentRepo:  app.documentRepo,
			RawStore:      app.rawStore,
			ChunkRepo:     app.chunkRepo,
			KnowledgeRepo: app.knowledgeRepo,
			MediaProvider: app.mediaProvider,
			Embedder:      app.embedder,
			ClaimAnalyzer: app.claimAnalyzer,
			ClaimJudge:    app.claimJudge,
		},
		jwtAuth, config,
	)

	e.StaticFS("/static", ui.StaticFS)

	return app
}
