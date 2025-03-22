package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/davecgh/go-spew/spew"
	"github.com/dustin/go-humanize"
	zone "github.com/lrstanley/bubblezone"
	"github.com/samber/lo"
)

type errMsg error

// # Useful links
// https://docs.syncthing.net/dev/rest.html#rest-pagination

// TODO create const for syncthing ports paths
// TODO currently we are skipping tls verification for the https://localhost path. What can we do about it

type model struct {
	dump              io.Writer
	loading           bool
	err               error
	width             int
	height            int
	thisDevice        thisDeviceModel
	expandedFolder    map[string]struct{}
	ongoingUserAction bool

	// Syncthing DATA
	syncthingApiKey string
	config          Config
	version         SyncthingSystemVersion
	status          SyncthingSystemStatus
	connections     SyncthingSystemConnections
	folderStats     map[string]FolderStat
	folderStatuses  map[string]SyncthingFolderStatus
}

type thisDeviceModel struct {
	name          string
	deltaBytesIn  int64
	deltaBytesOut int64
	deltaTime     int64
}

// ------------------ constants -----------------------
const (
	REFETCH_STATUS_INTERVAL = 10 * time.Second
	PAUSE_ALL_MARK          = "pause-all"
	RESUME_ALL_MARK         = "resume-all"
)

var VERSION = "unknown"

var quitKeys = key.NewBinding(
	key.WithKeys("q", "esc", "ctrl+c"),
	key.WithHelp("", "press q to quit"),
)

