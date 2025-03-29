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
// TODO decide folder/device widths and the responsiveness based on terminal width
// TODO create scrollable interface
// TODO when there a no more bytes to be transfered but still have files to be delete. show as 95%

type model struct {
	dump              io.Writer
	loading           bool
	err               error
	width             int
	height            int
	expandedFields    map[string]struct{}
	ongoingUserAction bool
	currentTime       time.Time

	// http data
	httpData HttpData

	// Syncthing DATA
	config           Config
	version          SyncthingSystemVersion
	status           SyncthingSystemStatus
	connections      SyncthingSystemConnections
	prevConnections  SyncthingSystemConnections
	folderStats      map[string]FolderStats
	deviceStats      map[string]DeviceStats
	deviceCompletion map[string]SyncStatusCompletion
	folderStatuses   map[string]SyncthingFolderStatus
}

type HttpData struct {
	// TODO think of a better name
	client http.Client
	apiKey string
	url    url.URL
}

// ------------------ constants -----------------------
const (
	REFETCH_STATUS_INTERVAL       = 10 * time.Second
	REFETCH_CURRENT_TIME_INTERVAL = 1 * time.Minute
	PAUSE_ALL_MARK                = "pause-all"
	RESUME_ALL_MARK               = "resume-all"
	DEFAULT_SYNCTHING_URL         = "http://localhost:8384"

	// Syncthing rest paths
	DB_COMPLETION_PATH = "/rest/db/completion"
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
	envUrl, hasEnv := os.LookupEnv("SYNCTHING_URL")
	if !hasEnv {
		envUrl = DEFAULT_SYNCTHING_URL
	}
	syncthingURL, err := url.Parse(envUrl)
	if err != nil {
		err = fmt.Errorf("invalid syncthing host: %w", err)
	}

	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip certificate verification
			},
		},
	}
	foo := HttpData{
		apiKey: syncthingApiKey,
		client: client,
		url:    *syncthingURL,
	}

	return model{
		loading:          true,
		httpData:         foo,
		dump:             dump,
		err:              err,
		folderStatuses:   make(map[string]SyncthingFolderStatus),
		expandedFields:   make(map[string]struct{}),
		deviceCompletion: make(map[string]SyncStatusCompletion),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchConfig(m.httpData),
		fetchDeviceStats(m.httpData),
		fetchEvents(m.httpData, 0),
		fetchFolderStats(m.httpData),
		fetchSystemConnections(m.httpData),
		fetchSystemStatus(m.httpData),
		fetchSystemVersion(m.httpData),
		tea.SetWindowTitle("tui-syncthing"),
		currentTimeCmd(),
		tea.Tick(REFETCH_STATUS_INTERVAL, func(time.Time) tea.Msg { return TickedRefetchStatusMsg{} }),
	)
}

// ------------------------------- MSGS ---------------------------------
type FetchedFolderStatus struct {
	folder SyncthingFolderStatus
	id     string
	err    error
}

type FetchedEventsMsg struct {
	events []SyncthingEvent
	since  int
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
	folderStats map[string]FolderStats
	err         error
}

type FetchedDeviceStats struct {
	deviceStats map[string]DeviceStats
	err         error
}

type FetchedDeviceCompletion struct {
	deviceID      string
	completion    SyncStatusCompletion
	hasCompletion bool
	err           error
}

type TickedRefetchStatusMsg struct{}

type TickedCurrentTimeMsg struct {
	currentTime time.Time
}

type UserPostPutEndedMsg struct {
	action string
	err    error
}

func fetchFolderStatus(foo HttpData, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderID)
		var statusFolder SyncthingFolderStatus
		err := fetchBytes(
			"http://localhost:8384/rest/db/status?"+params.Encode(),
			foo.apiKey,
			&statusFolder)
		if err != nil {
			return FetchedFolderStatus{err: err}
		}

		return FetchedFolderStatus{folder: statusFolder, id: folderID}
	}
}

func fetchEvents(foo HttpData, since int) tea.Cmd {
	return func() tea.Msg {
		var events []SyncthingEvent
		params := url.Values{}
		params.Add("since", strconv.Itoa(since))
		err := fetchBytes("http://localhost:8384/rest/events?"+params.Encode(), foo.apiKey, &events)
		if err != nil {
			return FetchedEventsMsg{err: err, since: since}
		}

		return FetchedEventsMsg{events: events, since: since}
	}
}

