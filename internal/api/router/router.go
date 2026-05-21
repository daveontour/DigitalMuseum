// Package router wires all HTTP routes onto a chi.Mux.
package router

import (
	"database/sql"
	"encoding/json"
	"net/http"

	appai "github.com/daveontour/aimuseum/internal/ai"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/handler"
	"github.com/daveontour/aimuseum/internal/keystore"
	"github.com/daveontour/aimuseum/internal/middleware"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
	backgroundjobs "github.com/daveontour/aimuseum/internal/service/background_jobs"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// New returns the fully-wired application router along with the background-jobs
// scheduler that the caller is expected to start (e.g. cmd/server/main.go).
// The scheduler may be nil if construction fails non-fatally; callers should
// nil-check before launching its goroutine.
// When pool is nil (no main archive DB), a minimal router is returned that serves
// /health, /, /login, /profiles, /static/*, /api/profiles, and /api/resolved-main-sqlite-path
// (GET / redirects to /login).
func New(pool *sql.DB, billingPool *sql.DB, cfg *config.Config) (http.Handler, *backgroundjobs.Scheduler, error) {
	r := chi.NewRouter()

	// ── Global middleware ───────────────────────────────────────────────────────
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)

	// ── Profile repo (always available — billing DB) ──────────────────────────
	profileRepo := repository.NewProfileRepo(billingPool)

	if pool == nil {
		// No main archive DB — serve only what is needed to choose/create one.
		r.Get("/health", healthHandler)
		r.Get("/api/resolved-main-sqlite-path", handler.ResolvedMainSQLitePath(cfg))

		profileHandler := handler.NewProfileHandler(profileRepo, nil, nil)
		profileHandler.RegisterRoutes(r)

		templateHandler := handler.NewTemplateHandler(nil, nil, cfg)
		r.Get("/login", templateHandler.GetLogin)
		// Desktop shell and browsers open "/"; send them to the login / first-run flow.
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/login", http.StatusFound)
		})

		staticFS := http.FileServer(http.Dir(cfg.App.AssetStaticDir))
		r.Handle("/static/*", http.StripPrefix("/static/", staticFS))

		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"no archive configured — select or create an archive first"}`))
		})
		return r, nil, nil
	}

	billingRepo := repository.NewBillingRepo(billingPool)

	// ── Authentication service (shared by handler + middleware) ───────────────
	userRepo := repository.NewUserRepo(pool)
	authSvc := service.NewAuthService(userRepo, cfg.Server.SessionCookieSecure)

	// ── Auth middleware — runs on every request after RealIP ───────────────────
	// Auth endpoints and static assets are exempt (see middleware.isExempt).
	// Unauthenticated requests to non-exempt paths get 401 JSON or a redirect to /login.
	r.Use(middleware.NewAuthMiddleware(authSvc))

	// ── Health check ───────────────────────────────────────────────────────────
	r.Get("/health", healthHandler)
	r.Get("/api/resolved-main-sqlite-path", handler.ResolvedMainSQLitePath(cfg))

	sessionMasterStore := keystore.NewSessionMasterStore(cfg.Server.SessionCookieSecure)

	localAIProvider := appai.NewLocalAIProvider(cfg.AI.LocalAIBaseURL, cfg.AI.LocalAIAPIKey, cfg.AI.LocalAIModelName, cfg.AI.LocalAINumCtx)
	embeddingSvc := service.NewEmbeddingService(localAIProvider, cfg.AI.LocalAIEmbeddingModel)

	// ── Emails ─────────────────────────────────────────────────────────────────
	emailRepo := repository.NewEmailRepo(pool)
	emailSvc := service.NewEmailService(emailRepo)
	emailSvc.WithBilling(billingRepo, userRepo)
	emailHandler := handler.NewEmailHandler(emailSvc, sessionMasterStore)
	emailHandler.RegisterRoutes(r)

	// ── Shared: documents / sensitive / private store / RAM master key ───────
	documentRepo := repository.NewDocumentRepo(pool)
	documentSvc := service.NewDocumentService(documentRepo, pool, cfg.Crypto.KeyringPepper)
	sensitiveSvc := service.NewSensitiveService(documentRepo, pool, cfg.Crypto.KeyringPepper)

	privateStoreRepo := repository.NewPrivateStoreRepo(pool)
	privateStoreSvc := service.NewPrivateStoreService(privateStoreRepo, pool, cfg.Crypto.KeyringPepper)

	// ── Import dialog saved settings (private_store, RAM master key) ────────────
	importDialogSettingsHandler := handler.NewImportDialogSettingsHandler(privateStoreSvc, sessionMasterStore, authSvc)
	importDialogSettingsHandler.RegisterRoutes(r)

	// ── IMAP ──────────────────────────────────────────────────────────────────
	imapHandler := handler.NewIMAPHandler(pool, sessionMasterStore, sensitiveSvc, authSvc)
	imapHandler.RegisterRoutes(r)

	// ── Gmail ─────────────────────────────────────────────────────────────────
	gmailHandler := handler.NewGmailHandler(pool,
		cfg.Gmail.ClientID, cfg.Gmail.ClientSecret, cfg.Gmail.RedirectURL, sessionMasterStore, sensitiveSvc, authSvc)
	gmailHandler.RegisterRoutes(r)

	// ── Images & media ─────────────────────────────────────────────────────────
	imageRepo := repository.NewImageRepo(pool)
	tagEmbedHelper := service.NewMediaTagEmbeddingHelper(pool, imageRepo, embeddingSvc)
	imageSvc := service.NewImageService(imageRepo, tagEmbedHelper)
	imageHandler := handler.NewImageHandler(imageSvc, sessionMasterStore, pool, embeddingSvc)
	imageHandler.RegisterRoutes(r)

	// ── Messages ────────────────────────────────────────────────────────────────
	messageRepo := repository.NewMessageRepo(pool)
	messageSvc := service.NewMessageService(messageRepo)
	messageSvc.WithBilling(billingRepo, userRepo)
	messageHandler := handler.NewMessageHandler(messageSvc, sessionMasterStore)
	messageHandler.RegisterRoutes(r)

	// ── Dashboard & subject configuration ────────────────────────────────────
	dashboardRepo := repository.NewDashboardRepo(pool)
	subjectConfigRepo := repository.NewSubjectConfigRepo(pool)
	appInstrRepo := repository.NewAppSystemInstructionsRepo(pool)
	contactRepo := repository.NewContactRepo(pool)
	dashboardSvc := service.NewDashboardService(dashboardRepo, subjectConfigRepo)
	subjectConfigSvc := service.NewSubjectConfigService(subjectConfigRepo, appInstrRepo, contactRepo)

	// ── Auth HTTP endpoints ────────────────────────────────────────────────────
	authHandler := handler.NewAuthHandler(authSvc, sensitiveSvc, subjectConfigSvc, sessionMasterStore, cfg.Server.SessionCookieSecure)
	authHandler.RegisterRoutes(r)

	dashboardHandler := handler.NewDashboardHandler(dashboardSvc, subjectConfigSvc, sessionMasterStore)
	dashboardHandler.RegisterRoutes(r)

	// ── Templated endpoints (GET /, suggestions, JS files) ───────────────────
	templateHandler := handler.NewTemplateHandler(subjectConfigRepo, userRepo, cfg)
	templateHandler.RegisterRoutes(r)

	// ── Static files (must be after template routes so /static/js/museum/foundation.js
	// and modals-people.js are served via TemplateHandler with {{ }} substitution)
	staticFS := http.FileServer(http.Dir(cfg.App.AssetStaticDir))
	r.Handle("/static/*", http.StripPrefix("/static/", staticFS))

	// ── Reference documents & sensitive data (shared keyring) ────────────────
	sensitiveHandler := handler.NewSensitiveHandler(sensitiveSvc, cfg.App.AssetStaticDir, sessionMasterStore)
	sensitiveHandler.RegisterRoutes(r)
	documentHandler := handler.NewDocumentHandler(documentSvc, sensitiveSvc, authSvc, sessionMasterStore)
	documentHandler.RegisterRoutes(r)

	sessionHandler := handler.NewSessionHandler(sensitiveSvc, sessionMasterStore, authSvc)
	sessionHandler.RegisterRoutes(r)

	// ── Private key-value store (master-only DEK) ─────────────────────────────
	privateStoreHandler := handler.NewPrivateStoreHandler(privateStoreSvc, sessionMasterStore)
	privateStoreHandler.RegisterRoutes(r)

	// ── Artefacts ─────────────────────────────────────────────────────────────
	artefactRepo := repository.NewArtefactRepo(pool)
	artefactSvc := service.NewArtefactService(artefactRepo)
	artefactHandler := handler.NewArtefactHandler(artefactSvc, sensitiveSvc, authSvc, sessionMasterStore)
	artefactHandler.RegisterRoutes(r)

	// ── Voices ────────────────────────────────────────────────────────────────
	voiceRepo := repository.NewVoiceRepo(pool)
	voiceSvc := service.NewVoiceService(voiceRepo, subjectConfigRepo, cfg.App.AssetStaticDir)
	voiceHandler := handler.NewVoiceHandler(voiceSvc, sessionMasterStore)
	voiceHandler.RegisterRoutes(r)

	// ── Interests ─────────────────────────────────────────────────────────────
	interestRepo := repository.NewInterestRepo(pool)
	interestSvc := service.NewInterestService(interestRepo)
	interestHandler := handler.NewInterestHandler(interestSvc, sessionMasterStore)
	interestHandler.RegisterRoutes(r)

	// ── Saved responses ───────────────────────────────────────────────────────
	savedResponseRepo := repository.NewSavedResponseRepo(pool)
	savedResponseSvc := service.NewSavedResponseService(savedResponseRepo)
	savedResponseHandler := handler.NewSavedResponseHandler(savedResponseSvc, sessionMasterStore)
	savedResponseHandler.RegisterRoutes(r)

	// ── Configuration ─────────────────────────────────────────────────────────
	configRepo := repository.NewConfigRepo(pool)
	configSvc := service.NewConfigService(configRepo)
	configHandler := handler.NewConfigHandler(configSvc, sessionMasterStore)
	configHandler.RegisterRoutes(r)

	// ── Contacts, email-matches, exclusions, classifications ──────────────────
	contactSvc := service.NewContactService(contactRepo, subjectConfigRepo)
	contactHandler := handler.NewContactHandler(contactSvc, sessionMasterStore, pool)
	contactHandler.RegisterRoutes(r)

	// ── Attachments ───────────────────────────────────────────────────────────
	attachmentRepo := repository.NewAttachmentRepo(pool)
	attachmentSvc := service.NewAttachmentService(attachmentRepo)
	attachmentHandler := handler.NewAttachmentHandler(attachmentSvc, cfg.App.AssetStaticDir, sessionMasterStore, sensitiveSvc, authSvc)
	attachmentHandler.RegisterRoutes(r)

	// ── Import jobs ───────────────────────────────────────────────────────────
	importerHandler := handler.NewImporterHandler(handler.ImporterHandlerDeps{
		ExcludePatterns:   cfg.Filesystem.ExcludePatterns,
		ImageRepo:         imageRepo,
		Pool:              pool,
		SubjectConfigRepo: subjectConfigRepo,
		SessionStore:      sessionMasterStore,
		SensitiveSvc:      sensitiveSvc,
		AuthSvc:           authSvc,
		EmbeddingSvc:      embeddingSvc,
	})
	importerHandler.RegisterRoutes(r)

	// ── Upload-based imports (Tier B ZIP upload, Tier C1 photo batch) ─────────
	uploadImportHandler := handler.NewUploadImportHandler(pool, subjectConfigRepo, sessionMasterStore, sensitiveSvc, authSvc, cfg.Upload)
	if err := uploadImportHandler.RegisterRoutes(r); err != nil {
		return nil, nil, err
	}

	embeddingHandler := handler.NewEmbeddingHandler(embeddingSvc)
	embeddingHandler.RegisterRoutes(r)
	messageSimilarityHandler := handler.NewMessageSimilarityHandler(pool, embeddingSvc)
	messageSimilarityHandler.RegisterRoutes(r)

	// ── Chat & AI ────────────────────────────────────────────────────────────
	geminiProvider := appai.NewGeminiProvider(cfg.AI.GeminiAPIKey, cfg.AI.GeminiModelName)
	emailSvc.WithGemini(geminiProvider)
	messageSvc.WithGemini(geminiProvider)

	// ── Admin & AI summarization ───────────────────────────────────────────────
	adminHandler := handler.NewAdminHandler(pool, subjectConfigRepo, contactRepo, sessionMasterStore)
	adminHandler.WithGemini(geminiProvider)
	adminHandler.WithBilling(billingRepo, userRepo)
	adminHandler.WithInterestService(interestSvc)
	adminHandler.RegisterRoutes(r)

	importDataPurgeHandler := handler.NewImportDataPurgeHandler(pool, sessionMasterStore, sensitiveSvc, authSvc)
	importDataPurgeHandler.RegisterRoutes(r)

	// ── Admin user management ──────────────────────────────────────────────────
	adminUsersHandler := handler.NewAdminUsersHandler(userRepo, authSvc, sensitiveSvc, subjectConfigSvc, dashboardSvc, billingRepo, appInstrRepo, cfg.Server.SessionCookieSecure)
	adminUsersHandler.RegisterRoutes(r)

	// ── Archive profiles (billing DB) ─────────────────────────────────────────
	archiveProvision := service.NewArchiveProvisionService(
		cfg.Server.SessionCookieSecure,
		cfg.Server.AdminEmail,
		cfg.Server.AdminPassword,
		cfg.Crypto.KeyringPepper,
		sensitiveSvc,
	)
	profileHandler := handler.NewProfileHandler(profileRepo, adminUsersHandler.RequireAdmin, archiveProvision)
	profileHandler.RegisterRoutes(r)

	billingExportHandler := handler.NewBillingExportHandler(userRepo, billingRepo)
	billingExportHandler.RegisterRoutes(r)
	chatRepo := repository.NewChatRepo(pool)
	completeProfileRepo := repository.NewCompleteProfileRepo(pool)
	chatSvc := service.NewChatService(
		chatRepo,
		subjectConfigRepo,
		appInstrRepo,
		completeProfileRepo,
		documentRepo,
		pool,
		userRepo,
		cfg.AI.GeminiAPIKey,
		cfg.AI.GeminiModelName,
		cfg.AI.AnthropicAPIKey,
		cfg.AI.ClaudeModelName,
		cfg.AI.DeepSeekAPIKey,
		cfg.AI.DeepSeekModelName,
		cfg.AI.TavilyAPIKey,
		cfg.AI.LocalAIBaseURL,
		cfg.AI.LocalAIAPIKey,
		cfg.AI.LocalAIModelName,
		cfg.AI.LocalAINumCtx,
		cfg.App.AssetStaticDir,
		cfg.Crypto.KeyringPepper,
		sessionMasterStore,
		privateStoreSvc,
		billingRepo,
	)
	chatHandler := handler.NewChatHandler(chatSvc, completeProfileRepo, sessionMasterStore)
	chatHandler.RegisterRoutes(r)

	haveAChatSessionRepo := repository.NewHaveAChatSessionRepo(pool)
	haveAChatHandler := handler.NewHaveAChatHandler(chatSvc, sessionMasterStore, haveAChatSessionRepo)
	haveAChatHandler.RegisterRoutes(r)

	// ── Identity Profile Wizard ───────────────────────────────────────────────
	identityProfileHandler := handler.NewIdentityProfileHandler(
		chatSvc, documentSvc, documentRepo, subjectConfigSvc, sessionMasterStore,
	)
	identityProfileHandler.RegisterRoutes(r)

	interviewRepo := repository.NewInterviewRepo(pool)
	interviewHandler := handler.NewInterviewHandler(chatSvc, interviewRepo, sessionMasterStore)
	interviewHandler.RegisterRoutes(r)

	llmToolsAccessHandler := handler.NewLLMToolsAccessHandler(privateStoreSvc, sessionMasterStore, authSvc)
	llmToolsAccessHandler.RegisterRoutes(r)

	// ── Background jobs scheduler (per-user maintenance jobs) ─────────────────
	backgroundJobsRepo := repository.NewBackgroundJobRepo(pool)
	backgroundJobsRunner := handler.NewBackgroundJobsRunner(pool, imageSvc, embeddingSvc)
	backgroundJobsScheduler := backgroundjobs.NewScheduler(backgroundJobsRepo, backgroundJobsRunner, 0)
	backgroundJobsHandler := handler.NewBackgroundJobsHandler(backgroundJobsRepo, backgroundJobsRunner, backgroundJobsScheduler)
	backgroundJobsHandler.RegisterRoutes(r)

	// ── Pam Bot (dementia companion) ─────────────────────────────────────────
	pamBotRepo := repository.NewPamBotRepo(pool)
	pamBotSvc := service.NewPamBotService(
		pamBotRepo, subjectConfigRepo, appInstrRepo, pool, userRepo,
		cfg.AI.GeminiAPIKey, cfg.AI.GeminiModelName,
		cfg.AI.AnthropicAPIKey, cfg.AI.ClaudeModelName,
		cfg.AI.TavilyAPIKey, cfg.Crypto.KeyringPepper,
		sessionMasterStore, billingRepo,
	)
	pamBotHandler := handler.NewPamBotHandler(pamBotSvc)
	pamBotHandler.RegisterRoutes(r)

	// ── Share tokens ──────────────────────────────────────────────────────────
	shareRepo := repository.NewArchiveShareRepo(pool)
	shareSvc := service.NewArchiveShareService(shareRepo, authSvc)
	shareHandler := handler.NewShareHandler(shareSvc, cfg.Server.SessionCookieSecure, sessionMasterStore)
	shareHandler.RegisterRoutes(r)

	// ── Visitor access (unauthenticated discovery + key login) ────────────────
	visitorSvc := service.NewVisitorService(userRepo, subjectConfigRepo, sensitiveSvc, pool, cfg.Crypto.KeyringPepper)
	visitorHandler := handler.NewVisitorHandler(visitorSvc, authSvc, sessionMasterStore, cfg.Server.SessionCookieSecure)
	visitorHandler.RegisterRoutes(r)

	return r, backgroundJobsScheduler, nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