func initModel() model {
	var dump *os.File
	if _, ok := os.LookupEnv("DEBUG"); ok {
		var err error
		dump, err = os.OpenFile("messages.log", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			os.Exit(1)
		}
	}
	syncthingApiKey := os.Getenv("SYNCTHING_API_KEY")

	return model{
		loading:         true,
		syncthingApiKey: syncthingApiKey,
		dump:            dump,
		folderStatuses:  make(map[string]SyncthingFolderStatus),
		expandedFolder:  make(map[string]struct{}),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchConfig(m.syncthingApiKey),
		fetchEvents(m.syncthingApiKey, 0),
		fetchFolderStats(m.syncthingApiKey),
		fetchSystemConnections(m.syncthingApiKey),
		fetchSystemStatus(m.syncthingApiKey),
		fetchSystemVersion(m.syncthingApiKey),
		tea.SetWindowTitle("tui-syncthing"),
		tea.Tick(REFETCH_STATUS_INTERVAL, func(time.Time) tea.Msg { return TickedRefetchStatusMsg{} }),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dump != nil {
		spew.Fdump(m.dump, msg)
	}

	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, quitKeys):
			return m, tea.Quit
		default:
			return m, nil
		}

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
			return m, nil
		}

		if zone.Get(PAUSE_ALL_MARK).InBounds(msg) && !m.ongoingUserAction {
			pausedFolders := lo.Map(m.config.Folders, func(item SyncthingFolderConfig, index int) SyncthingFolderConfig {
				item.Paused = true
				return item
			})
			m.ongoingUserAction = true
			return m, putFolder(m.syncthingApiKey, pausedFolders...)
		}

		if zone.Get(RESUME_ALL_MARK).InBounds(msg) {
			resumedFolders := lo.Map(m.config.Folders, func(item SyncthingFolderConfig, index int) SyncthingFolderConfig {
				item.Paused = false
				return item
			})
			m.ongoingUserAction = true
			return m, putFolder(m.syncthingApiKey, resumedFolders...)
		}

		for _, folder := range m.config.Folders {
			if zone.Get(folder.ID).InBounds(msg) {
				if _, exists := m.expandedFolder[folder.ID]; exists {
					delete(m.expandedFolder, folder.ID)
				} else {
					m.expandedFolder[folder.ID] = struct{}{}
				}
				return m, nil
			}

			if zone.Get(folder.ID+"/pause").InBounds(msg) && !m.ongoingUserAction {
				folder.Paused = !folder.Paused
				m.ongoingUserAction = true
				return m, putFolder(m.syncthingApiKey, folder)
			}

			if zone.Get(folder.ID+"/rescan").InBounds(msg) && !m.ongoingUserAction {
				m.ongoingUserAction = true
				return m, rescanFolder(m.syncthingApiKey, folder.ID)
			}
		}

		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case FetchedEventsMsg:
		if msg.err != nil {
			// TODO figure out what to do if event errors
			m.err = msg.err
		}

		since := 0
		if len(msg.events) > 0 {
			since = msg.events[len(msg.events)-1].ID
		}
		cmds := make([]tea.Cmd, 0)
		for _, e := range msg.events {
			if e.Type == "StateChanged" || e.Type == "FolderPaused" {
				cmds = append(cmds, fetchConfig(m.syncthingApiKey))
				break
			}
		}
		cmds = append(cmds, fetchEvents(m.syncthingApiKey, since))
		return m, tea.Batch(cmds...)

	case FetchedSystemStatusMsg:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}
		m.status = msg.status
		m.thisDevice.name = thisDeviceName(m.status.MyID, m.config.Devices)
		return m, nil

	case FetchedSystemVersionMsg:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}
		m.version = msg.version
		return m, nil
	case FetchedSystemConnectionsMsg:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}

		if m.connections.Total.InBytesTotal != 0 && m.connections.Total.OutBytesTotal != 0 {
			deltaBytesIn := msg.connections.Total.InBytesTotal - m.connections.Total.InBytesTotal
			deltaBytesOut := msg.connections.Total.OutBytesTotal - m.connections.Total.OutBytesTotal
			deltaTime := msg.connections.Total.At.Sub(m.connections.Total.At).Seconds()
			m.thisDevice.deltaBytesIn = deltaBytesIn
			m.thisDevice.deltaBytesOut = deltaBytesOut
			m.thisDevice.deltaTime = int64(deltaTime)
		}
		m.connections = msg.connections
		return m, nil
	case FetchedFolderStats:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}
		m.folderStats = msg.folderStats
		return m, nil

	case TickedRefetchStatusMsg:
		cmds := tea.Batch(
			fetchSystemConnections(m.syncthingApiKey),
			fetchSystemStatus(m.syncthingApiKey),
			tea.Tick(REFETCH_STATUS_INTERVAL, func(time.Time) tea.Msg { return TickedRefetchStatusMsg{} }),
		)
		return m, cmds
	case UserPostPutEndedMsg:
		m.err = msg.err
		m.ongoingUserAction = false

		return m, nil

	case FetchedConfig:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.config = msg.config
		m.thisDevice.name = thisDeviceName(m.status.MyID, msg.config.Devices)
		cmds := lo.Map(m.config.Folders, func(folder SyncthingFolderConfig, index int) tea.Cmd {
			return fetchFolderStatus(m.syncthingApiKey, folder.ID)
		})

		return m, tea.Batch(cmds...)
	case FetchedFolderStatus:
		if msg.err != nil {
			delete(m.folderStatuses, msg.id)
			return m, nil
		}

		m.folderStatuses[msg.id] = msg.folder
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil

	default:
		var cmds []tea.Cmd
		return m, tea.Batch(cmds...)
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

	// if m.loading {
	// 	str := fmt.Sprintf("\n\n   %s Loading syncthing data... %s\n\n", m.spinner.View(), quitKeys.Help().Desc)
	// 	return str
	// }

	folders := lo.Map(m.config.Folders, func(folder SyncthingFolderConfig, index int) FolderWithStatusAndStats {
		status, hasStatus := m.folderStatuses[folder.ID]
		stats, hasStats := m.folderStats[folder.ID]
		return FolderWithStatusAndStats{
			config:    folder,
			status:    status,
			hasStatus: hasStatus,
			stats:     stats,
			hasStats:  hasStats,
		}
	})
	return zone.Scan(lipgloss.NewStyle().MaxHeight(m.height).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			viewFolders(folders, m.config.Devices, m.status.MyID, m.expandedFolder),
			lipgloss.JoinVertical(lipgloss.Left,
				viewStatus(
					m.status,
					m.connections,
					lo.Values(m.folderStatuses),
					m.version,
					m.thisDevice),
				viewDevices(m.config.Devices),
			))))
}