func fetchSystemStatus(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var status SyncthingSystemStatus
		err := fetchBytes("http://localhost:8384/rest/system/status", foo.apiKey, &status)
		if err != nil {
			return FetchedSystemStatusMsg{err: err}
		}

		return FetchedSystemStatusMsg{status: status}
	}
}

func fetchSystemVersion(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var version SyncthingSystemVersion
		err := fetchBytes("http://localhost:8384/rest/system/version", foo.apiKey, &version)
		if err != nil {
			return FetchedSystemVersionMsg{err: err}
		}

		return FetchedSystemVersionMsg{version: version}
	}
}

func fetchSystemConnections(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var connections SyncthingSystemConnections
		err := fetchBytes("http://localhost:8384/rest/system/connections", foo.apiKey, &connections)
		if err != nil {
			return FetchedSystemConnectionsMsg{err: err}
		}

		return FetchedSystemConnectionsMsg{connections: connections}
	}
}

func fetchConfig(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var config Config
		err := fetchBytes("http://localhost:8384/rest/config", foo.apiKey, &config)
		if err != nil {
			return FetchedConfig{err: err}
		}

		return FetchedConfig{config: config}
	}
}

func fetchFolderStats(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var folderStats map[string]FolderStats
		err := fetchBytes("http://localhost:8384/rest/stats/folder", foo.apiKey, &folderStats)
		if err != nil {
			return FetchedFolderStats{err: err}
		}

		return FetchedFolderStats{folderStats: folderStats}
	}
}

func fetchDeviceStats(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var deviceStats map[string]DeviceStats
		err := fetchBytes("http://localhost:8384/rest/stats/device", foo.apiKey, &deviceStats)
		if err != nil {
			return FetchedDeviceStats{err: err}
		}

		return FetchedDeviceStats{deviceStats: deviceStats}
	}
}

func fetchDeviceCompletion(foo HttpData, id string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("device", id)
		fooUrl := foo.url.JoinPath(DB_COMPLETION_PATH)
		fooUrl.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodGet, fooUrl.String(), nil)
		if err != nil {
			return FetchedDeviceCompletion{err: err}
		}

		req.Header.Set("X-API-Key", foo.apiKey)
		resp, err := foo.client.Do(req)
		if err != nil {
			return FetchedDeviceCompletion{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return FetchedDeviceCompletion{}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return FetchedDeviceCompletion{err: err}
		}

		var deviceCompletion SyncStatusCompletion
		err = json.Unmarshal(body, &deviceCompletion)
		if err != nil {
			err = fmt.Errorf("error unmarshalling JSON: %w", err)
			return FetchedDeviceCompletion{err: err}
		}

		return FetchedDeviceCompletion{
			deviceID:      id,
			completion:    deviceCompletion,
			hasCompletion: true,
		}
	}
}

func putFolder(foo HttpData, folders ...SyncthingFolderConfig) tea.Cmd {
	return func() tea.Msg {
		err := put("http://localhost:8384/rest/config/folders/", foo.apiKey, folders)
		ids := strings.Join(lo.Map(folders, func(item SyncthingFolderConfig, index int) string { return item.ID }), ", ")
		return UserPostPutEndedMsg{err: err, action: "putFolder: " + ids}
	}
}

func rescanFolder(foo HttpData, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderID)
		err := post("http://localhost:8384/rest/db/scan/"+"?"+params.Encode(), foo.apiKey)
		return UserPostPutEndedMsg{err: err, action: "rescanFolder: " + folderID}
	}
}

