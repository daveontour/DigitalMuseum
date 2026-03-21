// launcher is a Windows GUI-subsystem process that starts PostgreSQL and the
// DigitalMuseum web server, opens a browser, and provides a system-tray icon
// with "Open DigitalMuseum" and "Quit" menu items.
//
// Build with:
//
//	go build -ldflags="-H windowsgui" -o bin/launcher.exe ./cmd/launcher
package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"image/png"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/getlantern/systray"
)

//go:embed icon.png
var iconData []byte

// root is the directory containing the launcher executable.
// All other paths are derived from it.
var root string

func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot resolve executable path: %v", err)
	}
	root = filepath.Dir(exe)

	// Log to file since there is no console window.
	logFile, err := os.OpenFile(filepath.Join(root, "launcher.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}

	systray.Run(onReady, onExit)
}

func onReady() {
	// Prevent a second instance from interfering with the running one.
	if alreadyRunning() {
		log.Println("Another instance is already running — exiting.")
		systray.Quit()
		return
	}

	if ico, err := pngToICO(iconData); err == nil {
		systray.SetIcon(ico)
	}
	systray.SetTitle("DigitalMuseum")
	systray.SetTooltip("DigitalMuseum — running")

	mOpen := systray.AddMenuItem("Open DigitalMuseum", "Open in browser")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Shut down app and database")

	go startServices()

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openBrowser()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func onExit() {
	shutdown()
}

// ---------------------------------------------------------------------------
// Service lifecycle
// ---------------------------------------------------------------------------

func startServices() {
	pgBin := filepath.Join(root, "bin", "postgres", "pgsql", "bin")
	pgCtl := filepath.Join(pgBin, "pg_ctl.exe")
	initdbExe := filepath.Join(pgBin, "initdb.exe")
	dataDir := filepath.Join(root, "data")
	appBin := filepath.Join(root, "bin", "digitalmuseum.exe")

	// 1. Kill zombie postgres processes from a previous unclean exit.
	killZombies()

	// 2. Initialize the database cluster on first run.
	if _, err := os.Stat(filepath.Join(dataDir, "PG_VERSION")); os.IsNotExist(err) {
		log.Println("Initializing database...")
		if err := runCmd(initdbExe,
			"-D", dataDir,
			"-U", "postgres",
			"--auth=trust",
			"--encoding=UTF8",
			"--locale=C",
		); err != nil {
			log.Printf("initdb failed: %v", err)
		}
	}

	// 3. Start PostgreSQL.
	log.Println("Starting PostgreSQL...")
	if err := runCmd(pgCtl, "start",
		"-D", dataDir,
		"-o", "-p 5433 -c shared_buffers=128MB -c autovacuum_vacuum_cost_delay=20ms -c autovacuum_vacuum_cost_limit=200 -c log_checkpoints=off -c log_min_messages=warning",
	); err != nil {
		log.Printf("pg_ctl start failed: %v", err)
	}

	// 4. Start the web application (detached, no console window).
	log.Println("Starting DigitalMuseum...")
	cmd := exec.Command(appBin)
	cmd.Dir = root
	cmd.SysProcAttr = detachedProcAttr()
	if err := cmd.Start(); err != nil {
		log.Printf("app start failed: %v", err)
	}

	// 5. Wait until the web server is accepting connections, then open browser.
	log.Println("Waiting for server...")
	waitForServer("localhost:8001", 30*time.Second)
	openBrowser()
}

func shutdown() {
	pgBin := filepath.Join(root, "bin", "postgres", "pgsql", "bin")
	pgCtl := filepath.Join(pgBin, "pg_ctl.exe")
	dataDir := filepath.Join(root, "data")

	log.Println("Shutting down DigitalMuseum...")
	_ = runCmd("taskkill", "/f", "/im", "digitalmuseum.exe", "/t")

	log.Println("Stopping PostgreSQL...")
	_ = runCmd(pgCtl, "stop", "-D", dataDir, "-m", "fast")
	log.Println("Shutdown complete.")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func killZombies() {
	_ = runCmd("taskkill", "/f", "/im", "postgres.exe", "/t")
}

func openBrowser() {
	_ = runCmd("cmd", "/c", "start", "", "http://localhost:8001")
}

// waitForServer polls addr (host:port) until it accepts a TCP connection or
// the deadline is reached.
func waitForServer(addr string, maxWait time.Duration) {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Printf("server did not become ready within %s", maxWait)
}

// pngToICO wraps raw PNG bytes in a minimal ICO container so that the Windows
// LoadImage API (used internally by getlantern/systray) can load it.
func pngToICO(pngData []byte) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	// ICO files store width/height as 0 when the dimension is 256.
	wb, hb := byte(w), byte(h)
	if w >= 256 {
		wb = 0
	}
	if h >= 256 {
		hb = 0
	}

	const offset = 6 + 16 // ICONDIR (6) + one ICONDIRENTRY (16)
	buf := new(bytes.Buffer)

	// ICONDIR: reserved, type=1 (icon), count=1
	buf.Write([]byte{0, 0, 1, 0, 1, 0})

	// ICONDIRENTRY
	entry := make([]byte, 16)
	entry[0] = wb
	entry[1] = hb
	entry[2] = 0 // color count (0 = no palette)
	entry[3] = 0 // reserved
	binary.LittleEndian.PutUint16(entry[4:], 1)                    // planes
	binary.LittleEndian.PutUint16(entry[6:], 32)                   // bit count
	binary.LittleEndian.PutUint32(entry[8:], uint32(len(pngData))) // image size
	binary.LittleEndian.PutUint32(entry[12:], offset)              // image offset
	buf.Write(entry)

	buf.Write(pngData)
	return buf.Bytes(), nil
}

// runCmd runs an external command synchronously with no visible window.
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = hiddenProcAttr()
	return cmd.Run()
}
