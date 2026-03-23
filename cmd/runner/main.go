// runner is a CLI application that runs import jobs without the web server.
// Jobs are configured via a JSON file. Jobs within a stage run in parallel;
// stages run sequentially.
//
// Usage:
//
//	runner -config runner.json
//
// Database connection is read from the .env file (DB_HOST, DB_PORT, DB_NAME,
// DB_USER, DB_PASSWORD, KEYRING_PEPPER).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/daveontour/digitalmuseum/internal/config"
	"github.com/daveontour/digitalmuseum/internal/database"
	contactsimport "github.com/daveontour/digitalmuseum/internal/import/contacts"
	facebookimport "github.com/daveontour/digitalmuseum/internal/import/facebook"
	facebookalbumsimport "github.com/daveontour/digitalmuseum/internal/import/facebookalbums"
	facebookallimport "github.com/daveontour/digitalmuseum/internal/import/facebookall"
	facebookplacesimport "github.com/daveontour/digitalmuseum/internal/import/facebookplaces"
	facebookpostsimport "github.com/daveontour/digitalmuseum/internal/import/facebookposts"
	filesystemimport "github.com/daveontour/digitalmuseum/internal/import/filesystem"
	imapimport "github.com/daveontour/digitalmuseum/internal/import/imap"
	imessageimport "github.com/daveontour/digitalmuseum/internal/import/imessage"
	instagramimport "github.com/daveontour/digitalmuseum/internal/import/instagram"
	thumbnailsimport "github.com/daveontour/digitalmuseum/internal/import/thumbnails"
	whatsappimport "github.com/daveontour/digitalmuseum/internal/import/whatsapp"
	"github.com/daveontour/digitalmuseum/internal/importstorage"
	"github.com/daveontour/digitalmuseum/internal/repository"
	"mime"
	"path/filepath"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Config types ──────────────────────────────────────────────────────────────

// RunnerConfig is the top-level JSON configuration file structure.
type RunnerConfig struct {
	// MasterKey is the keyring master password. Optional — not required by
	// current import jobs but may be needed for private-store access in future.
	MasterKey string `json:"master_key"`

	// StopOnError causes the runner to abort remaining stages if any job in a
	// stage returns an error. Default: false (log errors and continue).
	StopOnError bool `json:"stop_on_error"`

	// Stages is an ordered list of parallel groups. Each inner array is one
	// stage; all jobs within a stage run concurrently. Stages run sequentially.
	Stages [][]JobConfig `json:"stages"`
}