func currentTimeCmd() tea.Cmd {
	return tea.Every(REFETCH_CURRENT_TIME_INTERVAL, func(currentTime time.Time) tea.Msg { return TickedCurrentTimeMsg{currentTime: currentTime} })
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
			return m, putFolder(m.httpData, pausedFolders...)
		}

		if zone.Get(RESUME_ALL_MARK).InBounds(msg) {
			resumedFolders := lo.Map(m.config.Folders, func(item SyncthingFolderConfig, index int) SyncthingFolderConfig {
				item.Paused = false
				return item
			})
			m.ongoingUserAction = true
			return m, putFolder(m.httpData, resumedFolders...)
		}

		for _, folder := range m.config.Folders {
			if zone.Get(folder.ID).InBounds(msg) {
				if _, exists := m.expandedFields[folder.ID]; exists {
					delete(m.expandedFields, folder.ID)
				} else {
					m.expandedFields[folder.ID] = struct{}{}
				}
				return m, nil
			}

			if zone.Get(folder.ID+"/pause").InBounds(msg) && !m.ongoingUserAction {
				folder.Paused = !folder.Paused
				m.ongoingUserAction = true
				return m, putFolder(m.httpData, folder)
			}

			if zone.Get(folder.ID+"/rescan").InBounds(msg) && !m.ongoingUserAction {
				m.ongoingUserAction = true
				return m, rescanFolder(m.httpData, folder.ID)
			}
		}

		for _, device := range m.config.Devices {
			if zone.Get(device.DeviceID).InBounds(msg) {
				if _, exists := m.expandedFields[device.DeviceID]; exists {
					delete(m.expandedFields, device.DeviceID)
				} else {
					m.expandedFields[device.DeviceID] = struct{}{}
				}
				return m, nil
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

		// ignore the first request
		if msg.since == 0 {
			return m, fetchEvents(m.httpData, since)
		}

		cmds := make([]tea.Cmd, 0)
		for _, e := range msg.events {
			if e.Type == "StateChanged" || e.Type == "FolderPaused" {
				cmds = append(cmds, fetchConfig(m.httpData))
				break
			}
		}

		for _, e := range msg.events {
			if e.Type == "DeviceConnected" ||
				e.Type == "DeviceDisconnected" ||
				e.Type == "DeviceDiscovered" ||
				e.Type == "DevicePaused" ||
				e.Type == "DeviceResumed" {
				for _, d := range m.config.Devices {
					cmds = append(cmds, fetchDeviceCompletion(m.httpData, d.DeviceID))
				}
				break
			}
		}
		for _, e := range msg.events {
			if e.Type == "FolderSummary" {
				var foo FolderSummaryEvent
				err := json.Unmarshal(e.Data, &foo)
				if err != nil {
					// TODO figure out how to handle this
					continue
				}
				m.folderStatuses[foo.Folder] = foo.Summary
			}

			if e.Type == "ConfigSaved" {
				var foo Config
				err := json.Unmarshal(e.Data, &foo)
				if err != nil {
					// TODO figure out how to handle this
					continue
				}
				m.config = foo
			}
		}
		cmds = append(cmds, fetchEvents(m.httpData, since))
		return m, tea.Batch(cmds...)
	case FetchedSystemStatusMsg:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}
		m.status = msg.status
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

		m.prevConnections = m.connections
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
			fetchSystemConnections(m.httpData),
			fetchSystemStatus(m.httpData),
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
		cmds := make([]tea.Cmd, 0)
		for _, f := range msg.config.Folders {
			cmds = append(cmds, fetchFolderStatus(m.httpData, f.ID))
		}
		for _, d := range msg.config.Devices {
			cmds = append(cmds, fetchDeviceCompletion(m.httpData, d.DeviceID))
		}

		return m, tea.Batch(cmds...)
	case FetchedFolderStatus:
		if msg.err != nil {
			delete(m.folderStatuses, msg.id)
			return m, nil
		}

		m.folderStatuses[msg.id] = msg.folder
		return m, nil
	case FetchedDeviceStats:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}
		m.deviceStats = msg.deviceStats
		return m, nil
	case FetchedDeviceCompletion:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}

		if msg.hasCompletion {
			m.deviceCompletion[msg.deviceID] = msg.completion
		} else {
			delete(m.deviceCompletion, msg.deviceID)
		}

		return m, nil
	case TickedCurrentTimeMsg:
		m.currentTime = msg.currentTime
		return m, currentTimeCmd()
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

	if m.httpData.apiKey == "" {
		return "Missing api key to acess syncthing. Env: SYNCTHING_API_KEY"
	}

	// if m.loading {
	// 	str := fmt.Sprintf("\n\n   %s Loading syncthing data... %s\n\n", m.spinner.View(), quitKeys.Help().Desc)
	// 	return str
	// }

	folders := lo.Map(m.config.Folders, func(folder SyncthingFolderConfig, index int) GroupedFolderData {
		status, hasStatus := m.folderStatuses[folder.ID]
		stats, hasStats := m.folderStats[folder.ID]
		return GroupedFolderData{
			config:    folder,
			status:    status,
			hasStatus: hasStatus,
			stats:     stats,
			hasStats:  hasStats,
		}
	})

	devices := lo.Map(m.config.Devices, func(device DeviceConfig, index int) GroupedDeviceData {
		completion, hasCompletion := m.deviceCompletion[device.DeviceID]
		stats, hasStats := m.deviceStats[device.DeviceID]
		connection, hasConnection := m.connections.Connections[device.DeviceID]
		prevConnection := m.prevConnections.Connections[device.DeviceID]
		folders := lo.Filter(m.config.Folders, func(folder SyncthingFolderConfig, index int) bool {
			return lo.SomeBy(folder.Devices, func(sharedDevice FolderDevice) bool {
				return device.DeviceID == sharedDevice.DeviceID
			})
		})
		_, expanded := m.expandedFields[device.DeviceID]
		return GroupedDeviceData{
			config:         device,
			completion:     completion,
			hasCompletion:  hasCompletion,
			stats:          stats,
			hasStats:       hasStats,
			connection:     connection,
			hasConnection:  hasConnection,
			prevConnection: prevConnection,
			folders:        folders,
			expanded:       expanded,
		}
	})

	return zone.Scan(lipgloss.NewStyle().MaxHeight(m.height).Render(
		lipgloss.JoinHorizontal(lipgloss.Top,
			viewFolders(folders, m.config.Devices, m.status.MyID, m.expandedFields),
			lipgloss.JoinVertical(lipgloss.Left,
				viewStatus(
					m.status,
					m.connections,
					m.prevConnections,
					lo.Values(m.folderStatuses),
					m.version,
					thisDeviceName(m.status.MyID, m.config.Devices),
					m.config.Options,
				),

				viewDevices(devices, m.currentTime),
			))))
}

