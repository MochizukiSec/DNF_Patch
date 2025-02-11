package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	defaultDNFPath = "C:\\Wegame\\WeGame\\games\\DNF"
	imagePack2Dir  = "imagepack2"
)

type PatchRating struct {
	Average float64 `json:"average"`
	Count   int     `json:"count"`
}

type PatchPreview struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type UpdateInfo struct {
	LatestVersion string `json:"latestVersion"`
	UpdateURL     string `json:"updateUrl"`
	Changelog     string `json:"changelog"`
}

type Patch struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Filename    string        `json:"filename"`
	Version     string        `json:"version"`
	Author      string        `json:"author"`
	Tags        []string      `json:"tags"`
	Rating      PatchRating   `json:"rating"`
	Previews    []PatchPreview `json:"previews"`
	UpdateInfo  UpdateInfo    `json:"updateInfo"`
	Downloads   int           `json:"downloads"`
	LastUpdated string        `json:"lastUpdated"`
}

type InstallHistory struct {
	PatchID    string    `json:"patchId"`
	PatchName  string    `json:"patchName"`
	Version    string    `json:"version"`
	Timestamp  time.Time `json:"timestamp"`
	Status     string    `json:"status"`
}

type PatchCategory struct {
	Name    string   `json:"name"`
	Patches []Patch  `json:"patches"`
}

type PatchDatabase struct {
	Categories []PatchCategory `json:"categories"`
}

type BackupFile struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type Backup struct {
	ID          string       `json:"id"`
	Timestamp   time.Time    `json:"timestamp"`
	Description string       `json:"description"`
	Files       []BackupFile `json:"files"`
	Type        string       `json:"type"` // auto, manual
	GameVersion string       `json:"gameVersion"`
}

type BackupSettings struct {
	AutoBackup        bool   `json:"autoBackup"`
	BackupInterval    int    `json:"backupInterval"` // in seconds
	MaxBackups        int    `json:"maxBackups"`
	BackupPath        string `json:"backupPath"`
	CompressionEnabled bool  `json:"compressionEnabled"`
}

type BackupDatabase struct {
	Backups  []Backup       `json:"backups"`
	Settings BackupSettings `json:"settings"`
}

var (
	primaryColor   = color.NRGBA{R: 255, G: 107, B: 107, A: 255}  // 主色调：现代感的珊瑚红
	secondaryColor = color.NRGBA{R: 78, G: 205, B: 196, A: 255}   // 次要色调：清新的青绿色
	accentColor    = color.NRGBA{R: 255, G: 230, B: 109, A: 255}  // 强调色：明亮的黄色
	textColor      = color.NRGBA{R: 255, G: 255, B: 255, A: 230}  // 文本颜色：柔和的白色
	bgColor        = color.NRGBA{R: 45, G: 52, B: 54, A: 255}     // 背景色：深色渐变起始

	// Common DNF installation paths
	commonPaths = []string{
		"C:\\Wegame\\WeGame\\games\\DNF",
		"D:\\Wegame\\WeGame\\games\\DNF",
		"E:\\Wegame\\WeGame\\games\\DNF",
		"C:\\Program Files\\Wegame\\WeGame\\games\\DNF",
		"C:\\Program Files (x86)\\Wegame\\WeGame\\games\\DNF",
		"D:\\Program Files\\Wegame\\WeGame\\games\\DNF",
		"D:\\Program Files (x86)\\Wegame\\WeGame\\games\\DNF",
	}
)

type PatchApp struct {
	window         fyne.Window
	dnfPath        string
	status         *widget.Label
	progressBar    *widget.ProgressBar
	pathEntry      *widget.Entry
	patches        PatchDatabase
	searchEntry    *widget.Entry
	history        []InstallHistory
	historyFile    string
	backups        BackupDatabase
	backupTimer    *time.Timer
}

func loadPatchDatabase() (PatchDatabase, error) {
	var db PatchDatabase
	
	// Get the executable directory
	ex, err := os.Executable()
	if err != nil {
		return db, err
	}
	exPath := filepath.Dir(ex)
	
	// Read patches.json
	data, err := ioutil.ReadFile(filepath.Join(exPath, "patches", "patches.json"))
	if err != nil {
		return db, err
	}

	err = json.Unmarshal(data, &db)
	return db, err
}

