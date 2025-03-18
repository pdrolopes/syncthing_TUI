package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	dump           io.Writer
	loading        bool
	err            error
	width          int
	height         int
	thisDevice     thisDeviceModel
	expandedFolder string

	// Syncthing DATA
	syncthingApiKey string
	version         SyncthingSystemVersion
	folders         []SyncthingFolder
	status          SyncthingSystemStatus
	connections     SyncthingSystemConnections
	devices         []SyncthingDevice
	folderStats     map[string]FolderStat
}

type thisDeviceModel struct {
	name          string
	deltaBytesIn  int64
	deltaBytesOut int64
	deltaTime     int64
}

// ------------------ constants -----------------------
const REFETCH_STATUS_INTERVAL = 10 * time.Second

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
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchFolders(m.syncthingApiKey),
		fetchEvents(m.syncthingApiKey, 0),
		fetchSystemStatus(m.syncthingApiKey),
		fetchSystemConnections(m.syncthingApiKey),
		fetchDevices(m.syncthingApiKey),
		fetchFolderStats(m.syncthingApiKey),
		fetchSystemVersion(m.syncthingApiKey),
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

		for i, f := range m.folders {
			if zone.Get(f.config.ID).InBounds(msg) {
				if m.expandedFolder == f.config.ID {
					m.expandedFolder = ""
				} else {
					m.expandedFolder = f.config.ID
				}
				break
			}

			if zone.Get(f.config.ID + "/pause").InBounds(msg) {
				m.folders[i].config.Paused = true
				break
			}
		}

		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case FetchedFoldersMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}

		m.folders = msg.folders
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
				cmds = append(cmds, fetchFolders(m.syncthingApiKey))
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
		m.thisDevice.name = thisDeviceName(m.status.MyID, m.devices)
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
	case FetchedDevicesMsg:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}
		m.devices = msg.devices
		m.thisDevice.name = thisDeviceName(m.status.MyID, m.devices)
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

	case errMsg:
		m.err = msg
		return m, nil

	default:
		var cmds []tea.Cmd
		return m, tea.Batch(cmds...)
	}
}

func thisDeviceName(myID string, devices []SyncthingDevice) string {
	for _, device := range devices {
		if device.DeviceID == myID {
			return device.Name
		}
	}

	return "no-name"
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
	return zone.Scan(lipgloss.NewStyle().MaxHeight(m.height).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			ViewFolders(m.folders, m.folderStats, m.expandedFolder),
			viewStatus(
				m.status,
				m.connections,
				lo.Map(m.folders, func(f SyncthingFolder, _ int) SyncthingFolderStatus { return f.status }),
				m.version,
				m.thisDevice),
		)))
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
		Row("Version", fmt.Sprintf("%s, %s (%s)", version.Version, osName(version.OS), archName(version.Arch)))

	return foo.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			thisDevice.name,
			t.Render(),
		),
	)
}

func ViewFolders(folders []SyncthingFolder, stats map[string]FolderStat, expandedFolder string) string {
	views := lo.Map(folders, func(item SyncthingFolder, index int) string {
		return ViewFolder(item, stats[item.config.ID], item.config.ID == expandedFolder)
	})

	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

func ViewFolder(folder SyncthingFolder, stat FolderStat, expanded bool) string {
	folderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(folderColor(folder)).
		Width(60)

	// TODO this borderless table to so reusable table
	t := table.New().BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).BorderColumn(false).
		Width(folderStyle.GetWidth()).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Align(lipgloss.Right)
			}
			return lipgloss.NewStyle()
		}).Row(folder.config.Label, lipgloss.NewStyle().Foreground(folderColor(folder)).Render(statusLabel(folder)))

	content := ""
	footer := ""
	if expanded {
		foo := lo.Ternary(folder.config.FsWatcherEnabled, "Enabled", "Disabled")

		footerStyle := lipgloss.NewStyle().Width(folderStyle.GetWidth()).Align(lipgloss.Right)
		btnStyle := lipgloss.NewStyle().Border(lipgloss.NormalBorder(), true)

		pauseBtn := zone.Mark(folder.config.ID+"/pause", btnStyle.Render(lo.Ternary(folderState(folder) == Paused, "Resume", "Pause")))

		footer = footerStyle.Render(pauseBtn)

		content = table.New().Border(lipgloss.HiddenBorder()).Width(folderStyle.GetWidth()).StyleFunc(func(row, col int) lipgloss.Style {
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
			Row("Shared With", fmt.Sprint(folder.config.RescanIntervalS)).
			Row("Last Scan", fmt.Sprint(stat.LastScan)).
			Row("Last File", fmt.Sprint(stat.LastFile.Filename)).
			Render()

	}

	return folderStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		zone.Mark(folder.config.ID, t.Render()),
		content,
		footer,
	))
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
	Idle     FolderState = iota // 0
	Syncing                     // 1
	Error                       // 2
	Paused                      // 3
	Unshared                    // 4
	Scanning                    // 5
)

func folderState(foo SyncthingFolder) FolderState {
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

	return Idle
}

// TODO return colors somehow
func statusLabel(foo SyncthingFolder) string {
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
	}

	return ""
}

func folderColor(foo SyncthingFolder) lipgloss.AdaptiveColor {
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
	}

	return lipgloss.AdaptiveColor{Light: "", Dark: ""}
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
type FetchedFoldersMsg struct {
	folders []SyncthingFolder
	err     error
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

type FetchedDevicesMsg struct {
	devices []SyncthingDevice
	err     error
}

type FetchedFolderStats struct {
	folderStats map[string]FolderStat
	err         error
}

type TickedRefetchStatusMsg struct{}

func fetchFolders(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var folders []SyncthingFolderConfig
		err := fetchBytes("http://localhost:8384/rest/config/folders", apiKey, &folders)
		if err != nil {
			return FetchedFoldersMsg{err: err}
		}

		foo := make([]SyncthingFolder, 0, len(folders))
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

			foo = append(foo, SyncthingFolder{
				config: configFolder,
				status: statusFolder,
			})
		}

		return FetchedFoldersMsg{folders: foo}
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

func fetchDevices(apiKey string) tea.Cmd {
	return func() tea.Msg {
		var devices []SyncthingDevice
		err := fetchBytes("http://localhost:8384/rest/config/devices", apiKey, &devices)
		if err != nil {
			return FetchedDevicesMsg{err: err}
		}

		return FetchedDevicesMsg{devices: devices}
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

type SyncthingFolder struct {
	config SyncthingFolderConfig
	status SyncthingFolderStatus
}

func (f SyncthingFolder) FilterValue() string {
	return f.config.Label
}

func (f SyncthingFolder) Title() string {
	return f.config.Label
}

func (f SyncthingFolder) Description() string {
	return statusLabel(f)
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
