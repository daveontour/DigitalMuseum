//go:build windows

package browse

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	shell32       = syscall.NewLazyDLL("shell32.dll")
	shellExecuteW = shell32.NewProc("ShellExecuteW")
)

func PickFolder() (string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.FolderBrowserDialog
$dialog.Description = "Select a folder"
$dialog.ShowNewFolderButton = $true
$result = $dialog.ShowDialog()
if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
    Write-Output $dialog.SelectedPath
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("folder dialog failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	path := strings.TrimSpace(stdout.String())
	return path, nil
}

func PickFile() (string, error) {
	script := `
Add-Type -AssemblyName System.Windows.Forms
$dialog = New-Object System.Windows.Forms.OpenFileDialog
$dialog.Title = "Select a file"
$dialog.Filter = "All files (*.*)|*.*"
$dialog.Multiselect = $false
$result = $dialog.ShowDialog()
if ($result -eq [System.Windows.Forms.DialogResult]::OK) {
    Write-Output $dialog.FileName
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("file dialog failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	path := strings.TrimSpace(stdout.String())
	return path, nil
}

func OpenInExplorer(filePath string) error {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return fmt.Errorf("path is required")
	}

	absPath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	params := fmt.Sprintf(`/select,"%s"`, absPath)
	if err := shellExecute("open", "explorer.exe", params); err == nil {
		return nil
	}

	folder := filepath.Dir(absPath)
	if err := shellExecute("explore", folder, ""); err != nil {
		return fmt.Errorf("open in explorer: %w", err)
	}

	return nil
}

func shellExecute(verb, file, params string) error {
	verbPtr, err := syscall.UTF16PtrFromString(verb)
	if err != nil {
		return err
	}
	filePtr, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		return err
	}

	var paramPtr *uint16
	if params != "" {
		paramPtr, err = syscall.UTF16PtrFromString(params)
		if err != nil {
			return err
		}
	}

	ret, _, _ := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(paramPtr)),
		0,
		1,
	)
	if ret <= 32 {
		return fmt.Errorf("shellExecute failed (code %d)", ret)
	}

	return nil
}