func viewStatus(
	status SyncthingSystemStatus,
	connections SyncthingSystemConnections,
	folders []SyncthingFolderStatus,
	version SyncthingSystemVersion,
	thisDevice thisDeviceModel,
) string {
	foo := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		PaddingRight(1).
		PaddingLeft(1).
		Width(50)
	totalFiles := lo.SumBy(folders, func(f SyncthingFolderStatus) int { return f.LocalFiles })
	totalDirectories := lo.SumBy(folders, func(f SyncthingFolderStatus) int { return f.LocalDirectories })
	totalBytes := lo.SumBy(folders, func(f SyncthingFolderStatus) int64 { return f.LocalBytes })

	var inBytesPerSecond int64
	var outBytesPerSecond int64
	if thisDevice.deltaTime != 0 {
		inBytesPerSecond = thisDevice.deltaBytesIn / thisDevice.deltaTime
		outBytesPerSecond = thisDevice.deltaBytesOut / thisDevice.deltaTime
	}
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Width(foo.GetWidth()-foo.GetHorizontalPadding()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Align(lipgloss.Right)
			}
			return lipgloss.NewStyle()
		}).
		Row(
			"Download rate",
			fmt.Sprintf("%s/s (%s)",
				humanize.IBytes(uint64(inBytesPerSecond)),
				humanize.IBytes(uint64(connections.Total.InBytesTotal)),
			),
		).
		Row("Upload rate",
			fmt.Sprintf("%s/s (%s)",
				humanize.IBytes(uint64(outBytesPerSecond)),
				humanize.IBytes(uint64(connections.Total.OutBytesTotal)),
			),
		).
		Row("Local State (Total)",
			fmt.Sprintf("📄 %d 📁 %d 📁 %s",
				totalFiles,
				totalDirectories,
				humanize.IBytes(uint64(totalBytes))),
		).
		Row("Uptime", HumanizeDuration(int(status.Uptime))).
		Row("Syncthing Version", fmt.Sprintf("%s, %s (%s)", version.Version, osName(version.OS), archName(version.Arch))).
		Row("Version", VERSION)

	return foo.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			thisDevice.name,
			t.Render(),
		),
	)
}

var btnStyle = lipgloss.
	NewStyle().
	Border(lipgloss.RoundedBorder(), true).
	PaddingLeft(1).
	PaddingRight(1)

func viewFolders(
	folders []FolderWithStatusAndStats,
	devices []SyncthingDevice,
	myID string,
	expandedFolder map[string]struct{},
) string {
	views := lo.Map(folders, func(item FolderWithStatusAndStats, index int) string {
		_, isExpanded := expandedFolder[item.config.ID]
		return viewFolder(item, devices, myID, isExpanded)
	})

	btns := make([]string, 0)
	areAllFoldersPaused := lo.EveryBy(folders, func(item FolderWithStatusAndStats) bool { return item.config.Paused })
	anyFolderPaused := lo.SomeBy(folders, func(item FolderWithStatusAndStats) bool { return item.config.Paused })
	if !areAllFoldersPaused {
		btns = append(btns, zone.Mark(PAUSE_ALL_MARK, btnStyle.Render("Pause All")))
	}
	if anyFolderPaused {
		btns = append(btns, zone.Mark(RESUME_ALL_MARK, btnStyle.Render("Resume All")))
	}
	btns = append(btns, zone.Mark("add-folder", btnStyle.Render("Add Folder")))

	views = append(views, (lipgloss.JoinHorizontal(lipgloss.Top, btns...)))

	return lipgloss.JoinVertical(lipgloss.Right, views...)
}

func viewFolder(folder FolderWithStatusAndStats, devices []SyncthingDevice, myID string, expanded bool) string {
	folderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(folderColor(folder)).
		Width(60)

	// TODO this borderless table to so reusable table
	t := table.New().
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		Width(folderStyle.GetWidth()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Align(lipgloss.Right)
			}
			return lipgloss.NewStyle()
		}).
		Row(
			folder.config.Label,
			lipgloss.NewStyle().Foreground(folderColor(folder)).Render(statusLabel(folder)),
		)

	verticalViews := make([]string, 0)
	verticalViews = append(verticalViews, zone.Mark(folder.config.ID, t.Render()))
	if expanded {
		foo := lo.Ternary(folder.config.FsWatcherEnabled, "Enabled", "Disabled")

		sharedDevices := lo.FilterMap(folder.config.Devices, func(sharedDevice FolderDevice, index int) (string, bool) {
			if sharedDevice.DeviceID == myID {
				// folder devices includes the host device. we want to ignore our device
				return "", false
			}
			d, found := lo.Find(devices, func(d SyncthingDevice) bool {
				return d.DeviceID == sharedDevice.DeviceID
			})

			return d.Name, found
		})

		verticalViews = append(verticalViews, table.
			New().
			Border(lipgloss.HiddenBorder()).
			Width(folderStyle.GetWidth()).
			StyleFunc(func(row, col int) lipgloss.Style {
				if col == 1 {
					return lipgloss.NewStyle().Align(lipgloss.Right)
				}
				return lipgloss.NewStyle()
			}).
			Row("Folder ID", folder.config.ID).
			Row("Folder Path", folder.config.Path).
			Row("Folder Type", folder.config.Type). // TODO create custom label
			Row("Global State",
				fmt.Sprintf("📄 %d 📁 %d 📁 %s",
					folder.status.GlobalFiles,
					folder.status.GlobalDirectories,
					humanize.IBytes(uint64(folder.status.GlobalBytes))),
			).
			Row("Local State",
				fmt.Sprintf("📄 %d 📁 %d 📁 %s",
					folder.status.LocalFiles,
					folder.status.LocalDirectories,
					humanize.IBytes(uint64(folder.status.LocalBytes))),
			).
			Row("Rescans ", fmt.Sprintf("%s  %s", HumanizeDuration(folder.config.RescanIntervalS), foo)).
			Row("File Pull Order", fmt.Sprint(folder.config.Order)).
			Row("File Versioning", fmt.Sprint(folder.config.Versioning.Type)).
			Row("Shared With", strings.Join(sharedDevices, ", ")).
			Row("Last Scan", fmt.Sprint(folder.stats.LastScan)).
			Row("Last File", fmt.Sprint(folder.stats.LastFile.Filename)).
			Render())

		footerStyle := lipgloss.
			NewStyle().
			Width(folderStyle.GetWidth()).
			Align(lipgloss.Right)

		pauseBtn := zone.
			Mark(folder.config.ID+"/pause",
				btnStyle.
					Render(lo.Ternary(
						folderState(folder) == Paused,
						"Resume",
						"Pause",
					)))
		rescanBtn := zone.
			Mark(folder.config.ID+"/rescan",
				btnStyle.Render("Rescan"))

		verticalViews = append(verticalViews, footerStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, pauseBtn, rescanBtn)))

	}

	return folderStyle.Render(lipgloss.JoinVertical(lipgloss.Left, verticalViews...))
}

