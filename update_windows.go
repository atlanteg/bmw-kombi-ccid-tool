//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lxn/walk"
)

const (
	ghReleasesAPI = "https://api.github.com/repos/atlanteg/super-kombi-ccid-tool/releases/latest"
	updateAsset   = "kombi-ccid-win32.exe"
	idYes         = 6 // Windows IDYES — returned by MsgBox when user clicks Yes
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// checkAndUpdate must be called in a goroutine.
// It silently checks GitHub for a newer release; if one is found it prompts
// the user and, on confirmation, downloads the new exe and restarts the app.
func checkAndUpdate(mw *walk.MainWindow) {
	if version == "dev" {
		return // never auto-update in dev builds
	}

	rel, err := fetchRelease()
	if err != nil || rel == nil {
		return // network unavailable or API error — silent fail
	}
	if !versionNewer(rel.TagName, version) {
		return // already up to date
	}

	// Find the Windows exe in the release assets.
	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == updateAsset {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return
	}

	// All UI must happen on the Walk main thread.
	proceed := false
	mw.Synchronize(func() {
		res := walk.MsgBox(mw,
			"Update Available",
			fmt.Sprintf(
				"Version %s is available (you have %s).\n\nDownload and restart now?",
				rel.TagName, version),
			walk.MsgBoxYesNo|walk.MsgBoxIconInformation,
		)
		proceed = res == idYes
	})
	if !proceed {
		return
	}

	// Resolve the real path of the running exe.
	exePath, err := os.Executable()
	if err != nil {
		showError(mw, "Cannot locate executable:\n"+err.Error())
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	newExePath := exePath + ".new"

	// Download.
	mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator — downloading update…") })
	if err := downloadFile(downloadURL, newExePath); err != nil {
		mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator " + version) })
		os.Remove(newExePath)
		showError(mw, "Download failed:\n"+err.Error())
		return
	}

	// Write a small batch script to %TEMP% that:
	//   1. waits 2 s for this process to exit
	//   2. atomically replaces the exe
	//   3. starts the new exe
	//   4. self-deletes
	scriptPath := filepath.Join(os.TempDir(), "kombi_update.bat")
	script := "@echo off\r\n" +
		"timeout /t 2 /nobreak >nul\r\n" +
		"move /y \"" + newExePath + "\" \"" + exePath + "\"\r\n" +
		"start \"\" \"" + exePath + "\"\r\n" +
		"del \"%~f0\"\r\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		os.Remove(newExePath)
		showError(mw, "Cannot write updater script:\n"+err.Error())
		return
	}

	// Launch the batch script without a visible console window.
	cmd := exec.Command("cmd", "/c", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		os.Remove(newExePath)
		os.Remove(scriptPath)
		showError(mw, "Cannot launch updater:\n"+err.Error())
		return
	}

	// Exit — the batch script will replace the exe and relaunch it.
	os.Exit(0)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func fetchRelease() (*ghRelease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", ghReleasesAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "super-kombi-ccid-tool/"+version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func downloadFile(url, destPath string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// versionNewer returns true when latest is strictly greater than current.
func versionNewer(latest, current string) bool {
	l, c := parseVer(latest), parseVer(current)
	for i := range l {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

// parseVer splits "v1.2.3" into [1, 2, 3].
func parseVer(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var r [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		// strip pre-release suffixes like "-rc1"
		p = strings.FieldsFunc(p, func(c rune) bool { return c == '-' || c == '+' })[0]
		r[i], _ = strconv.Atoi(p)
	}
	return r
}

func showError(mw *walk.MainWindow, msg string) {
	mw.Synchronize(func() {
		walk.MsgBox(mw, "Update Error", msg, walk.MsgBoxIconError)
	})
}