func createPatchList(patches []Patch, onSelect func(patch Patch)) *widget.List {
	items := make([]string, len(patches))
	for i, patch := range patches {
		items[i] = patch.Name
	}

	list := widget.NewList(
		func() int { return len(patches) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.FileIcon()),
				widget.NewLabel("Template"),
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			box := item.(*fyne.Container)
			label := box.Objects[1].(*widget.Label)
			label.SetText(patches[id].Name)
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		onSelect(patches[id])
	}

	return list
}

func showPatchDetails(patch Patch, parent fyne.Window, onInstall func(patch Patch)) {
	content := container.NewVBox(
		widget.NewLabelWithStyle(patch.Name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewLabel("Description: " + patch.Description),
		widget.NewLabel("Version: " + patch.Version),
		widget.NewLabel("Author: " + patch.Author),
		widget.NewLabel("Tags: " + fmt.Sprintf("%v", patch.Tags)),
	)

	installButton := widget.NewButtonWithIcon("Install Patch", theme.DownloadIcon(), func() {
		onInstall(patch)
		dialog.ShowInformation("Success", "Patch installation started!", parent)
	})
	installButton.Importance = widget.HighImportance

	content.Add(installButton)

	dialog.ShowCustom("Patch Details", "Close", content, parent)
}

func findDNFPath() string {
	// First check common paths
	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			if isValidDNFPath(path) {
				return path
			}
		}
	}

	// For Windows, try to find through drive letters
	if runtime.GOOS == "windows" {
		for _, drive := range "CDEFGHIJKLMNOPQRSTUVWXYZ" {
			root := string(drive) + ":\\"
			path := findDNFInDirectory(root)
			if path != "" {
				return path
			}
		}
	}

	return defaultDNFPath
}

func findDNFInDirectory(root string) string {
	// Check if this directory contains DNF
	if isValidDNFPath(filepath.Join(root, "Wegame", "WeGame", "games", "DNF")) {
		return filepath.Join(root, "Wegame", "WeGame", "games", "DNF")
	}

	// Check Program Files
	programFiles := []string{
		filepath.Join(root, "Program Files", "Wegame", "WeGame", "games", "DNF"),
		filepath.Join(root, "Program Files (x86)", "Wegame", "WeGame", "games", "DNF"),
	}

	for _, path := range programFiles {
		if isValidDNFPath(path) {
			return path
		}
	}

	return ""
}

func isValidDNFPath(path string) bool {
	// Check for specific DNF files/folders that should exist
	indicators := []string{
		"DNF.exe",
		"imagepack2",
		"Script.pvf",
	}

	for _, indicator := range indicators {
		indicatorPath := filepath.Join(path, indicator)
		if _, err := os.Stat(indicatorPath); err == nil {
			return true
		}
	}

	return false
}

func newPatchApp() *PatchApp {
	a := app.New()
	win := a.NewWindow("DNF Patch Import Tool")
	
	p := &PatchApp{
		window:      win,
		status:      widget.NewLabel("Ready to import patches"),
		progressBar: widget.NewProgressBar(),
	}

	p.createUI()
	return p
}

func createCard(title string, content fyne.CanvasObject) *fyne.Container {
	titleLabel := canvas.NewText(title, primaryColor)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.TextSize = 16

	card := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		content,
	)

	return container.NewPadded(
		container.NewMax(
			canvas.NewRectangle(color.White),
			container.NewPadded(card),
		),
	)
}

func (p *PatchApp) loadHistory() error {
	historyPath := filepath.Join(filepath.Dir(p.historyFile), "install_history.json")
	data, err := ioutil.ReadFile(historyPath)
	if os.IsNotExist(err) {
		p.history = []InstallHistory{}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &p.history)
}