func viewStatus(
	status SyncthingSystemStatus,
	connections SyncthingSystemConnections,
	prevConnections SyncthingSystemConnections,
	folders []SyncthingFolderStatus,
	version SyncthingSystemVersion,
	thisDeviceName string,
	options Options,
) string {
	foo := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		PaddingRight(1).
		PaddingLeft(1).
		Width(50)
	totalFiles := lo.SumBy(folders, func(f SyncthingFolderStatus) int { return f.LocalFiles })
	totalDirectories := lo.SumBy(folders, func(f SyncthingFolderStatus) int { return f.LocalDirectories })
	totalBytes := lo.SumBy(folders, func(f SyncthingFolderStatus) int64 { return f.LocalBytes })

	inBytesPerSecond := byteThroughputInSeconds(
		TotalBytes{
			bytes: prevConnections.Total.InBytesTotal,
			at:    prevConnections.Total.At,
		},
		TotalBytes{
			bytes: connections.Total.InBytesTotal,
			at:    connections.Total.At,
		},
	)
	outBytesPerSecond := byteThroughputInSeconds(
		TotalBytes{
			bytes: prevConnections.Total.OutBytesTotal,
			at:    prevConnections.Total.At,
		},
		TotalBytes{
			bytes: connections.Total.OutBytesTotal,
			at:    connections.Total.At,
		},
	)
	italicStyle := lipgloss.NewStyle().Italic(true).Render

	t := spaceAroundTable().
		Row(
			"Download rate",
			fmt.Sprintf("%s/s (%s)",
				humanize.IBytes(uint64(inBytesPerSecond)),
				humanize.IBytes(uint64(connections.Total.InBytesTotal)),
			),
		)

	if options.MaxSendKbps > 0 {
		t = t.Row("",
			italicStyle(fmt.Sprintf("Limit: %s/s",
				humanize.IBytes(uint64(options.MaxSendKbps)*humanize.KiByte))))
	}

	t = t.Row("Upload rate",
		fmt.Sprintf("%s/s (%s)",
			humanize.IBytes(uint64(outBytesPerSecond)),
			humanize.IBytes(uint64(connections.Total.OutBytesTotal)),
		),
	)

	if options.MaxRecvKbps > 0 {
		t = t.Row("",
			italicStyle(
				fmt.Sprintf("Limit: %s/s",
					humanize.IBytes(uint64(options.MaxRecvKbps)*humanize.KiByte))))
	}

	t = t.Row("Local State (Total)",
		fmt.Sprintf("📄 %d 📁 %d 📁 %s",
			totalFiles,
			totalDirectories,
			humanize.IBytes(uint64(totalBytes))),
	).
		Row("Uptime", HumanizeDuration(int(status.Uptime))).
		Row("Syncthing Version", fmt.Sprintf("%s, %s (%s)", version.Version, osName(version.OS), archName(version.Arch))).
		Row("Version", VERSION)

	header := lipgloss.NewStyle().PaddingBottom(1).Bold(true).Render(thisDeviceName)
	return foo.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			header,
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
	folders []GroupedFolderData,
	devices []DeviceConfig,
	myID string,
	expandedFolder map[string]struct{},
) string {
	views := lo.Map(folders, func(item GroupedFolderData, index int) string {
		_, isExpanded := expandedFolder[item.config.ID]
		return viewFolder(item, devices, myID, isExpanded)
	})

	btns := make([]string, 0)
	areAllFoldersPaused := lo.EveryBy(folders, func(item GroupedFolderData) bool { return item.config.Paused })
	anyFolderPaused := lo.SomeBy(folders, func(item GroupedFolderData) bool { return item.config.Paused })
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

func spaceAroundTable() *table.Table {
	return table.New().
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Align(lipgloss.Right)
			}
			return lipgloss.NewStyle()
		})
}

