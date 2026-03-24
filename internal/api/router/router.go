// Package router wires all HTTP routes onto a chi.Mux.
package router

import (
	"encoding/json"
	"net/http"

	appai "github.com/daveontour/digitalmuseum/internal/ai"
	"github.com/daveontour/digitalmuseum/internal/config"
	"github.com/daveontour/digitalmuseum/internal/handler"
	"github.com/daveontour/digitalmuseum/internal/keystore"
	"github.com/daveontour/digitalmuseum/internal/middleware"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"github.com/daveontour/digitalmuseum/internal/service"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New returns the fully-wired application router.
func New(pool *pgxpool.Pool, cfg *config.Config) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ───────────────────────────────────────────────────────
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)

	// ── Health check ───────────────────────────────────────────────────────────
	r.Get("/health", healthHandler)

	// ── Static files ───────────────────────────────────────────────────────────
	// Serve /static/* from PYTHON_STATIC_DIR (../src/api/static by default).
	// This covers css/, images/, js/ and any other assets the frontend requests.
	fs := http.FileServer(http.Dir(cfg.App.AssetStaticDir))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	// ── Emails ─────────────────────────────────────────────────────────────────
	emailRepo := repository.NewEmailRepo(pool)
	emailSvc := service.NewEmailService(emailRepo)
	emailHandler := handler.NewEmailHandler(emailSvc)
	emailHandler.RegisterRoutes(r)

	// ── Shared: documents / sensitive / private store / RAM master key ───────
	documentRepo := repository.NewDocumentRepo(pool)
	documentSvc := service.NewDocumentService(documentRepo, pool, cfg.Crypto.KeyringPepper)
	sensitiveSvc := service.NewSensitiveService(documentRepo, pool, cfg.Crypto.KeyringPepper)
	sessionMasterStore := keystore.NewSessionMasterStore(cfg.Server.SessionCookieSecure)
	privateStoreRepo := repository.NewPrivateStoreRepo(pool)
	privateStoreSvc := service.NewPrivateStoreService(privateStoreRepo, pool, cfg.Crypto.KeyringPepper)

	// ── Import dialog saved settings (private_store, RAM master key) ────────────
	importDialogSettingsHandler := handler.NewImportDialogSettingsHandler(privateStoreSvc, sessionMasterStore)
	importDialogSettingsHandler.RegisterRoutes(r)

	// ── IMAP ──────────────────────────────────────────────────────────────────
	imapHandler := handler.NewIMAPHandler(pool)
	imapHandler.RegisterRoutes(r)

	// ── Gmail ─────────────────────────────────────────────────────────────────
	gmailHandler := handler.NewGmailHandler(pool,
		cfg.Gmail.ClientID, cfg.Gmail.ClientSecret, cfg.Gmail.RedirectURL)
	gmailHandler.RegisterRoutes(r)

	// ── Images & media ─────────────────────────────────────────────────────────
	imageRepo := repository.NewImageRepo(pool)
	imageSvc := service.NewImageService(imageRepo)
	imageHandler := handler.NewImageHandler(imageSvc)
	imageHandler.RegisterRoutes(r)

	// ── Messages ────────────────────────────────────────────────────────────────
	messageRepo := repository.NewMessageRepo(pool)
	messageSvc := service.NewMessageService(messageRepo)
	messageHandler := handler.NewMessageHandler(messageSvc)
	messageHandler.RegisterRoutes(r)

	// ── Dashboard & subject configuration ────────────────────────────────────
	dashboardRepo := repository.NewDashboardRepo(pool)
	subjectConfigRepo := repository.NewSubjectConfigRepo(pool)
	dashboardSvc := service.NewDashboardService(dashboardRepo, subjectConfigRepo)
	subjectConfigSvc := service.NewSubjectConfigService(subjectConfigRepo)
	dashboardHandler := handler.NewDashboardHandler(dashboardSvc, subjectConfigSvc)
	dashboardHandler.RegisterRoutes(r)

	// ── Templated endpoints (GET /, suggestions, JS files) ───────────────────
	templateHandler := handler.NewTemplateHandler(subjectConfigRepo, cfg, sessionMasterStore)
	templateHandler.RegisterRoutes(r)

	// ── Reference documents & sensitive data (shared keyring) ────────────────
	sensitiveHandler := handler.NewSensitiveHandler(sensitiveSvc, cfg.App.AssetStaticDir, sessionMasterStore)
	sensitiveHandler.RegisterRoutes(r)
	documentHandler := handler.NewDocumentHandler(documentSvc, sensitiveSvc, sessionMasterStore)
	documentHandler.RegisterRoutes(r)

	sessionHandler := handler.NewSessionHandler(sensitiveSvc, sessionMasterStore)
	sessionHandler.RegisterRoutes(r)

	// ── Private key-value store (master-only DEK) ─────────────────────────────
	privateStoreHandler := handler.NewPrivateStoreHandler(privateStoreSvc, sessionMasterStore)
	privateStoreHandler.RegisterRoutes(r)

	// ── Artefacts ─────────────────────────────────────────────────────────────
	artefactRepo := repository.NewArtefactRepo(pool)
	artefactSvc := service.NewArtefactService(artefactRepo)
	artefactHandler := handler.NewArtefactHandler(artefactSvc)
	artefactHandler.RegisterRoutes(r)

	// ── Voices ────────────────────────────────────────────────────────────────
	voiceRepo := repository.NewVoiceRepo(pool)
	voiceSvc := service.NewVoiceService(voiceRepo, subjectConfigRepo, cfg.App.AssetStaticDir)
	voiceHandler := handler.NewVoiceHandler(voiceSvc)
	voiceHandler.RegisterRoutes(r)

	// ── Interests ─────────────────────────────────────────────────────────────
	interestRepo := repository.NewInterestRepo(pool)
	interestSvc := service.NewInterestService(interestRepo)
	interestHandler := handler.NewInterestHandler(interestSvc)
	interestHandler.RegisterRoutes(r)

	// ── Saved responses ───────────────────────────────────────────────────────
	savedResponseRepo := repository.NewSavedResponseRepo(pool)
	savedResponseSvc := service.NewSavedResponseService(savedResponseRepo)
	savedResponseHandler := handler.NewSavedResponseHandler(savedResponseSvc)
	savedResponseHandler.RegisterRoutes(r)

	// ── Configuration ─────────────────────────────────────────────────────────
	configRepo := repository.NewConfigRepo(pool)
	configSvc := service.NewConfigService(configRepo)
	configHandler := handler.NewConfigHandler(configSvc)
	configHandler.RegisterRoutes(r)

	// ── Contacts, email-matches, exclusions, classifications ──────────────────
	contactRepo := repository.NewContactRepo(pool)
	contactSvc := service.NewContactService(contactRepo)
	contactHandler := handler.NewContactHandler(contactSvc)
	contactHandler.RegisterRoutes(r)

	// ── Attachments ───────────────────────────────────────────────────────────
	attachmentRepo := repository.NewAttachmentRepo(pool)
	attachmentSvc := service.NewAttachmentService(attachmentRepo)
	attachmentHandler := handler.NewAttachmentHandler(attachmentSvc, cfg.App.AssetStaticDir)
	attachmentHandler.RegisterRoutes(r)

	// ── Import jobs ───────────────────────────────────────────────────────────
	importerHandler := handler.NewImporterHandler(handler.ImporterHandlerDeps{
		ExcludePatterns:   cfg.Filesystem.ExcludePatterns,
		ImageRepo:         imageRepo,
		Pool:              pool,
		SubjectConfigRepo: subjectConfigRepo,
	})
	importerHandler.RegisterRoutes(r)

	// ── Chat & AI ────────────────────────────────────────────────────────────
	geminiProvider := appai.NewGeminiProvider(cfg.AI.GeminiAPIKey, cfg.AI.GeminiModelName)
	emailSvc.WithGemini(geminiProvider)
	messageSvc.WithGemini(geminiProvider)

	// ── Admin & AI summarization ───────────────────────────────────────────────
	adminHandler := handler.NewAdminHandler(pool, subjectConfigRepo)
	adminHandler.WithGemini(geminiProvider)
	adminHandler.RegisterRoutes(r)
	claudeProvider := appai.NewClaudeProvider(cfg.AI.AnthropicAPIKey, cfg.AI.ClaudeModelName)
	chatRepo := repository.NewChatRepo(pool)
	completeProfileRepo := repository.NewCompleteProfileRepo(pool)
	chatSvc := service.NewChatService(
		chatRepo,
		subjectConfigRepo,
		completeProfileRepo,
		pool,
		geminiProvider,
		claudeProvider,
		cfg.App.AssetStaticDir,
		cfg.AI.TavilyAPIKey,
		cfg.Crypto.KeyringPepper,
		sessionMasterStore,
		privateStoreSvc,
	)
	chatHandler := handler.NewChatHandler(chatSvc, completeProfileRepo, sessionMasterStore)
	chatHandler.RegisterRoutes(r)

	haveAChatHandler := handler.NewHaveAChatHandler(chatSvc, sessionMasterStore)
	haveAChatHandler.RegisterRoutes(r)

	llmToolsAccessHandler := handler.NewLLMToolsAccessHandler(privateStoreSvc, sessionMasterStore)
	llmToolsAccessHandler.RegisterRoutes(r)

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
