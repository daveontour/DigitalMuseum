//go:build !windows

package browse

import "errors"

func PickFolder() (string, error) {
	return "", errors.New("folder browse is only supported on Windows; enter a path manually")
}

func PickFile() (string, error) {
	return "", errors.New("file browse is only supported on Windows; enter a path manually")
}

func OpenInExplorer(filePath string) error {
	return errors.New("open in explorer is only supported on Windows")
}