func viewDevices(devices []SyncthingDevice) string {

	views := lo.Map(devices, func(device SyncthingDevice, index int) string {
		return viewDevice(device)
	})

	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

func viewDevice(device SyncthingDevice) string {
	return ""
}

func osName(os string) string {
	switch os {
	case "darwin":
		return "macOs"
	case "dragonfly":
		return "DragonFly BSD"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	case "solaris":
		return "Solaris"
	}

	return "unknown os"
}

func archName(arch string) string {
	switch arch {
	case "386":
		return "32-bit Intel/AMD"
	case "amd64":
		return "64-bit Intel/AMD"
	case "arm":
		return "32-bit ARM"
	case "arm64":
		return "64-bit ARM"
	case "ppc64":
		return "64-bit PowerPC"
	case "ppc64le":
		return "64-bit PowerPC (LE)"
	case "mips":
		return "32-bit MIPS"
	case "mipsle":
		return "32-bit MIPS (LE)"
	case "mips64":
		return "64-bit MIPS"
	case "mips64le":
		return "64-bit MIPS (LE)"
	case "riscv64":
		return "64-bit RISC-V"
	case "s390x":
		return "64-bit z/Architecture"
	}

	return "unknown arch"
}

// Define a custom type for the enum
type FolderState int

// Use iota to define constants for each day
const (
	Idle FolderState = iota
	Syncing
	Error
	Paused
	Unshared
	Scanning
	Unknown
)

func folderState(foo FolderWithStatusAndStats) FolderState {
	if foo.status.State == "syncing" {
		return Syncing
	}
	if foo.status.State == "scanning" {
		return Scanning
	}

	if len(foo.status.Invalid) > 0 || len(foo.status.Error) > 0 {
		return Error
	}

	if foo.config.Paused {
		return Paused
	}

	if len(foo.config.Devices) == 1 {
		return Unshared
	}

	if !foo.hasStatus {
		return Unknown
	}

	return Idle
}

// TODO return colors somehow
func statusLabel(foo FolderWithStatusAndStats) string {
	switch folderState(foo) {
	case Idle:
		return "Up to Date"
	case Scanning:
		return "Scanning"
	case Syncing:
		return "Syncing"
	case Paused:
		return "Paused"
	case Unshared:
		return "Unshared"
	case Error:
		return "Error"
	case Unknown:
		return "Unknown"
	}

	return ""
}

func folderColor(foo FolderWithStatusAndStats) lipgloss.AdaptiveColor {
	switch folderState(foo) {
	case Idle:
		return lipgloss.AdaptiveColor{Light: "#75e4af", Dark: "#75e4af"}
	case Scanning:
		return lipgloss.AdaptiveColor{Light: "#58b5dc", Dark: "#58b5dc"}
	case Syncing:
		return lipgloss.AdaptiveColor{Light: "#58b5dc", Dark: "#58b5dc"}
	case Paused:
		return lipgloss.AdaptiveColor{Light: "", Dark: ""}
	case Unshared:
		return lipgloss.AdaptiveColor{Light: "", Dark: ""}
	case Error:
		return lipgloss.AdaptiveColor{Light: "#ff7092", Dark: "#ff7092"}
	case Unknown:
		return lipgloss.AdaptiveColor{Light: "", Dark: ""}
	}

	return lipgloss.AdaptiveColor{Light: "", Dark: ""}
}

func thisDeviceName(myID string, devices []SyncthingDevice) string {
	for _, device := range devices {
		if device.DeviceID == myID {
			return device.Name
		}
	}

	return "no-name"
}

func main() {
	zone.NewGlobal()
	p := tea.NewProgram(initModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// ------------------------------- MSGS ---------------------------------
type FetchedFolderStatus struct {
	folder SyncthingFolderStatus
	id     string
	err    error
}

type FetchedEventsMsg struct {
	events []SyncthingEvent
	err    error
}

type FetchedSystemStatusMsg struct {
	status SyncthingSystemStatus
	err    error
}

type FetchedSystemVersionMsg struct {
	version SyncthingSystemVersion
	err     error
}

type FetchedSystemConnectionsMsg struct {
	connections SyncthingSystemConnections
	err         error
}

type FetchedConfig struct {
	config Config
	err    error
}

type FetchedFolderStats struct {
	folderStats map[string]FolderStat
	err         error
}

type TickedRefetchStatusMsg struct{}

type UserPostPutEndedMsg struct {
	action string
	err    error
}

func fetchFolderStatus(apiKey string, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderID)
		var statusFolder SyncthingFolderStatus
		err := fetchBytes(
			"http://localhost:8384/rest/db/status?"+params.Encode(),
			apiKey,
			&statusFolder)
		if err != nil {
			return FetchedFolderStatus{err: err}
		}

		return FetchedFolderStatus{folder: statusFolder, id: folderID}
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

func fetchSystemStatus(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var status SyncthingSystemStatus
		err := fetchBytes("http://localhost:8384/rest/system/status", apiKey, &status)
		if err != nil {
			return FetchedSystemStatusMsg{err: err}
		}

		return FetchedSystemStatusMsg{status: status}
	}
}

func fetchSystemVersion(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var version SyncthingSystemVersion
		err := fetchBytes("http://localhost:8384/rest/system/version", apiKey, &version)
		if err != nil {
			return FetchedSystemVersionMsg{err: err}
		}

		return FetchedSystemVersionMsg{version: version}
	}
}

func fetchSystemConnections(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var connections SyncthingSystemConnections
		err := fetchBytes("http://localhost:8384/rest/system/connections", apiKey, &connections)
		if err != nil {
			return FetchedSystemConnectionsMsg{err: err}
		}

		return FetchedSystemConnectionsMsg{connections: connections}
	}
}

