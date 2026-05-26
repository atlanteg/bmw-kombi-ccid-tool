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
	"time"

	"github.com/lxn/walk"
)

const (
	ghReleasesAPI = "https://api.github.com/repos/atlanteg/super-kombi-ccid-tool/releases/latest"
	updateAsset   = "kombi-ccid-win32.exe"
	idYes         = 6 // Windows IDYES
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// checkAndUpdate runs in a goroutine.
// Strategy: while the process is still running, rename the current exe
// out of the way (Windows allows renaming a running exe via FILE_SHARE_DELETE),
// move the downloaded exe into its place, start it, then exit.
// No batch scripts, no timing hacks.
func checkAndUpdate(mw *walk.MainWindow) {
	if version == "dev" {
		return
	}

	// On startup: clean up any leftover .bak from a previous update.
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".bak")
	}

	rel, err := fetchRelease()
	if err != nil || rel == nil {
		return // network unavailable — silent fail
	}
	if !versionNewer(rel.TagName, version) {
		return
	}

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

	// Ask user.
	proceed := false
	mw.Synchronize(func() {
		res := walk.MsgBox(mw,
			"Update Available",
			fmt.Sprintf("Version %s is available (you have %s).\n\nDownload and restart now?",
				rel.TagName, version),
			walk.MsgBoxYesNo|walk.MsgBoxIconInformation,
		)
		proceed = res == idYes
	})
	if !proceed {
		return
	}

	// Resolve real exe path (follow symlinks, etc.)
	exePath, err := os.Executable()
	if err != nil {
		showUpdateError(mw, "Cannot locate executable:\n"+err.Error())
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	newExePath := exePath + ".new"
	bakPath := exePath + ".bak"

	// Download new exe into the same directory.
	mw.Synchronize(func() {
		mw.SetTitle("BMW Kombi CC-ID Calculator — downloading update…")
	})
	if err := downloadFile(downloadURL, newExePath); err != nil {
		_ = os.Remove(newExePath)
		mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator " + version) })
		showUpdateError(mw, "Download failed:\n"+err.Error())
		return
	}

	// ── Atomic swap (all os.Rename calls stay within the same directory) ──────
	//
	// 1. Rename running exe → .bak   (allowed while running on Windows 7+)
	_ = os.Remove(bakPath) // clear any leftover
	if err := os.Rename(exePath, bakPath); err != nil {
		_ = os.Remove(newExePath)
		mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator " + version) })
		showUpdateError(mw,
			"Cannot move the current executable.\n"+
				"Make sure the app is in a folder you have write access to.\n\n"+
				err.Error())
		return
	}

	// 2. Move downloaded exe → final path
	if err := os.Rename(newExePath, exePath); err != nil {
		// Restore original
		_ = os.Rename(bakPath, exePath)
		_ = os.Remove(newExePath)
		mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator " + version) })
		showUpdateError(mw, "Cannot install update:\n"+err.Error())
		return
	}

	// 3. Start the new version.
	cmd := exec.Command(exePath)
	if err := cmd.Start(); err != nil {
		// Undo swap
		_ = os.Rename(exePath, newExePath)
		_ = os.Rename(bakPath, exePath)
		mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator " + version) })
		showUpdateError(mw, "Cannot start updated version:\n"+err.Error())
		return
	}

	// 4. Best-effort: delete backup (new process also attempts this on startup).
	_ = os.Remove(bakPath)

	// 5. Exit — the new version is already running.
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
		return nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// versionNewer returns true when latest > current (semver comparison).
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

func parseVer(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var r [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		if idx := strings.IndexAny(p, "-+"); idx >= 0 {
			p = p[:idx]
		}
		r[i], _ = strconv.Atoi(p)
	}
	return r
}

func showUpdateError(mw *walk.MainWindow, msg string) {
	mw.Synchronize(func() {
		walk.MsgBox(mw, "Update Error", msg, walk.MsgBoxIconError)
	})
}