// ReferenceFileEntry describes a single file to import into reference_documents.
type ReferenceFileEntry struct {
	Path        string `json:"path"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Tags        string `json:"tags"`
	Categories  string `json:"categories"`
}

// JobConfig holds configuration for a single import job.
// Only the fields relevant to the chosen Type need to be set.
type JobConfig struct {
	// Type selects which importer to run. Required.
	// Supported values: filesystem, thumbnails, whatsapp, imessage, instagram,
	// facebook_all, contacts_extract, reference_import, image_export, imap.
	Type string `json:"type"`

	// Execute controls whether this job is run. Set to false to skip it.
	Execute bool `json:"execute"`

	// ── Filesystem ──────────────────────────────────────────────────────────
	// Directories is the list of root directories to scan (semicolons also
	// accepted as separator for compatibility with the web UI).
	Directories []string `json:"directories"`
	// MaxImages limits the number of images imported. Omit or set 0 for no limit.
	MaxImages *int `json:"max_images"`
	// ReferenceMode stores images as references (path only) instead of embedding
	// the binary data in the database.
	ReferenceMode bool `json:"reference_mode"`

	// ── Thumbnails ──────────────────────────────────────────────────────────
	// Reprocess forces re-generation of already-processed thumbnails.
	Reprocess bool `json:"reprocess"`

	// ── Messaging (WhatsApp / iMessage / Instagram / Facebook All) ──────────
	// Directory is the root export directory for the importer.
	Directory string `json:"directory"`
	// UserName is the account username (used by Instagram and Facebook All).
	UserName string `json:"user_name"`
	// ExportRoot overrides the export root path (used by Instagram).
	ExportRoot string `json:"export_root"`

	// ── Image export ────────────────────────────────────────────────────────
	// TargetDirectory is where exported images are written.
	TargetDirectory string `json:"target_directory"`

	// ── Reference files ─────────────────────────────────────────────────────
	// Files is the list of documents to import into reference_documents.
	Files []ReferenceFileEntry `json:"files"`

	// ── IMAP ────────────────────────────────────────────────────────────────
	IMAPHost       string   `json:"imap_host"`
	IMAPPort       int      `json:"imap_port"`
	IMAPUsername   string   `json:"imap_username"`
	IMAPPassword   string   `json:"imap_password"`
	IMAPUseSSL     bool     `json:"imap_use_ssl"`
	IMAPFolders        []string `json:"imap_folders"`
	IMAPAllFolders     bool     `json:"imap_all_folders"`
	IMAPExcludeFolders []string `json:"imap_exclude_folders"`
	IMAPNewOnly        bool     `json:"imap_new_only"`
}

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

const jobHelp = `
Supported job types
===================

All job types share this common field:
  "execute"        bool      Required. Set to true to run this job; false to skip it.

filesystem
  Scan directories for images and import them into the database.
  "directories"    []string  Required. List of root directories to scan.
  "max_images"     int|null  Optional. Maximum number of images to import (omit or null = no limit).
  "reference_mode" bool      Optional. Store file path only instead of embedding image data (default: false).

thumbnails
  Generate/update thumbnails and read EXIF data for all images.
  "reprocess"      bool      Optional. Re-generate already-processed thumbnails (default: false).

whatsapp
  Import WhatsApp conversations from an exported directory.
  "directory"      string    Required. Path to the WhatsApp export root.

imessage
  Import iMessage conversations from an exported directory.
  "directory"      string    Required. Path to the iMessage export root.

instagram
  Import Instagram direct messages from an exported directory.
  "directory"      string    Required. Path to the Instagram export root.
  "user_name"      string    Optional. Instagram username (helps resolve conversation participants).
  "export_root"    string    Optional. Override the export root for attachment resolution.

facebook_all
  Import all Facebook data (Messenger + Albums + Places + Posts) from one export.
  Sub-imports run in parallel within the job.
  "directory"      string    Required. Path to the Facebook export root.
  "user_name"      string    Optional. Facebook username (helps resolve conversation participants).

contacts_extract
  Extract and normalise contacts from all imported messages.
  No additional parameters.

reference_import
  Import images that were previously stored as file-path references into the database as binary data.
  No additional parameters.

reference_files
  Import a list of files (PDFs, Word docs, etc.) into the reference_documents table.
  Content type is detected automatically from the file extension.
  "files"  []object  Required. List of file entries, each with:
    "path"        string  Required. Absolute path to the file.
    "title"       string  Optional. Display title for the document.
    "description" string  Optional. Short description of the document.
    "author"      string  Optional. Author name.
    "tags"        string  Optional. Comma-separated tags.
    "categories"  string  Optional. Comma-separated categories.

image_export
  Export all images stored in the database to a directory on disk.
  "target_directory" string  Required. Directory to write exported images into.

imap
  Import emails from an IMAP mailbox.
  "imap_host"        string    Required. IMAP server hostname.
  "imap_port"        int       Required. IMAP server port (usually 993 for SSL, 143 for plain).
  "imap_username"    string    Required. IMAP account username / email address.
  "imap_password"    string    Required. IMAP account password.
  "imap_use_ssl"     bool      Optional. Connect with TLS (default: false).
  "imap_folders"         []string  Optional. Specific folders to import. Default: ["INBOX"].
  "imap_all_folders"     bool      Optional. Import every folder on the server (overrides imap_folders).
  "imap_exclude_folders" []string  Optional. Regex patterns — any folder whose name matches is skipped.
                                   Applied after imap_all_folders or imap_folders is resolved.
                                   Example: ["^Spam$", "^Trash", "Junk"]
  "imap_new_only"        bool      Optional. Skip messages already present in the database (default: false).

Top-level config fields
=======================
  "master_key"    string  Optional. Keyring master password (not required by current import jobs).
  "stop_on_error" bool    Optional. Abort remaining stages on error (default: false).
  "stages"        array   Required. Ordered list of stages. Each stage is an array of job configs.
                          Jobs within a stage run in parallel; stages run sequentially.

Example
=======
  runner -config runner.json
  runner -config C:\path\to\myjobs.json
`

func printHelp() {
	fmt.Print(`Usage: runner [flags]

Flags:
  -config string   Path to the runner JSON config file (default "runner.json")
  -jobs            List all supported job types and their parameters
  -h, -help        Show this help message
`)
	fmt.Println(jobHelp)
}

func run() error {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	configPath := flag.String("config", "runner.json", "path to runner JSON config file")
	showJobs := flag.Bool("jobs", false, "list all supported job types and their parameters")
	flag.Usage = printHelp
	flag.Parse()

	if *showJobs {
		fmt.Println(jobHelp)
		return nil
	}

	// Load .env + environment variables (uses the same logic as the web server).
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Parse the runner JSON config.
	data, err := os.ReadFile(*configPath)
	if err != nil {
		return fmt.Errorf("read config %s: %w", *configPath, err)
	}
	var runnerCfg RunnerConfig
	if err := json.Unmarshal(data, &runnerCfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if len(runnerCfg.Stages) == 0 {
		slog.Info("no stages defined — nothing to do")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Ensure the database exists (creates it if missing).
	slog.Info("checking database", "host", cfg.DB.Host, "port", cfg.DB.Port, "name", cfg.DB.Name)
	ensureCtx, ensureCancel := context.WithTimeout(ctx, 30*time.Second)
	defer ensureCancel()
	if err := database.EnsureDatabase(ensureCtx, cfg.DB); err != nil {
		return fmt.Errorf("ensure database: %w", err)
	}

	// Connect to the database.
	connCtx, connCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connCancel()
	db, err := database.New(connCtx, cfg.DB)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer db.Close()
	slog.Info("database connected")

	// Run migrations to create/update tables.
	migrateCtx, migrateCancel := context.WithTimeout(ctx, 60*time.Second)
	defer migrateCancel()
	if err := database.Migrate(migrateCtx, db.Pool); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("migrations complete")

	// Seed reference data (idempotent — skips rows that already exist).
	if err := database.SeedEmailExclusionsFromJSON(migrateCtx, db.Pool, "static/data/exclusions.json"); err != nil {
		return fmt.Errorf("seed email exclusions: %w", err)
	}
	if err := database.SeedEmailMatchesFromJSON(migrateCtx, db.Pool, "static/data/email_matches.json"); err != nil {
		return fmt.Errorf("seed email matches: %w", err)
	}
	if err := database.SeedEmailClassificationsFromJSON(migrateCtx, db.Pool, "static/data/email_classifications.json"); err != nil {
		return fmt.Errorf("seed email classifications: %w", err)
	}
	slog.Info("seed data ready")

	pool := db.Pool
	subjectRepo := repository.NewSubjectConfigRepo(pool)
	imageRepo := repository.NewImageRepo(pool)

	// Handle SIGINT / SIGTERM for graceful cancellation.
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Run stages sequentially.
	for i, stage := range runnerCfg.Stages {
		if sigCtx.Err() != nil {
			slog.Info("cancelled — stopping before stage", "stage", i+1)
			break
		}
		slog.Info("starting stage", "stage", i+1, "of", len(runnerCfg.Stages), "jobs", len(stage))

		errs := runStage(sigCtx, pool, subjectRepo, imageRepo, cfg, stage)
		for _, e := range errs {
			slog.Error("job error", "stage", i+1, "err", e)
		}
		if len(errs) > 0 && runnerCfg.StopOnError {
			return fmt.Errorf("stage %d had %d error(s) — stopping (stop_on_error=true)", i+1, len(errs))
		}
		slog.Info("stage complete", "stage", i+1)
	}

	slog.Info("all stages complete")
	return nil
}

// ── Stage runner ─────────────────────────────────────────────────────────────

// runStage executes all jobs in a stage concurrently and waits for them all.
func runStage(ctx context.Context, pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, imageRepo *repository.ImageRepo, cfg *config.Config, jobs []JobConfig) []error {
	if len(jobs) == 1 {
		if err := runJob(ctx, pool, subjectRepo, imageRepo, cfg, jobs[0]); err != nil {
			return []error{err}
		}
		return nil
	}

	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(job JobConfig) {
			defer wg.Done()
			if err := runJob(ctx, pool, subjectRepo, imageRepo, cfg, job); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("[%s] %w", job.Type, err))
				mu.Unlock()
			}
		}(j)
	}
	wg.Wait()
	return errs
}

// runJob dispatches to the appropriate import function for the job type.
func runJob(ctx context.Context, pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, imageRepo *repository.ImageRepo, cfg *config.Config, job JobConfig) error {
	if !job.Execute {
		slog.Info("job skipped (execute=false)", "type", job.Type)
		return nil
	}
	slog.Info("job starting", "type", job.Type)
	var err error
	switch job.Type {
	case "filesystem":
		err = runFilesystem(ctx, pool, cfg, job)
	case "thumbnails":
		err = runThumbnails(ctx, pool, job)
	case "whatsapp":
		err = runWhatsApp(ctx, pool, subjectRepo, job)
	case "imessage":
		err = runIMessage(ctx, pool, subjectRepo, job)
	case "instagram":
		err = runInstagram(ctx, pool, subjectRepo, job)
	case "facebook_all":
		err = runFacebookAll(ctx, pool, subjectRepo, job)
	case "contacts_extract":
		err = runContactsExtract(ctx, pool)
	case "reference_import":
		err = runReferenceImport(ctx, imageRepo)
	case "reference_files":
		err = runReferenceFiles(ctx, pool, job)
	case "image_export":
		err = runImageExport(ctx, imageRepo, job)
	case "imap":
		err = runIMAP(ctx, pool, job)
	default:
		err = fmt.Errorf("unknown job type %q", job.Type)
	}
	if err != nil {
		slog.Error("job failed", "type", job.Type, "err", err)
		return err
	}
	slog.Info("job complete", "type", job.Type)
	return nil
}

// ── Individual job runners ────────────────────────────────────────────────────

func runFilesystem(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, job JobConfig) error {
	storage := importstorage.NewImageStorage(pool)
	cancelFn := func() bool { return ctx.Err() != nil }
	maxImages := job.MaxImages
	if maxImages != nil && *maxImages <= 0 {
		maxImages = nil
	}

	progressCallback := func(stats filesystemimport.ImportStats) {
		slog.Info("filesystem",
			"processed", stats.FilesProcessed,
			"total", stats.TotalFiles,
			"imported", stats.ImagesImported,
			"referenced", stats.ImagesReferenced,
			"updated", stats.ImagesUpdated,
			"errors", stats.Errors,
		)
	}

	stats, err := filesystemimport.ImportImagesFromDirectories(
		ctx, storage, job.Directories, cfg.Filesystem.ExcludePatterns,
		maxImages, job.ReferenceMode, progressCallback, cancelFn,
	)
	if err != nil {
		return err
	}
	slog.Info("filesystem complete",
		"imported", stats.ImagesImported,
		"referenced", stats.ImagesReferenced,
		"updated", stats.ImagesUpdated,
		"errors", stats.Errors,
	)
	return nil
}

func runThumbnails(ctx context.Context, pool *pgxpool.Pool, job JobConfig) error {
	// Region updates before (matches web server behaviour).
	_, _ = pool.Exec(ctx, "SELECT update_location_regions()")
	_, _ = pool.Exec(ctx, "SELECT update_image_location_regions()")

	cancelFn := func() bool { return ctx.Err() != nil }
	progressCallback := func(stats thumbnailsimport.ImportStats) {
		slog.Info("thumbnails",
			"processed", stats.Processed,
			"total", stats.TotalItems,
			"errors", stats.Errors,
		)
	}

	stats, err := thumbnailsimport.ProcessThumbnailsAndExif(ctx, pool, job.Reprocess, progressCallback, cancelFn)

	// Region updates after.
	_, _ = pool.Exec(ctx, "SELECT update_location_regions()")
	_, _ = pool.Exec(ctx, "SELECT update_image_location_regions()")

	if err != nil {
		return err
	}
	slog.Info("thumbnails complete", "processed", stats.Processed, "errors", stats.Errors)
	return nil
}

func runWhatsApp(ctx context.Context, pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job JobConfig) error {
	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)
	cancelFn := func() bool { return ctx.Err() != nil }
	progressCallback := func(stats whatsappimport.ImportStats) {
		slog.Info("whatsapp",
			"conversations", stats.ConversationsProcessed,
			"messages", stats.MessagesImported,
			"errors", stats.Errors,
		)
	}

	stats, err := whatsappimport.ImportWhatsAppFromDirectory(ctx, storage, job.Directory, progressCallback, cancelFn)
	if err != nil {
		return err
	}
	slog.Info("whatsapp complete",
		"conversations", stats.ConversationsProcessed,
		"messages", stats.MessagesImported,
		"attachments_found", stats.AttachmentsFound,
		"attachments_missing", stats.AttachmentsMissing,
	)
	return nil
}

func runIMessage(ctx context.Context, pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job JobConfig) error {
	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)
	cancelFn := func() bool { return ctx.Err() != nil }
	progressCallback := func(stats imessageimport.ImportStats) {
		slog.Info("imessage",
			"conversations", stats.ConversationsProcessed,
			"messages", stats.MessagesImported,
			"errors", stats.Errors,
		)
	}

	stats, err := imessageimport.ImportIMessagesFromDirectory(ctx, storage, job.Directory, progressCallback, cancelFn)
	if err != nil {
		return err
	}
	slog.Info("imessage complete",
		"conversations", stats.ConversationsProcessed,
		"messages", stats.MessagesImported,
		"attachments_found", stats.AttachmentsFound,
		"attachments_missing", stats.AttachmentsMissing,
	)
	return nil
}

func runInstagram(ctx context.Context, pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job JobConfig) error {
	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)
	cancelFn := func() bool { return ctx.Err() != nil }
	progressCallback := func(stats instagramimport.ImportStats) {
		slog.Info("instagram",
			"conversations", stats.ConversationsProcessed,
			"messages", stats.MessagesImported,
			"errors", stats.Errors,
		)
	}

	stats, err := instagramimport.ImportInstagramFromDirectory(
		ctx, storage, job.Directory, progressCallback, cancelFn, job.ExportRoot, job.UserName,
	)
	if err != nil {
		return err
	}
	slog.Info("instagram complete",
		"conversations", stats.ConversationsProcessed,
		"messages", stats.MessagesImported,
	)
	return nil
}

func runFacebookAll(ctx context.Context, pool *pgxpool.Pool, subjectRepo *repository.SubjectConfigRepo, job JobConfig) error {
	slog.Info("facebook_all clearing existing data")
	if err := facebookallimport.ClearFacebookAllData(ctx, pool); err != nil {
		return fmt.Errorf("clear Facebook data: %w", err)
	}

	storage := importstorage.NewMessageStorage(ctx, pool, subjectRepo)
	exportRoot := job.Directory
	cancelFn := func() bool { return ctx.Err() != nil }

	var wg sync.WaitGroup
	type result[S any] struct {
		stats *S
		err   error
	}
	messengerCh := make(chan result[facebookimport.ImportStats], 1)
	albumsCh := make(chan result[facebookalbumsimport.ImportStats], 1)
	placesCh := make(chan result[facebookplacesimport.ImportStats], 1)
	postsCh := make(chan result[facebookpostsimport.ImportStats], 1)

	wg.Add(1)
	go func() {
		defer wg.Done()
		progressFn := func(stats facebookimport.ImportStats) {
			slog.Info("facebook_all/messenger", "conversations", stats.ConversationsProcessed, "messages", stats.MessagesImported)
		}
		s, err := facebookimport.ImportFacebookFromDirectory(ctx, storage, job.Directory, progressFn, cancelFn, exportRoot, job.UserName)
		messengerCh <- result[facebookimport.ImportStats]{s, err}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		progressFn := func(stats facebookalbumsimport.ImportStats) {
			slog.Info("facebook_all/albums", "albums", stats.AlbumsProcessed, "images", stats.ImagesImported)
		}
		s, err := facebookalbumsimport.ImportFacebookAlbumsFromDirectory(ctx, pool, job.Directory, progressFn, cancelFn, exportRoot)
		albumsCh <- result[facebookalbumsimport.ImportStats]{s, err}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		progressFn := func(stats facebookplacesimport.ImportStats) {
			slog.Info("facebook_all/places", "imported", stats.PlacesImported)
		}
		s, err := facebookplacesimport.ImportFacebookPlacesFromDirectory(ctx, pool, job.Directory, progressFn, cancelFn)
		placesCh <- result[facebookplacesimport.ImportStats]{s, err}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		progressFn := func(stats facebookpostsimport.ImportStats) {
			slog.Info("facebook_all/posts", "processed", stats.PostsProcessed, "imported", stats.PostsImported)
		}
		s, err := facebookpostsimport.ImportFacebookPostsFromPath(ctx, pool, job.Directory, exportRoot, progressFn, cancelFn)
		postsCh <- result[facebookpostsimport.ImportStats]{s, err}
	}()

	wg.Wait()

	var errs []error
	if r := <-messengerCh; r.err != nil {
		errs = append(errs, fmt.Errorf("messenger: %w", r.err))
	} else if r.stats != nil {
		slog.Info("facebook_all/messenger complete", "conversations", r.stats.ConversationsProcessed, "messages", r.stats.MessagesImported)
	}
	if r := <-albumsCh; r.err != nil {
		errs = append(errs, fmt.Errorf("albums: %w", r.err))
	} else if r.stats != nil {
		slog.Info("facebook_all/albums complete", "albums", r.stats.AlbumsProcessed, "images", r.stats.ImagesImported)
	}
	if r := <-placesCh; r.err != nil {
		errs = append(errs, fmt.Errorf("places: %w", r.err))
	} else if r.stats != nil {
		slog.Info("facebook_all/places complete", "imported", r.stats.PlacesImported)
	}
	if r := <-postsCh; r.err != nil {
		errs = append(errs, fmt.Errorf("posts: %w", r.err))
	} else if r.stats != nil {
		slog.Info("facebook_all/posts complete", "processed", r.stats.PostsProcessed, "imported", r.stats.PostsImported)
	}

	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return fmt.Errorf("facebook_all sub-errors: %s", joinStrings(msgs, "; "))
	}
	return nil
}

func runContactsExtract(ctx context.Context, pool *pgxpool.Pool) error {
	progressFn := func(msg string) {
		slog.Info("contacts_extract", "status", msg)
	}
	opts := contactsimport.RunOptions{
		Workers:      runtime.NumCPU(),
		ContactsDB:   pool,
		ProgressFunc: progressFn,
	}
	if err := contactsimport.RunContactsNormalise(ctx, opts); err != nil {
		return err
	}
	slog.Info("contacts_extract complete")
	return nil
}

func runReferenceImport(ctx context.Context, imageRepo *repository.ImageRepo) error {
	items, err := imageRepo.ListReferencedItems(ctx)
	if err != nil {
		return fmt.Errorf("list referenced items: %w", err)
	}
	slog.Info("reference_import", "total", len(items))

	imported, skipped, errCount := 0, 0, 0
	for i, it := range items {
		if ctx.Err() != nil {
			break
		}
		data, err := os.ReadFile(it.SourceReference)
		if err != nil {
			skipped++
		} else if err := imageRepo.UpdateBlobImageDataAndClearReferenced(ctx, it.ID, it.MediaBlobID, data); err != nil {
			errCount++
			slog.Warn("reference_import error", "id", it.ID, "err", err)
		} else {
			imported++
		}
		if (i+1)%100 == 0 {
			slog.Info("reference_import progress", "processed", i+1, "total", len(items), "imported", imported)
		}
	}
	slog.Info("reference_import complete", "imported", imported, "skipped", skipped, "errors", errCount)
	return nil
}

func runReferenceFiles(ctx context.Context, pool *pgxpool.Pool, job JobConfig) error {
	if len(job.Files) == 0 {
		slog.Info("reference_files: no files defined, nothing to do")
		return nil
	}
	repo := repository.NewDocumentRepo(pool)
	imported, skipped, errCount := 0, 0, 0

	for _, f := range job.Files {
		if ctx.Err() != nil {
			break
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			slog.Warn("reference_files: cannot read file", "path", f.Path, "err", err)
			skipped++
			continue
		}

		// Detect content type from extension; fall back to octet-stream.
		ct := mime.TypeByExtension(filepath.Ext(f.Path))
		if ct == "" {
			ct = "application/octet-stream"
		}

		filename := filepath.Base(f.Path)
		title := strPtr(f.Title)
		description := strPtr(f.Description)
		author := strPtr(f.Author)
		tags := strPtr(f.Tags)
		categories := strPtr(f.Categories)

		if _, err := repo.Create(ctx,
			filename, ct, int64(len(data)), data,
			title, description, author, tags, categories, nil,
			false, false, false, false,
		); err != nil {
			slog.Error("reference_files: failed to import", "path", f.Path, "err", err)
			errCount++
			continue
		}
		slog.Info("reference_files: imported", "file", filename, "title", f.Title)
		imported++
	}

	slog.Info("reference_files complete", "imported", imported, "skipped", skipped, "errors", errCount)
	if errCount > 0 {
		return fmt.Errorf("%d file(s) failed to import", errCount)
	}
	return nil
}

// strPtr returns nil if s is empty, otherwise a pointer to s.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func runImageExport(ctx context.Context, imageRepo *repository.ImageRepo, job JobConfig) error {
	if err := os.MkdirAll(job.TargetDirectory, 0755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	items, err := imageRepo.ListMediaItemsForExport(ctx)
	if err != nil {
		return fmt.Errorf("list media items: %w", err)
	}
	slog.Info("image_export", "total", len(items))

	exported, skipped, errCount := 0, 0, 0
	subdirCount := 0
	const maxPerDir = 200

	for i, it := range items {
		if ctx.Err() != nil {
			break
		}
		data, err := imageRepo.GetBlobImageData(ctx, it.MediaBlobID)
		if err != nil || len(data) == 0 {
			skipped++
			continue
		}
		subIdx := i / maxPerDir
		subdir := fmt.Sprintf("%s/%04d", job.TargetDirectory, subIdx)
		if subIdx > subdirCount {
			_ = os.MkdirAll(subdir, 0755)
			subdirCount = subIdx
		}
		ext := extForMediaType(it.MediaType)
		filename := fmt.Sprintf("%d.%s", it.ID, ext)
		if it.SourceRef != nil && *it.SourceRef != "" {
			if e := pathExt(*it.SourceRef); e != "" {
				filename = fmt.Sprintf("%d%s", it.ID, e)
			}
		}
		if err := os.WriteFile(subdir+"/"+filename, data, 0644); err != nil {
			errCount++
		} else {
			exported++
		}
		if (i+1)%100 == 0 {
			slog.Info("image_export progress", "processed", i+1, "total", len(items))
		}
	}
	slog.Info("image_export complete", "exported", exported, "skipped", skipped, "errors", errCount)
	return nil
}

func runIMAP(ctx context.Context, pool *pgxpool.Pool, job JobConfig) error {
	params := imapimport.ConnParams{
		Host:     job.IMAPHost,
		Port:     job.IMAPPort,
		Username: job.IMAPUsername,
		Password: job.IMAPPassword,
		UseSSL:   job.IMAPUseSSL,
	}

	// Resolve folder list.
	folders, err := imapimport.ListFolders(params)
	if err != nil {
		return fmt.Errorf("list IMAP folders: %w", err)
	}
	if job.IMAPAllFolders {
		// use all folders from server, then apply exclusions below
	} else if len(job.IMAPFolders) > 0 {
		folders = job.IMAPFolders
	} else {
		folders = []string{"INBOX"}
	}

	// Apply exclusion patterns (compiled as regex, matched against full folder name).
	if len(job.IMAPExcludeFolders) > 0 {
		var patterns []*regexp.Regexp
		for _, p := range job.IMAPExcludeFolders {
			re, err := regexp.Compile(p)
			if err != nil {
				return fmt.Errorf("invalid imap_exclude_folders pattern %q: %w", p, err)
			}
			patterns = append(patterns, re)
		}
		kept := folders[:0]
		for _, f := range folders {
			excluded := false
			for _, re := range patterns {
				if re.MatchString(f) {
					excluded = true
					break
				}
			}
			if excluded {
				slog.Info("imap folder excluded", "folder", f)
			} else {
				kept = append(kept, f)
			}
		}
		folders = kept
	}

	cancelFn := func() bool { return ctx.Err() != nil }
	progressFn := func(folder string, folderIdx, totalFolders, emailsProcessed int) {
		slog.Info("imap", "folder", folder, "folder_idx", folderIdx, "total_folders", totalFolders, "emails", emailsProcessed)
	}

	total, err := imapimport.ImportFolders(ctx, pool, params, folders, job.IMAPNewOnly, cancelFn, progressFn)
	if err != nil {
		return err
	}
	slog.Info("imap complete", "emails_processed", total)
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func extForMediaType(mt *string) string {
	if mt == nil {
		return "jpg"
	}
	switch *mt {
	case "image/png":
		return "png"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/heic", "image/heif":
		return "heic"
	case "image/bmp":
		return "bmp"
	case "image/tiff":
		return "tiff"
	default:
		return "jpg"
	}
}

func pathExt(p string) string {
	for i := len(p) - 1; i >= 0 && p[i] != '/'; i-- {
		if p[i] == '.' {
			return p[i:]
		}
	}
	return ""
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