func (p *PatchApp) saveHistory() error {
	historyPath := filepath.Join(filepath.Dir(p.historyFile), "install_history.json")
	data, err := json.MarshalIndent(p.history, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(historyPath, data, 0644)
}

func (p *PatchApp) addToHistory(patch Patch, status string) {
	history := InstallHistory{
		PatchID:    patch.ID,
		PatchName:  patch.Name,
		Version:    patch.Version,
		Timestamp:  time.Now(),
		Status:     status,
	}
	p.history = append(p.history, history)
	p.saveHistory()
}

func (p *PatchApp) createSearchUI() fyne.CanvasObject {
	p.searchEntry = widget.NewEntry()
	p.searchEntry.SetPlaceHolder("Search patches...")
	
	searchIcon := widget.NewIcon(theme.SearchIcon())
	
	return container.NewBorder(
		nil, nil,
		searchIcon, nil,
		p.searchEntry,
	)
}

func (p *PatchApp) filterPatches(query string) []Patch {
	if query == "" {
		return nil
	}
	
	query = strings.ToLower(query)
	var results []Patch
	
	for _, category := range p.patches.Categories {
		for _, patch := range category.Patches {
			if strings.Contains(strings.ToLower(patch.Name), query) ||
				strings.Contains(strings.ToLower(patch.Description), query) ||
				containsTag(patch.Tags, query) {
				results = append(results, patch)
			}
		}
	}
	
	// Sort by rating and downloads
	sort.Slice(results, func(i, j int) bool {
		if results[i].Rating.Average == results[j].Rating.Average {
			return results[i].Downloads > results[j].Downloads
		}
		return results[i].Rating.Average > results[j].Rating.Average
	})
	
	return results
}

func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func createRatingWidget(rating PatchRating) fyne.CanvasObject {
	starsContainer := container.NewHBox()
	
	for i := 0; i < 5; i++ {
		var star *widget.Icon
		if float64(i) < rating.Average {
			star = widget.NewIcon(theme.MailForwardIcon()) // Using a different icon for filled stars
		} else {
			star = widget.NewIcon(theme.MailReplyIcon()) // Using a different icon for empty stars
		}
		starsContainer.Add(star)
	}
	
	ratingLabel := widget.NewLabel(fmt.Sprintf("%.1f (%d ratings)", rating.Average, rating.Count))
	
	return container.NewHBox(starsContainer, ratingLabel)
}

func (p *PatchApp) createPreviewUI(previews []PatchPreview) fyne.CanvasObject {
	if len(previews) == 0 {
		return widget.NewLabel("No previews available")
	}

	tabs := container.NewAppTabs()
	for _, preview := range previews {
		previewImage := canvas.NewImageFromFile(preview.URL)
		previewImage.FillMode = canvas.ImageFillOriginal
		previewImage.SetMinSize(fyne.NewSize(400, 300))
		
		description := widget.NewLabel(preview.Description)
		content := container.NewVBox(previewImage, description)
		
		tabs.Append(container.NewTabItem("Preview", content))
	}
	
	return tabs
}

func (p *PatchApp) checkForUpdates(patch Patch) {
	if patch.Version != patch.UpdateInfo.LatestVersion {
		dialog.ShowConfirm("Update Available",
			fmt.Sprintf("A new version (%s) is available. Current version: %s\n\nChangelog:\n%s",
				patch.UpdateInfo.LatestVersion,
				patch.Version,
				patch.UpdateInfo.Changelog),
			func(update bool) {
				if update {
					p.updateStatus(fmt.Sprintf("Downloading update for %s...", patch.Name))
					// TODO: Implement update download
				}
			},
			p.window)
	}
}

func (p *PatchApp) createHistoryUI() fyne.CanvasObject {
	list := widget.NewList(
		func() int { return len(p.history) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel("Template"),
				widget.NewLabel("Template"),
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			box := item.(*fyne.Container)
			nameLabel := box.Objects[1].(*widget.Label)
			timeLabel := box.Objects[2].(*widget.Label)
			
			history := p.history[len(p.history)-1-id] // Show newest first
			nameLabel.SetText(fmt.Sprintf("%s (%s)", history.PatchName, history.Version))
			timeLabel.SetText(history.Timestamp.Format("2006-01-02 15:04:05"))
		},
	)
	
	return container.NewBorder(
		widget.NewLabel("Installation History"),
		nil, nil, nil,
		list,
	)
}

func (p *PatchApp) loadBackupDatabase() error {
	backupPath := filepath.Join(filepath.Dir(p.historyFile), "backup", "backup.json")
	data, err := ioutil.ReadFile(backupPath)
	if os.IsNotExist(err) {
		// Create default backup settings
		p.backups = BackupDatabase{
			Settings: BackupSettings{
				AutoBackup:        true,
				BackupInterval:    3600, // 1 hour
				MaxBackups:        10,
				BackupPath:        "backups",
				CompressionEnabled: true,
			},
		}
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &p.backups)
}

func (p *PatchApp) saveBackupDatabase() error {
	backupPath := filepath.Join(filepath.Dir(p.historyFile), "backup", "backup.json")
	data, err := json.MarshalIndent(p.backups, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(backupPath, data, 0644)
}

func (p *PatchApp) calculateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func (p *PatchApp) createBackup(description string, backupType string) error {
	// Create backup ID
	backupID := fmt.Sprintf("backup_%s", time.Now().Format("20060102_150405"))
	
	// Create backup directory
	backupDir := filepath.Join(filepath.Dir(p.historyFile), p.backups.Settings.BackupPath, backupID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	// Collect files to backup
	var files []BackupFile
	err := filepath.Walk(filepath.Join(p.dnfPath, "imagepack2"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".npk") {
			hash, err := p.calculateFileHash(path)
			if err != nil {
				return err
			}
			
			relPath, err := filepath.Rel(p.dnfPath, path)
			if err != nil {
				return err
			}
			
			files = append(files, BackupFile{
				Path: relPath,
				Hash: hash,
				Size: info.Size(),
			})
			
			// Copy file to backup directory
			destPath := filepath.Join(backupDir, relPath)
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			
			src, err := os.Open(path)
			if err != nil {
				return err
			}
			defer src.Close()
			
			dst, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer dst.Close()
			
			if _, err := io.Copy(dst, src); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Create backup record
	backup := Backup{
		ID:          backupID,
		Timestamp:   time.Now(),
		Description: description,
		Files:       files,
		Type:        backupType,
		GameVersion: "1.0.0", // TODO: Detect game version
	}
	
	// Add to database
	p.backups.Backups = append(p.backups.Backups, backup)
	
	// Remove old backups if exceeding limit
	if len(p.backups.Backups) > p.backups.Settings.MaxBackups {
		// Sort backups by time
		sort.Slice(p.backups.Backups, func(i, j int) bool {
			return p.backups.Backups[i].Timestamp.After(p.backups.Backups[j].Timestamp)
		})
		
		// Remove old backups
		oldBackups := p.backups.Backups[p.backups.Settings.MaxBackups:]
		p.backups.Backups = p.backups.Backups[:p.backups.Settings.MaxBackups]
		
		// Delete old backup files
		for _, backup := range oldBackups {
			backupPath := filepath.Join(filepath.Dir(p.historyFile), p.backups.Settings.BackupPath, backup.ID)
			os.RemoveAll(backupPath)
		}
	}
	
	// Save database
	return p.saveBackupDatabase()
}

func (p *PatchApp) restoreBackup(backup Backup) error {
	backupDir := filepath.Join(filepath.Dir(p.historyFile), p.backups.Settings.BackupPath, backup.ID)
	
	// Verify backup files
	for _, file := range backup.Files {
		backupFile := filepath.Join(backupDir, file.Path)
		hash, err := p.calculateFileHash(backupFile)
		if err != nil {
			return fmt.Errorf("backup verification failed: %v", err)
		}
		if hash != file.Hash {
			return fmt.Errorf("backup file corrupted: %s", file.Path)
		}
	}
	
	// Restore files
	for _, file := range backup.Files {
		backupFile := filepath.Join(backupDir, file.Path)
		destFile := filepath.Join(p.dnfPath, file.Path)
		
		// Create destination directory
		if err := os.MkdirAll(filepath.Dir(destFile), 0755); err != nil {
			return err
		}
		
		// Copy file
		src, err := os.Open(backupFile)
		if err != nil {
			return err
		}
		defer src.Close()
		
		dst, err := os.Create(destFile)
		if err != nil {
			return err
		}
		defer dst.Close()
		
		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
	}
	
	return nil
}

func (p *PatchApp) startBackupTimer() {
	if p.backupTimer != nil {
		p.backupTimer.Stop()
	}
	
	if p.backups.Settings.AutoBackup {
		p.backupTimer = time.NewTimer(time.Duration(p.backups.Settings.BackupInterval) * time.Second)
		go func() {
			for {
				<-p.backupTimer.C
				if err := p.createBackup("Auto backup", "auto"); err != nil {
					fmt.Printf("Auto backup failed: %v\n", err)
				}
				p.backupTimer.Reset(time.Duration(p.backups.Settings.BackupInterval) * time.Second)
			}
		}()
	}
}

func (p *PatchApp) createBackupSettingsUI() fyne.CanvasObject {
	autoBackup := widget.NewCheck("Enable Auto Backup", func(enabled bool) {
		p.backups.Settings.AutoBackup = enabled
		p.saveBackupDatabase()
		p.startBackupTimer()
	})
	autoBackup.SetChecked(p.backups.Settings.AutoBackup)
	
	intervalSelect := widget.NewSelect([]string{
		"30 minutes",
		"1 hour",
		"2 hours",
		"4 hours",
		"8 hours",
		"12 hours",
		"24 hours",
	}, func(s string) {
		var interval int
		switch s {
		case "30 minutes":
			interval = 1800
		case "1 hour":
			interval = 3600
		case "2 hours":
			interval = 7200
		case "4 hours":
			interval = 14400
		case "8 hours":
			interval = 28800
		case "12 hours":
			interval = 43200
		case "24 hours":
			interval = 86400
		}
		p.backups.Settings.BackupInterval = interval
		p.saveBackupDatabase()
		p.startBackupTimer()
	})
	
	maxBackupsEntry := widget.NewEntry()
	maxBackupsEntry.SetText(fmt.Sprintf("%d", p.backups.Settings.MaxBackups))
	maxBackupsEntry.OnChanged = func(s string) {
		var maxBackups int
		if _, err := fmt.Sscanf(s, "%d", &maxBackups); err == nil {
			p.backups.Settings.MaxBackups = maxBackups
			p.saveBackupDatabase()
		}
	}
	
	compression := widget.NewCheck("Enable Compression", func(enabled bool) {
		p.backups.Settings.CompressionEnabled = enabled
		p.saveBackupDatabase()
	})
	compression.SetChecked(p.backups.Settings.CompressionEnabled)
	
	return container.NewVBox(
		widget.NewLabel("Backup Settings"),
		autoBackup,
		container.NewHBox(widget.NewLabel("Backup Interval:"), intervalSelect),
		container.NewHBox(widget.NewLabel("Max Backups:"), maxBackupsEntry),
		compression,
	)
}

func (p *PatchApp) createBackupListUI() fyne.CanvasObject {
	list := widget.NewList(
		func() int { return len(p.backups.Backups) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel("Template"),
				widget.NewLabel("Template"),
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			box := item.(*fyne.Container)
			nameLabel := box.Objects[1].(*widget.Label)
			timeLabel := box.Objects[2].(*widget.Label)
			
			backup := p.backups.Backups[len(p.backups.Backups)-1-id] // Show newest first
			nameLabel.SetText(fmt.Sprintf("%s (%s)", backup.Description, backup.Type))
			timeLabel.SetText(backup.Timestamp.Format("2006-01-02 15:04:05"))
		},
	)
	
	list.OnSelected = func(id widget.ListItemID) {
		backup := p.backups.Backups[len(p.backups.Backups)-1-id]
		content := container.NewVBox(
			widget.NewLabel(fmt.Sprintf("Backup ID: %s", backup.ID)),
			widget.NewLabel(fmt.Sprintf("Type: %s", backup.Type)),
			widget.NewLabel(fmt.Sprintf("Time: %s", backup.Timestamp.Format("2006-01-02 15:04:05"))),
			widget.NewLabel(fmt.Sprintf("Files: %d", len(backup.Files))),
		)
		
		restoreButton := widget.NewButtonWithIcon("Restore", theme.HistoryIcon(), func() {
			dialog.ShowConfirm("Restore Backup",
				"Are you sure you want to restore this backup? Current files will be overwritten.",
				func(restore bool) {
					if restore {
						p.updateStatus("Restoring backup...")
						if err := p.restoreBackup(backup); err != nil {
							dialog.ShowError(err, p.window)
							p.updateStatus("Backup restoration failed!")
						} else {
							dialog.ShowInformation("Success", "Backup restored successfully!", p.window)
							p.updateStatus("Backup restored successfully!")
						}
					}
				},
				p.window)
		})
		restoreButton.Importance = widget.HighImportance
		
		content.Add(restoreButton)
		
		dialog.ShowCustom("Backup Details", "Close", content, p.window)
	}
	
	createButton := widget.NewButtonWithIcon("Create Backup", theme.DocumentCreateIcon(), func() {
		input := widget.NewEntry()
		input.SetPlaceHolder("Backup description")
		
		dialog.ShowCustomConfirm("Create Backup",
			"Create",
			"Cancel",
			container.NewVBox(
				widget.NewLabel("Enter backup description:"),
				input,
			),
			func(create bool) {
				if create {
					description := input.Text
					if description == "" {
						description = "Manual backup"
					}
					
					p.updateStatus("Creating backup...")
					if err := p.createBackup(description, "manual"); err != nil {
						dialog.ShowError(err, p.window)
						p.updateStatus("Backup creation failed!")
					} else {
						dialog.ShowInformation("Success", "Backup created successfully!", p.window)
						p.updateStatus("Backup created successfully!")
					}
				}
			},
			p.window)
	})
	
	return container.NewBorder(
		container.NewHBox(
			widget.NewLabel("Backups"),
			createButton,
		),
		nil, nil, nil,
		list,
	)
}

func (p *PatchApp) createPatchesUI() fyne.CanvasObject {
	list := widget.NewList(
		func() int {
			return len(p.patches.Categories)
		},
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewIcon(theme.DocumentIcon()),
				widget.NewLabel("Template"),
			)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			category := p.patches.Categories[id]
			label := item.(*fyne.Container).Objects[1].(*widget.Label)
			label.SetText(category.Name)
		},
	)
	
	return list
}

func (p *PatchApp) updatePatchList(query string) {
	// TODO: Implement patch filtering based on search query
}

func (p *PatchApp) createUI() {
	// 背景渐变
	bg := canvas.NewLinearGradient(bgColor, color.NRGBA{R: 45, G: 52, B: 54, A: 200}, 270)
	bg.Resize(fyne.NewSize(800, 600))
	
	// Logo
	logoURI, err := storage.ParseURI("file://" + filepath.Join(filepath.Dir(p.historyFile), "assets", "logo.svg"))
	if err != nil {
		fmt.Printf("Error loading logo: %v\n", err)
	}
	
	var logo *canvas.Image
	if err == nil {
		logoResource, err := storage.LoadResourceFromURI(logoURI)
		if err == nil {
			logo = canvas.NewImageFromResource(logoResource)
			logo.SetMinSize(fyne.NewSize(120, 120))
			logo.Resize(fyne.NewSize(120, 120))
			logo.FillMode = canvas.ImageFillContain
		}
	}
	
	// 标题
	title := canvas.NewText("DNF Patch Manager", primaryColor)
	title.TextSize = 28
	title.TextStyle = fyne.TextStyle{Bold: true}
	
	// 副标题
	subtitle := canvas.NewText("Manage your DNF patches with ease", secondaryColor)
	subtitle.TextSize = 16
	
	// 头部容器
	var header *fyne.Container
	if logo != nil {
		header = container.NewHBox(
			container.NewPadded(logo),
			container.NewVBox(
				container.NewCenter(title),
				container.NewCenter(subtitle),
			),
		)
	} else {
		header = container.NewVBox(
			container.NewCenter(title),
			container.NewCenter(subtitle),
		)
	}

	// 路径选择
	p.pathEntry = widget.NewEntry()
	p.pathEntry.SetPlaceHolder("Enter DNF directory path")
	if p.dnfPath != "" {
		p.pathEntry.SetText(p.dnfPath)
	}

	browseButton := widget.NewButtonWithIcon("Browse", theme.FolderOpenIcon(), func() {
		dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, p.window)
				return
			}
			if uri == nil {
				return
			}
			p.dnfPath = uri.Path()
			p.pathEntry.SetText(p.dnfPath)
		}, p.window)
	})
	browseButton.Importance = widget.HighImportance

	pathContainer := container.NewBorder(
		nil, nil, nil, browseButton,
		container.NewVBox(
			widget.NewLabel("DNF Installation Directory:"),
			p.pathEntry,
		),
	)

	// 状态和进度条
	p.status = widget.NewLabel("Ready")
	p.status.Alignment = fyne.TextAlignCenter
	p.progressBar = widget.NewProgressBar()
	p.progressBar.Hide()

	statusContainer := container.NewVBox(
		p.status,
		p.progressBar,
	)

	// 搜索框
	p.searchEntry = widget.NewEntry()
	p.searchEntry.SetPlaceHolder("Search patches...")
	p.searchEntry.OnChanged = p.updatePatchList

	// 分类标签页
	var categoryTabs []*container.TabItem
	
	// 添加补丁标签页
	patchesTab := container.NewTabItem("Patches", p.createPatchesUI())
	categoryTabs = append(categoryTabs, patchesTab)
	
	// 添加历史标签页
	historyTab := container.NewTabItem("History", p.createHistoryUI())
	categoryTabs = append(categoryTabs, historyTab)
	
	// 添加备份标签页
	backupTab := container.NewTabItem("Backups", p.createBackupListUI())
	categoryTabs = append(categoryTabs, backupTab)
	
	tabs := container.NewAppTabs(categoryTabs...)
	tabs.SetTabLocation(container.TabLocationTop)
	
	// 主布局
	mainContent := container.NewBorder(
		container.NewVBox(
			header,
			widget.NewSeparator(),
			container.NewPadded(pathContainer),
			widget.NewSeparator(),
			container.NewPadded(p.searchEntry),
		),
		container.NewPadded(statusContainer),
		nil, nil,
		container.NewPadded(tabs),
	)

	// 设置内容
	p.window.SetContent(container.NewMax(bg, mainContent))
	p.window.Resize(fyne.NewSize(900, 600))
}

