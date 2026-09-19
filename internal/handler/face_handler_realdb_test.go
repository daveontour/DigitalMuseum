package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/daveontour/aimuseum/internal/appctx"
	"github.com/daveontour/aimuseum/internal/config"
	"github.com/daveontour/aimuseum/internal/database"
	"github.com/daveontour/aimuseum/internal/importer"
	"github.com/daveontour/aimuseum/internal/repository"
	"github.com/daveontour/aimuseum/internal/service"
)

// repoBundledFaceRecognizerPath resolves the real bundled facerecognizer.exe
// relative to the repo root (go test's working directory is this package's
// directory, internal/handler).
func repoBundledFaceRecognizerPath() string {
	return filepath.Join("..", "..", "bin", "FaceRecognizer", "facerecognizer.exe")
}

// TestRunFaceDetection_And_ClusterUnassignedFaces_RealStack drives the exact
// worker function the "Detect faces in photos" background job runs
// (runFaceDetection), against a real on-disk SQLite database (the real
// migration, not a hand-rolled schema) and the real bundled facerecognizer
// subprocess — not mocks at any layer. It then runs the exact same
// clustering path the "Group similar faces" job uses on the resulting real
// embeddings. Skipped automatically when the bundled binary/models aren't
// staged (fresh checkout / CI), matching the pattern used by
// internal/service/facerecognizer's own subprocess integration test.
func TestRunFaceDetection_And_ClusterUnassignedFaces_RealStack(t *testing.T) {
	exePath := repoBundledFaceRecognizerPath()
	if _, err := os.Stat(exePath); err != nil {
		t.Skipf("bin/FaceRecognizer/facerecognizer.exe not staged, skipping: %v", err)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "bin", "FaceRecognizer", "onnxruntime.dll")); err != nil {
		t.Skipf("bin/FaceRecognizer/onnxruntime.dll not staged, skipping: %v", err)
	}

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "archive.sqlite")

	ctx := context.Background()
	db, err := database.New(ctx, config.DatabaseConfig{SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("open real db: %v", err)
	}
	if db == nil {
		t.Fatal("database.New returned nil db")
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.MigrateSQLite(ctx, db.Std); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	uid := int64(2)
	ctx = context.WithValue(ctx, appctx.ContextKeyUserID, uid)

	if _, err := db.Std.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, is_active, is_admin) VALUES (?1, 'a@b.c', 'x', 'Test', 1, 0)`,
		uid); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Seed two real photos (persona avatars already in the repo — see the
	// sibling test in internal/service/facerecognizer for why these specific
	// files were picked: one has a detectable face, one doesn't).
	seedImage := func(path, mediaType string) int64 {
		imgBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var blobID int64
		if err := db.Std.QueryRowContext(ctx,
			`INSERT INTO media_blobs (image_data, user_id) VALUES (?1, ?2) RETURNING id`,
			imgBytes, uid,
		).Scan(&blobID); err != nil {
			t.Fatalf("seed media_blob for %s: %v", path, err)
		}
		var mediaItemID int64
		if err := db.Std.QueryRowContext(ctx,
			`INSERT INTO media_items (media_blob_id, media_type, user_id) VALUES (?1, ?2, ?3) RETURNING id`,
			blobID, mediaType, uid,
		).Scan(&mediaItemID); err != nil {
			t.Fatalf("seed media_item for %s: %v", path, err)
		}
		return mediaItemID
	}

	withFace := seedImage(filepath.Join("..", "..", "static", "images", "female-friend.png"), "image/png")
	withoutFace := seedImage(filepath.Join("..", "..", "static", "images", "bar-girl.png"), "image/png")

	imageRepo := repository.NewImageRepo(db.Std)
	imageSvc := service.NewImageService(imageRepo, nil)
	faceRepo := repository.NewFaceRepo(db.Std)
	embedHelper := service.NewFaceEmbeddingHelper(db.Std)
	faceSvc := service.NewFaceService(faceRepo, embedHelper)

	// Sanity check: both photos should be queued for detection before the job runs.
	queued, err := faceRepo.ListImageIDsForFaceDetection(ctx)
	if err != nil {
		t.Fatalf("ListImageIDsForFaceDetection (before): %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("want 2 photos queued before detection, got %d: %v", len(queued), queued)
	}

	// Run the exact worker function the background job runner launches via
	// `go runFaceDetection(...)` in startFaceDetection — called synchronously
	// here (against a fresh job instance, not the package-level
	// faceDetectionJob singleton) so the test can assert on its effects
	// deterministically without touching shared state.
	job := importer.NewImportJob("Face detection (test)", map[string]any{
		"status": "idle", "status_line": nil, "error_message": nil,
		"total": 0, "processed": 0, "faces_found": 0, "errors": 0,
	})
	runFaceDetection(faceRepo, embedHelper, imageSvc, job, exePath, uid, []int64{withFace, withoutFace})

	state := job.GetState()
	if state["status"] != "completed" {
		t.Fatalf("want job status completed, got %+v", state)
	}

	// Both photos should now be marked processed, regardless of whether a
	// face was found in each — that's what stops them being rescanned.
	queued, err = faceRepo.ListImageIDsForFaceDetection(ctx)
	if err != nil {
		t.Fatalf("ListImageIDsForFaceDetection (after): %v", err)
	}
	if len(queued) != 0 {
		t.Fatalf("want 0 photos left queued after detection, got %d: %v", len(queued), queued)
	}

	facesWithFace, err := faceRepo.ListFacesByMediaItem(ctx, withFace)
	if err != nil {
		t.Fatalf("ListFacesByMediaItem(withFace): %v", err)
	}
	t.Logf("female-friend.png: %d face(s) detected", len(facesWithFace))
	if len(facesWithFace) == 0 {
		t.Fatalf("want at least 1 face detected in female-friend.png (matches the standalone subprocess test's result)")
	}
	for _, f := range facesWithFace {
		if f.EmbeddingModel != faceEmbeddingModelName {
			t.Errorf("want embedding_model %q, got %q", faceEmbeddingModelName, f.EmbeddingModel)
		}
		if f.BBoxW <= 0 || f.BBoxH <= 0 {
			t.Errorf("non-positive bbox size on stored face: %+v", f)
		}
	}

	facesWithoutFace, err := faceRepo.ListFacesByMediaItem(ctx, withoutFace)
	if err != nil {
		t.Fatalf("ListFacesByMediaItem(withoutFace): %v", err)
	}
	t.Logf("bar-girl.png: %d face(s) detected", len(facesWithoutFace))

	// Now run the actual clustering job on whatever real embeddings detection
	// just produced, proving the two jobs compose correctly end-to-end.
	stats, err := faceSvc.ClusterUnassignedFaces(ctx)
	if err != nil {
		t.Fatalf("ClusterUnassignedFaces: %v", err)
	}
	t.Logf("clustering stats: %+v", stats)
	if stats.Errors != 0 {
		t.Errorf("want 0 clustering errors, got %d", stats.Errors)
	}
	if stats.Processed != len(facesWithFace)+len(facesWithoutFace) {
		t.Errorf("want %d faces processed by clustering, got %d", len(facesWithFace)+len(facesWithoutFace), stats.Processed)
	}

	// Every detected face should have ended up in some cluster.
	for _, f := range facesWithFace {
		got, err := faceRepo.GetFace(ctx, f.ID)
		if err != nil {
			t.Fatalf("GetFace: %v", err)
		}
		if got.FaceClusterID == nil {
			t.Errorf("face %d was not assigned a cluster by ClusterUnassignedFaces", f.ID)
		}
	}
}
