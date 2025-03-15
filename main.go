package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/elliotchance/orderedmap/v3"
)

type errMsg error

// # Useful links
// https://docs.syncthing.net/dev/rest.html#rest-pagination
// https://github.com/76creates/stickers verify flexbox
// https://leg100.github.io/en/posts/building-bubbletea-programs/#2-dump-messages-to-a-file

// TODO create const for syncthing ports paths
// TODO currently we are skipping tls verification for the https://localhost path. What can we do about it

type model struct {
	spinner            spinner.Model
	loading            bool
	err                error
	folders            orderedmap.OrderedMap[string, SyncthingFolder]
	width              int
	height             int
	syncthingApiKey    string
	synchingEventSince int
}

var quitKeys = key.NewBinding(
	key.WithKeys("q", "esc", "ctrl+c"),
	key.WithHelp("", "press q to quit"),
)

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	syncthingApiKey := os.Getenv("SYNCTHING_API_KEY")

	return model{
		spinner:         s,
		loading:         true,
		syncthingApiKey: syncthingApiKey,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchFolders(m.syncthingApiKey),
		fetchEvents(m.syncthingApiKey, m.synchingEventSince),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if key.Matches(msg, quitKeys) {
			return m, tea.Quit

		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case FetchedFoldersMsg:
		foo := FetchedFoldersMsg(msg)
		m.loading = false
		if foo.err != nil {
			m.err = foo.err
			return m, nil
		}

		m.folders = FetchedFoldersMsg(msg).folders
		return m, nil
	case FetchedEventsMsg:
		events := FetchedEventsMsg(msg)
		if events.err != nil {
			// TODO figure out what to do if event errors
			m.err = events.err
		}
		if len(events.events) > 1 {
			m.synchingEventSince = events.events[len(events.events)-1].ID
		}
		cmds := make([]tea.Cmd, 0)
		for _, e := range events.events {
			if e.Type == "StateChanged" || e.Type == "FolderPaused" {
				cmds = append(cmds, fetchFolders(m.syncthingApiKey))
				break
			}
		}
		cmds = append(cmds, fetchEvents(m.syncthingApiKey, m.synchingEventSince))
		return m, tea.Batch(cmds...)

	case errMsg:
		m.err = msg
		return m, nil

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

// ------------------ VIEW --------------------------
func (m model) View() string {
	if m.err != nil {
		return m.err.Error()
	}

	if m.syncthingApiKey == "" {
		return "Missing api key to acess syncthing. Env: SYNCTHING_API_KEY"
	}

	if m.loading {
		str := fmt.Sprintf("\n\n   %s Loading syncthing data... %s\n\n", m.spinner.View(), quitKeys.Help().Desc)
		return str
	}

	return ViewFolders(slices.Collect(m.folders.Values()))
}

func ViewFolders(folders []SyncthingFolder) string {
	folderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		PaddingRight(2).
		PaddingLeft(2).
		Width(60).
		Align(lipgloss.Left)

	foo := ""
	for _, folder := range folders {
		foo += folderStyle.Render(
			lipgloss.NewStyle().Width(40).Align(lipgloss.Left).Render(folder.config.Label),
			lipgloss.NewStyle().Width(15).Align(lipgloss.Right).Render(statusLabel(folder)),
		)

		foo += "\n"
	}

	return foo
}

// TODO return colors somehow
func statusLabel(foo SyncthingFolder) string {
	if foo.status.State == "syncing" {
		return "Syncthing"
	}
	if foo.status.State == "scanning" {
		return lipgloss.
			NewStyle().
			Foreground(lipgloss.
				Color("#087331")).
			Render("Scanning")
	}

	if len(foo.status.Invalid) > 0 || len(foo.status.Error) > 0 {
		return "Error"
	}

	if foo.config.Paused {
		return "Paused"
	}

	if len(foo.config.Devices) == 1 {
		return "Unshared"
	}

	return lipgloss.
		NewStyle().
		Foreground(lipgloss.
			Color("#087331")).
		Render("Everything ok")
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// Message for HTTP response
type FetchedFoldersMsg struct {
	folders orderedmap.OrderedMap[string, SyncthingFolder]
	err     error
}

type FetchedEventsMsg struct {
	events []SyncthingEvent
	err    error
}

func fetchFolders(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var folders []SyncthingFolderConfig
		err := fetchBytes("http://localhost:8384/rest/config/folders", apiKey, &folders)
		if err != nil {
			return FetchedFoldersMsg{err: err}
		}

		// foo := make(orderedmap[string]SyncthingFolder)
		foo := orderedmap.NewOrderedMap[string, SyncthingFolder]()
		for _, configFolder := range folders {
			params := url.Values{}
			params.Add("folder", configFolder.ID)
			var statusFolder SyncthingFolderStatus
			err := fetchBytes(
				"http://localhost:8384/rest/db/status?"+params.Encode(),
				apiKey,
				&statusFolder)
			if err != nil {
				return FetchedFoldersMsg{err: err}
			}
			foo.Set(configFolder.ID, SyncthingFolder{
				config: configFolder,
				status: statusFolder,
			})
		}

		return FetchedFoldersMsg{folders: *foo}
	}
}

func fetchEvents(apiKey string, since int) tea.Cmd {
	return func() tea.Msg {
		var events []SyncthingEvent
		params := url.Values{}
		params.Add("since", strconv.Itoa(since))
		err := fetchBytes("http://localhost:8384/rest/events?"+params.Encode(), apiKey, &events)
		if err != nil {
			return FetchedEventsMsg{err: err}
		}

		return FetchedEventsMsg{events: events}
	}
}

type SyncthingFolder struct {
	config SyncthingFolderConfig
	status SyncthingFolderStatus
}

// SYNCTHING DATA STRUCTURES
type SyncthingFolderConfig struct {
	ID                      string      `json:"id"`
	Label                   string      `json:"label"`
	FilesystemType          string      `json:"filesystemType"`
	Path                    string      `json:"path"`
	Type                    string      `json:"type"`
	Devices                 []Device    `json:"devices"`
	RescanIntervalS         int         `json:"rescanIntervalS"`
	FsWatcherEnabled        bool        `json:"fsWatcherEnabled"`
	FsWatcherDelayS         int         `json:"fsWatcherDelayS"`
	FsWatcherTimeoutS       int         `json:"fsWatcherTimeoutS"`
	IgnorePerms             bool        `json:"ignorePerms"`
	AutoNormalize           bool        `json:"autoNormalize"`
	MinDiskFree             MinDiskFree `json:"minDiskFree"`
	Versioning              Versioning  `json:"versioning"`
	Copiers                 int         `json:"copiers"`
	PullerMaxPendingKiB     int         `json:"pullerMaxPendingKiB"`
	Hashers                 int         `json:"hashers"`
	Order                   string      `json:"order"`
	IgnoreDelete            bool        `json:"ignoreDelete"`
	ScanProgressIntervalS   int         `json:"scanProgressIntervalS"`
	PullerPauseS            int         `json:"pullerPauseS"`
	MaxConflicts            int         `json:"maxConflicts"`
	DisableSparseFiles      bool        `json:"disableSparseFiles"`
	DisableTempIndexes      bool        `json:"disableTempIndexes"`
	Paused                  bool        `json:"paused"`
	WeakHashThresholdPct    int         `json:"weakHashThresholdPct"`
	MarkerName              string      `json:"markerName"`
	CopyOwnershipFromParent bool        `json:"copyOwnershipFromParent"`
	ModTimeWindowS          int         `json:"modTimeWindowS"`
	MaxConcurrentWrites     int         `json:"maxConcurrentWrites"`
	DisableFsync            bool        `json:"disableFsync"`
	BlockPullOrder          string      `json:"blockPullOrder"`
	CopyRangeMethod         string      `json:"copyRangeMethod"`
	CaseSensitiveFS         bool        `json:"caseSensitiveFS"`
	JunctionsAsDirs         bool        `json:"junctionsAsDirs"`
	SyncOwnership           bool        `json:"syncOwnership"`
	SendOwnership           bool        `json:"sendOwnership"`
	SyncXattrs              bool        `json:"syncXattrs"`
	SendXattrs              bool        `json:"sendXattrs"`
	XattrFilter             XattrFilter `json:"xattrFilter"`
}

type SyncthingFolderStatus struct {
	Errors                        int            `json:"errors"`
	PullErrors                    int            `json:"pullErrors"`
	Invalid                       string         `json:"invalid"`
	GlobalFiles                   int            `json:"globalFiles"`
	GlobalDirectories             int            `json:"globalDirectories"`
	GlobalSymlinks                int            `json:"globalSymlinks"`
	GlobalDeleted                 int            `json:"globalDeleted"`
	GlobalBytes                   int64          `json:"globalBytes"`
	GlobalTotalItems              int            `json:"globalTotalItems"`
	LocalFiles                    int            `json:"localFiles"`
	LocalDirectories              int            `json:"localDirectories"`
	LocalSymlinks                 int            `json:"localSymlinks"`
	LocalDeleted                  int            `json:"localDeleted"`
	LocalBytes                    int64          `json:"localBytes"`
	LocalTotalItems               int            `json:"localTotalItems"`
	NeedFiles                     int            `json:"needFiles"`
	NeedDirectories               int            `json:"needDirectories"`
	NeedSymlinks                  int            `json:"needSymlinks"`
	NeedDeletes                   int            `json:"needDeletes"`
	NeedBytes                     int64          `json:"needBytes"`
	NeedTotalItems                int            `json:"needTotalItems"`
	ReceiveOnlyChangedFiles       int            `json:"receiveOnlyChangedFiles"`
	ReceiveOnlyChangedDirectories int            `json:"receiveOnlyChangedDirectories"`
	ReceiveOnlyChangedSymlinks    int            `json:"receiveOnlyChangedSymlinks"`
	ReceiveOnlyChangedDeletes     int            `json:"receiveOnlyChangedDeletes"`
	ReceiveOnlyChangedBytes       int64          `json:"receiveOnlyChangedBytes"`
	ReceiveOnlyTotalItems         int            `json:"receiveOnlyTotalItems"`
	InSyncFiles                   int            `json:"inSyncFiles"`
	InSyncBytes                   int64          `json:"inSyncBytes"`
	State                         string         `json:"state"`
	StateChanged                  time.Time      `json:"stateChanged"`
	Error                         string         `json:"error"`
	Version                       int            `json:"version"`
	Sequence                      int            `json:"sequence"`
	RemoteSequence                map[string]int `json:"remoteSequence"`
	IgnorePatterns                bool           `json:"ignorePatterns"`
	WatchError                    string         `json:"watchError"`
}

func fetchBytes(url string, apiKey string, foo any) error {

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", apiKey)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip certificate verification
			},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &foo)
	if err != nil {
		return fmt.Errorf("Error unmarshalling JSON: %w", err)
	}

	return nil
}