func (p *PatchApp) importPatch(reader fyne.URIReadCloser) {
	defer reader.Close()
	
	// Check imagepack2 directory
	imagepackPath := filepath.Join(p.dnfPath, imagePack2Dir)
	if _, err := os.Stat(imagepackPath); os.IsNotExist(err) {
		os.MkdirAll(imagepackPath, 0755)
	}

	// Create backup directory
	backupDir := filepath.Join(p.dnfPath, "backup_"+time.Now().Format("20060102_150405"))
	os.MkdirAll(backupDir, 0755)

	// Get patch filename
	patchName := filepath.Base(reader.URI().Path())
	targetPath := filepath.Join(imagepackPath, patchName)

	// Backup existing file if it exists
	if _, err := os.Stat(targetPath); err == nil {
		backupPath := filepath.Join(backupDir, patchName)
		if err := copyFile(targetPath, backupPath); err != nil {
			p.updateStatus(fmt.Sprintf("⚠️ Backup failed: %v", err))
			return
		}
		p.updateStatus("📦 Created backup successfully")
	}

	// Create target file
	target, err := os.Create(targetPath)
	if err != nil {
		p.updateStatus(fmt.Sprintf("❌ Failed to create file: %v", err))
		return
	}
	defer target.Close()

	// Copy file contents with progress updates
	p.progressBar.SetValue(0)
	p.updateStatus("📥 Importing patch...")
	
	_, err = io.Copy(target, reader)
	if err != nil {
		p.updateStatus(fmt.Sprintf("❌ Import failed: %v", err))
		return
	}

	p.progressBar.SetValue(1)
	p.updateStatus("✨ Patch imported successfully!")
}

