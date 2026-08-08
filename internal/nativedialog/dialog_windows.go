//go:build windows

package nativedialog

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func Available() bool { return true }

func Folder(title string) (string, error) {
	title = psQuote(title)
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = '%s'
$d.ShowNewFolderButton = $true
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
  [Console]::Out.Write($d.SelectedPath)
}
`, title)
	return runPS(script)
}

func File(title, filter string) (string, error) {
	title = psQuote(title)
	if filter == "" {
		filter = "All files (*.*)|*.*"
	}
	filter = psQuote(filter)
	script := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Title = '%s'
$d.Filter = '%s'
$d.Multiselect = $false
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) {
  [Console]::Out.Write($d.FileName)
}
`, title, filter)
	return runPS(script)
}

func psQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func runPS(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("folder dialog: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCanceled
	}
	return path, nil
}