func fetchConfig(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var config Config
		err := fetchBytes("http://localhost:8384/rest/config", apiKey, &config)
		if err != nil {
			return FetchedConfig{err: err}
		}

		return FetchedConfig{config: config}
	}
}

func fetchFolderStats(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var folderStats map[string]FolderStat
		err := fetchBytes("http://localhost:8384/rest/stats/folder", apiKey, &folderStats)
		if err != nil {
			return FetchedFolderStats{err: err}
		}

		return FetchedFolderStats{folderStats: folderStats}
	}
}

func putFolder(apiKey string, folders ...SyncthingFolderConfig) tea.Cmd {
	return func() tea.Msg {
		err := put("http://localhost:8384/rest/config/folders/", apiKey, folders)
		ids := strings.Join(lo.Map(folders, func(item SyncthingFolderConfig, index int) string { return item.ID }), ", ")
		return UserPostPutEndedMsg{err: err, action: "putFolder: " + ids}
	}
}

func rescanFolder(apiKey string, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderID)
		err := post("http://localhost:8384/rest/db/scan/"+"?"+params.Encode(), apiKey)
		return UserPostPutEndedMsg{err: err, action: "rescanFolder: " + folderID}
	}
}

type FolderWithStatusAndStats struct {
	config    SyncthingFolderConfig
	status    SyncthingFolderStatus
	hasStatus bool
	stats     FolderStat
	hasStats  bool
}

func fetchBytes(url string, apiKey string, bodyType any) error {
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

	err = json.Unmarshal(body, &bodyType)
	if err != nil {
		return fmt.Errorf("Error unmarshalling JSON: %w", err)
	}

	return nil
}

func put(url string, apiKey string, body any) error {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("Error marshalling JSON: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
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

	return nil
}

func post(url string, apiKey string) error {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
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

	return nil
}