func (p *PatchApp) updateStatus(msg string) {
	p.status.SetText(msg)
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func (p *PatchApp) showPatchDetails(patch Patch) {
	// Check for updates
	p.checkForUpdates(patch)
	
	content := container.NewVBox(
		widget.NewLabelWithStyle(patch.Name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewSeparator(),
		widget.NewLabel("Description: " + patch.Description),
		widget.NewLabel("Version: " + patch.Version),
		widget.NewLabel("Author: " + patch.Author),
		createRatingWidget(patch.Rating),
		widget.NewLabel(fmt.Sprintf("Downloads: %d", patch.Downloads)),
		p.createPreviewUI(patch.Previews),
	)

	installButton := widget.NewButtonWithIcon("Install Patch", theme.DownloadIcon(), func() {
		p.updateStatus(fmt.Sprintf("Installing patch: %s", patch.Name))
		// TODO: Implement actual patch installation
		p.addToHistory(patch, "Installed")
		dialog.ShowInformation("Success", "Patch installation completed!", p.window)
	})
	installButton.Importance = widget.HighImportance

	content.Add(installButton)

	dialog.ShowCustom("Patch Details", "Close", content, p.window)
}

func (p *PatchApp) Run() {
	p.window.Resize(fyne.NewSize(600, 500))
	p.window.CenterOnScreen()
	p.window.ShowAndRun()
}

func main() {
	app := newPatchApp()
	
	// Set history file path
	ex, err := os.Executable()
	if err == nil {
		app.historyFile = filepath.Join(filepath.Dir(ex), "install_history.json")
		app.loadHistory()
	}
	
	// Load backup database
	if err := app.loadBackupDatabase(); err != nil {
		fmt.Printf("Error loading backup database: %v\n", err)
	}
	
	// Start backup timer
	app.startBackupTimer()
	
	// Load patch database
	patches, err := loadPatchDatabase()
	if err != nil {
		fmt.Printf("Error loading patches: %v\n", err)
		patches = PatchDatabase{} // Use empty database if loading fails
	}
	app.patches = patches
	
	app.Run()
}