//	if len(os.Getenv("DEBUG")) > 0 {
//		f, err := tea.LogToFile("debug.log", "debug")
//		if err != nil {
//			fmt.Println("fatal:", err)
//			os.Exit(1)
//		}
//		defer f.Close()
//	}

func mapValues[T any](m map[string]T) []T {
	values := make([]T, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

type Device struct {
	DeviceID           string `json:"deviceID"`
	IntroducedBy       string `json:"introducedBy"`
	EncryptionPassword string `json:"encryptionPassword"`
}

type MinDiskFree struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type VersioningParams struct {
	CleanoutDays string `json:"cleanoutDays"`
}

type Versioning struct {
	Type             string           `json:"type"`
	Params           VersioningParams `json:"params"`
	CleanupIntervalS int              `json:"cleanupIntervalS"`
	FsPath           string           `json:"fsPath"`
	FsType           string           `json:"fsType"`
}

type XattrFilter struct {
	Entries            []string `json:"entries"`
	MaxSingleEntrySize int      `json:"maxSingleEntrySize"`
	MaxTotalSize       int      `json:"maxTotalSize"`
}

type SyncthingEvent struct {
	ID       int       `json:"id"`
	GlobalID int       `json:"globalID"`
	Time     time.Time `json:"time"`
	Type     string    `json:"type"`
}