func viewFolder(folder GroupedFolderData, devices []DeviceConfig, myID string, expanded bool) string {
	status := folderStatus(folder)
	folderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		PaddingLeft(1).
		PaddingRight(1).
		BorderForeground(folderColor(folder)).
		Width(60)
	folderStyleInnerWidth := folderStyle.GetWidth() - folderStyle.GetHorizontalPadding()
	boldStyle := lipgloss.NewStyle().Bold(true)
	var label string
	if folder.status.NeedBytes > 0 && status == Syncing {
		syncPercent := float64(folder.status.GlobalBytes-folder.status.NeedBytes) / float64(folder.status.GlobalBytes) * 100
		label = fmt.Sprintf(
			"%s (%.0f%%, %s)",
			folderStatusLabel(status),
			syncPercent,
			humanize.IBytes(uint64(folder.status.NeedBytes)))
	} else {
		label = folderStatusLabel(status)
	}
	header := spaceAroundTable().
		Width(folderStyleInnerWidth).
		Row(
			boldStyle.Render(folder.config.Label),
			lipgloss.NewStyle().Foreground(folderColor(folder)).Bold(true).Render(label),
		)

	verticalViews := make([]string, 0)
	verticalViews = append(verticalViews, zone.Mark(folder.config.ID, header.Render()))
	if expanded {
		foo := lo.Ternary(folder.config.FsWatcherEnabled, "Enabled", "Disabled")

		sharedDevices := lo.FilterMap(folder.config.Devices, func(sharedDevice FolderDevice, index int) (string, bool) {
			if sharedDevice.DeviceID == myID {
				// folder devices includes the host device. we want to ignore our device
				return "", false
			}
			d, found := lo.Find(devices, func(d DeviceConfig) bool {
				return d.DeviceID == sharedDevice.DeviceID
			})

			return d.Name, found
		})

		var folderType string
		switch folder.config.Type {
		case "receiveonly":
			folderType = "Receive Only"
		case "sendreceive":
			folderType = "Send and Receive"
		case "sendonly":
			folderType = "Send Only"
		default:
			folderType = "unknown"
		}

		type RowTuple = lo.Tuple2[string, string]

		topRows := []RowTuple{
			lo.T2("Folder ID", folder.config.ID),
			lo.T2("Folder Path", folder.config.Path),
			lo.T2("Global State",
				fmt.Sprintf("📄 %d 📁 %d 📁 %s",
					folder.status.GlobalFiles,
					folder.status.GlobalDirectories,
					humanize.IBytes(uint64(folder.status.GlobalBytes))),
			),
			lo.T2("Local State",
				fmt.Sprintf("📄 %d 📁 %d 📁 %s",
					folder.status.LocalFiles,
					folder.status.LocalDirectories,
					humanize.IBytes(uint64(folder.status.LocalBytes))),
			),
		}

		var middleRows []RowTuple
		switch status {
		case OutOfSync, Syncing, SyncPrepare:
			middleRows = []RowTuple{lo.T2(
				"Out of Sync Items",
				fmt.Sprintf("%d items, %s", folder.status.NeedFiles, humanize.IBytes(uint64(folder.status.NeedBytes))),
			)}
		case LocalAdditions, LocalUnencrypted:
			middleRows = []RowTuple{lo.T2(
				"Locally Changed Items",
				fmt.Sprintf("%d items, %s",
					folder.status.ReceiveOnlyChangedFiles,
					humanize.IBytes(uint64(folder.status.ReceiveOnlyChangedBytes))),
			)}
		case Scanning, Idle, FailedItems, Error, Paused, Unknown, Unshared:

		}

		bottomRows := []RowTuple{
			lo.T2("Folder Type", folderType),
			lo.T2("Rescans ", fmt.Sprintf("%s  %s", HumanizeDuration(folder.config.RescanIntervalS), foo)),
			lo.T2("File Pull Order", fmt.Sprint(folder.config.Order)),
			lo.T2("File Versioning", fmt.Sprint(folder.config.Versioning.Type)),
			lo.T2("Shared With", strings.Join(sharedDevices, ", ")),
			lo.T2("Last Scan", fmt.Sprint(folder.stats.LastScan)),
			lo.T2("Last File", fmt.Sprint(folder.stats.LastFile.Filename)),
		}

		bar := spaceAroundTable().Width(folderStyleInnerWidth)
		for _, r := range topRows {
			bar = bar.Row(r.Unpack())
		}
		for _, r := range middleRows {
			bar = bar.Row(r.Unpack())
		}
		for _, r := range bottomRows {
			bar = bar.Row(r.Unpack())
		}
		verticalViews = append(verticalViews, bar.Render())

		footerStyle := lipgloss.
			NewStyle().
			Width(folderStyleInnerWidth).
			Align(lipgloss.Right)

		pauseBtn := zone.
			Mark(folder.config.ID+"/pause",
				btnStyle.
					Render(lo.Ternary(
						folderStatus(folder) == Paused,
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

func viewDevices(devices []GroupedDeviceData, currentTime time.Time) string {
	views := lo.Map(devices, func(device GroupedDeviceData, index int) string {
		return viewDevice(device, currentTime)
	})

	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

func viewDevice(device GroupedDeviceData, currentTime time.Time) string {
	status := deviceStatus(device, currentTime)
	color := deviceColor(status)
	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		PaddingLeft(1).
		PaddingRight(1).
		Width(50).
		BorderForeground(color)

	containerInnerWidth := container.GetWidth() - container.GetHorizontalPadding()
	var deviceStatusLabel string
	if int(device.completion.Completion) != 100 && status == DeviceSyncing {
		deviceStatusLabel = fmt.Sprintf(
			"%s (%d%%, %s)",
			deviceLabel(status),
			int(device.completion.Completion),
			humanize.IBytes(uint64(device.completion.NeedBytes)))
	} else {
		deviceStatusLabel = deviceLabel(status)
	}

	header := lipgloss.NewStyle().Bold(true).Render(
		zone.Mark(device.config.DeviceID, spaceAroundTable().Width(containerInnerWidth).
			Row(device.config.Name,
				lipgloss.
					NewStyle().
					Foreground(color).
					Render(deviceStatusLabel)).
			Render()),
	)

	if !device.expanded {
		return container.Render(header)
	}

	sharedFolders := make([]string, 0, len(device.folders))
	for _, f := range device.folders {
		sharedFolders = append(sharedFolders, f.Label)
	}
	inBytesPerSecond := byteThroughputInSeconds(
		TotalBytes{
			bytes: device.prevConnection.InBytesTotal,
			at:    device.prevConnection.At,
		},
		TotalBytes{
			bytes: device.connection.InBytesTotal,
			at:    device.connection.At,
		})

	outBytesPerSecond := byteThroughputInSeconds(
		TotalBytes{
			bytes: device.prevConnection.OutBytesTotal,
			at:    device.prevConnection.At,
		},
		TotalBytes{
			bytes: device.connection.OutBytesTotal,
			at:    device.connection.At,
		})

	table := spaceAroundTable().
		Width(containerInnerWidth)
	if device.connection.Connected {
		table.Row("Download Rate",
			fmt.Sprintf("%s/s (%s)",
				humanize.IBytes(uint64(inBytesPerSecond)),
				humanize.IBytes(uint64(device.connection.InBytesTotal)),
			),
		).
			Row("Upload Rate",
				fmt.Sprintf("%s/s (%s)",
					humanize.IBytes(uint64(outBytesPerSecond)),
					humanize.IBytes(uint64(device.connection.OutBytesTotal)),
				),
			)
		if status == DeviceSyncing {
			table.Row("Out of Sync Items", fmt.Sprint(device.completion.NeedItems))
		}
	} else {
		table.
			Row("Last Seen", device.stats.LastSeen.String()).
			Row("Sync Status", device.stats.LastSeen.String())
	}
	table.Row("Address", device.connection.Address).
		Row("Compresson", device.config.Compression).
		Row("Identification", shortIdentification(device.config.DeviceID)).
		Row("Version", (device.connection.ClientVersion)).
		Row("Folders", strings.Join(sharedFolders, ", ")).
		Render()
	content := table.Render()

	return container.Render(lipgloss.JoinVertical(lipgloss.Left, header, content))
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

func shortIdentification(id string) string {
	dashIndex := strings.Index(id, "-")
	return strings.ToUpper(id[0:dashIndex])
}

type FolderStatus int

const (
	Idle FolderStatus = iota
	SyncPrepare
	Syncing
	Error
	Paused
	Unshared
	Scanning
	OutOfSync
	FailedItems
	LocalAdditions
	LocalUnencrypted
	Unknown
)

func folderStatus(foo GroupedFolderData) FolderStatus {
	if foo.status.State == "syncing" {
		return Syncing
	}

	if foo.status.State == "sync-preparing" {
		return SyncPrepare
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

	if foo.status.NeedTotalItems > 0 {
		return OutOfSync
	}

	if (foo.config.Type == "receiveonly" ||
		foo.config.Type == "receiveencrypted") &&
		foo.status.ReceiveOnlyTotalItems > 0 {
		return lo.Ternary(foo.config.Type == "receiveonly", LocalAdditions, LocalUnencrypted)
	}

	if foo.status.State == "idle" {
		return Idle
	}

	return Unknown
}

type DeviceStatus int

const (
	DeviceDisconnected DeviceStatus = iota
	DeviceDisconnectedInactive
	DeviceInSync
	DevicePaused
	DeviceUnusedDisconnected
	DeviceUnusedInSync
	DeviceUnusedPaused
	DeviceSyncing
	DeviceUnknown
)

func deviceStatus(device GroupedDeviceData, currentTime time.Time) DeviceStatus {
	isUnused := len(device.folders) == 0

	if !device.hasConnection {
		return DeviceUnknown
	}

	if device.config.Paused {
		return lo.Ternary(isUnused, DeviceUnusedPaused, DevicePaused)
	}
	if device.connection.Connected {
		insync := lo.Ternary(isUnused, DeviceUnusedInSync, DeviceInSync)
		// when all folders are paused. completion doesnt have Completion value.
		// We also check that there isnt any needs things to assert that device is in sync
		needsSomething := device.completion.NeedBytes != 0 &&
			device.completion.NeedItems != 0 &&
			device.completion.NeedDeletes != 0
		return lo.Ternary(
			int(device.completion.Completion) == 100 || !needsSomething,
			insync,
			DeviceSyncing)
	}

	lastSeenDays := currentTime.Sub(device.stats.LastSeen).Hours() / 24

	if !isUnused && lastSeenDays > 7 {
		return DeviceDisconnectedInactive
	} else {
		return lo.Ternary(isUnused, DeviceUnusedDisconnected, DeviceDisconnected)
	}
}

func deviceLabel(state DeviceStatus) string {
	switch state {
	case DeviceDisconnected:
		return "Disconnected"
	case DeviceDisconnectedInactive:
		return "Disconnected (Inative)"
	case DeviceInSync:
		return "Up to Date"
	case DevicePaused:
		return "Paused"
	case DeviceUnusedDisconnected:
		return "Disconnected (Unused)"
	case DeviceUnusedInSync:
		return "Connected (Unused)"
	case DeviceUnusedPaused:
		return "Paused (Unused)"
	case DeviceSyncing:
		return "Syncing"
	case DeviceUnknown:
		return "Unknown"
	}

	return "Unknown"
}

var (
	// Adaptive colors for light/dark themes
	// primaryColor   = lipgloss.AdaptiveColor{Light: "#1a1a1a", Dark: "#f0f0f0"}
	// secondaryColor = lipgloss.AdaptiveColor{Light: "#4a4a4a", Dark: "#d0d0d0"}
	accentColor  = lipgloss.AdaptiveColor{Light: "#005f87", Dark: "#00afd7"}
	successColor = lipgloss.AdaptiveColor{Light: "#008700", Dark: "#00d700"}
	// warningColor   = lipgloss.AdaptiveColor{Light: "#af8700", Dark: "#ffd700"}
	// errorColor     = lipgloss.AdaptiveColor{Light: "#d70000", Dark: "#ff0000"}
	// highlightColor = lipgloss.AdaptiveColor{Light: "#ffd700", Dark: "#ffaf00"}
	// mutedColor     = lipgloss.AdaptiveColor{Light: "#6c757d", Dark: "#adb5bd"}
	purple = lipgloss.AdaptiveColor{Light: "#6920e8", Dark: "#8454fc"}
)

func deviceColor(state DeviceStatus) lipgloss.AdaptiveColor {
	switch state {
	case DeviceDisconnected:
		return purple
	case DeviceUnusedDisconnected:
		return purple
	case DeviceDisconnectedInactive:
		return purple
	case DeviceInSync:
		return successColor
	case DeviceUnusedInSync:
		return successColor
	case DevicePaused:
		return lipgloss.AdaptiveColor{}
	case DeviceUnusedPaused:
		return lipgloss.AdaptiveColor{}
	case DeviceUnknown:
		return lipgloss.AdaptiveColor{}
	case DeviceSyncing:
		return accentColor
	}

	return lipgloss.AdaptiveColor{}
}

func folderStatusLabel(foo FolderStatus) string {
	switch foo {
	case Idle:
		return "Up to Date"
	case Scanning:
		return "Scanning"
	case Syncing, SyncPrepare:
		return "Syncing"
	case Paused:
		return "Paused"
	case Unshared:
		return "Unshared"
	case Error:
		return "Error"
	case OutOfSync:
		return "Out of Sync"
	case FailedItems:
		return "Failed Items"
	case LocalAdditions:
		return "Local Additions"
	case LocalUnencrypted:
		return "Local Unencrypted"
	case Unknown:
		return "Unknown"
	}

	return ""
}

func folderColor(foo GroupedFolderData) lipgloss.AdaptiveColor {
	switch folderStatus(foo) {
	case Idle:
		return successColor
	case Scanning:
		return lipgloss.AdaptiveColor{Light: "#58b5dc", Dark: "#58b5dc"}
	case Syncing, SyncPrepare:
		return lipgloss.AdaptiveColor{Light: "#58b5dc", Dark: "#58b5dc"}
	case Paused:
		return lipgloss.AdaptiveColor{Light: "", Dark: ""}
	case Unshared:
		return lipgloss.AdaptiveColor{Light: "", Dark: ""}
	case Error:
		return lipgloss.AdaptiveColor{Light: "#ff7092", Dark: "#ff7092"}
	case OutOfSync:
		return lipgloss.AdaptiveColor{Light: "#ff7092", Dark: "#ff7092"}
	case FailedItems:
		return lipgloss.AdaptiveColor{Light: "#ff7092", Dark: "#ff7092"}
	case LocalAdditions:
		return successColor
	case LocalUnencrypted:
		return successColor
	case Unknown:
		return lipgloss.AdaptiveColor{Light: "", Dark: ""}
	}

	return lipgloss.AdaptiveColor{Light: "", Dark: ""}
}

func thisDeviceName(myID string, devices []DeviceConfig) string {
	for _, device := range devices {
		if device.DeviceID == myID {
			return device.Name
		}
	}

	return "no-name"
}

type TotalBytes struct {
	bytes int64
	at    time.Time
}

func byteThroughputInSeconds(before, after TotalBytes) int64 {
	if before.bytes == 0 {
		return 0
	}
	deltaBytes := after.bytes - before.bytes
	deltaTime := int64(after.at.Sub(before.at).Seconds())

	if deltaTime == 0 {
		return 0
	}

	return deltaBytes / deltaTime
}

type GroupedFolderData struct {
	config    SyncthingFolderConfig
	status    SyncthingFolderStatus
	hasStatus bool
	stats     FolderStats
	hasStats  bool
}

type GroupedDeviceData struct {
	config         DeviceConfig
	completion     SyncStatusCompletion
	hasCompletion  bool
	stats          DeviceStats
	hasStats       bool
	connection     Connection
	hasConnection  bool
	prevConnection Connection
	folders        []SyncthingFolderConfig
	expanded       bool
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
		return fmt.Errorf("error unmarshalling JSON: %w", err)
	}

	return nil
}

func put(url string, apiKey string, body any) error {
	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
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

func main() {
	zone.NewGlobal()
	p := tea.NewProgram(initModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
