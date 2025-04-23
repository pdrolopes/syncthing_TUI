package app

import (
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/davecgh/go-spew/spew"
	"github.com/dustin/go-humanize"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pdrolopes/syncthing_TUI/styles"
	"github.com/pdrolopes/syncthing_TUI/syncthing"
	"github.com/samber/lo"
)

type errMsg error

// # Useful links
// https://docs.syncthing.net/dev/rest.html#rest-pagination

// TODO when there a no more bytes to be transfered but still have files to be delete. show as 95%

type model struct {
	dump                           io.Writer
	err                            error
	width                          int
	height                         int
	expandedFields                 map[string]struct{}
	ongoingUserAction              bool
	currentTime                    time.Time
	addDeviceModal                 AddDeviceModel
	confirmRevertLocalChangesModal ConfirmRevertLocalAdditions

	thisDeviceStatus ThisDeviceStatus
	folders          []FolderViewModel

	// http data
	httpData HttpData

	// Syncthing DATA
	config         syncthing.Config // TODO remove this field
	pendingDevices map[string]PendingDevice
	version        syncthing.SystemVersion
	deviceStats    map[string]syncthing.DeviceStats
	completion     map[string]map[string]syncthing.StatusCompletion
}

type FolderViewModel struct {
	Config       syncthing.FolderConfig
	Status       syncthing.FolderStatus
	ExtraStats   syncthing.FolderStats
	ScanProgress syncthing.FolderScanProgressEventData
}

type ThisDeviceStatus struct {
	ID                     string
	Name                   string
	InGoingBytesPerSecond  int64
	OutGoingBytesPerSecond int64
	InBytesTotal           int64
	OutBytesTotal          int64
	UpTime                 int64
}

type PendingDevice struct {
	Address  string
	DeviceID string
	Name     string
	At       time.Time
}

type PendingDeviceList []PendingDevice

func (list PendingDeviceList) Len() int           { return len(list) }
func (list PendingDeviceList) Swap(i, j int)      { list[i], list[j] = list[j], list[i] }
func (list PendingDeviceList) Less(i, j int) bool { return list[i].Name < list[j].Name }

type HttpData struct {
	// TODO think of a better name
	client http.Client
	apiKey string
	url    url.URL
}

type ConfirmRevertLocalAdditions struct {
	Show     bool
	folderID string
}

// ------------------ constants -----------------------
const (
	REFETCH_STATUS_INTERVAL       = 10 * time.Second
	REFETCH_CURRENT_TIME_INTERVAL = time.Second
	PAUSE_ALL_MARK                = "pause-all"
	RESUME_ALL_MARK               = "resume-all"
	RESCAN_ALL_MARK               = "rescan-all"
	ADD_FOLDER_MARK               = "add-folder"
	DEFAULT_SYNCTHING_URL         = "http://localhost:8384"
)

var VERSION = "unknown"

var quitKeys = key.NewBinding(
	key.WithKeys("q", "esc", "ctrl+c"),
	key.WithHelp("", "press q to quit"),
)

func NewModel() model {
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
	httpData := HttpData{
		apiKey: syncthingApiKey,
		client: client,
		url:    *syncthingURL,
	}

	return model{
		httpData:       httpData,
		dump:           dump,
		err:            err,
		expandedFields: make(map[string]struct{}),
		completion:     make(map[string]map[string]syncthing.StatusCompletion),
		pendingDevices: make(map[string]PendingDevice),
		currentTime:    time.Now(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchSystemConnections(m.httpData, syncthing.SystemConnection{}),
		fetchSystemStatus(m.httpData),
		fetchSystemVersion(m.httpData),
		fetchConfig(m.httpData),
		fetchDeviceStats(m.httpData),
		fetchEvents(m.httpData, 0),
		fetchFolderStats(m.httpData),
		fetchPendingDevices(m.httpData),
		tea.SetWindowTitle("tui-syncthing"),
		currentTimeCmd(),
		tea.Tick(
			REFETCH_STATUS_INTERVAL,
			func(time.Time) tea.Msg { return TickedRefetchStatusMsg{} },
		),
	)
}

// ------------------------------- MSGS ---------------------------------
type FetchedFolderStatus struct {
	folderStatus syncthing.FolderStatus
	id           string
	err          error
}

type FetchedEventsMsg struct {
	events []syncthing.Event[any]
	since  int
	err    error
}

type FetchedSystemStatusMsg struct {
	status syncthing.SystemStatus
	err    error
}

type FetchedSystemVersionMsg struct {
	version syncthing.SystemVersion
	err     error
}

type FetchedSystemConnectionsMsg struct {
	prevConnections syncthing.SystemConnection
	connections     syncthing.SystemConnection
	err             error
}

type FetchedConfig struct {
	config syncthing.Config
	err    error
}

type FetchedFolderStats struct {
	folderStats map[string]syncthing.FolderStats
	err         error
}

type FetchedDeviceStats struct {
	deviceStats map[string]syncthing.DeviceStats
	err         error
}

type FetchedCompletion struct {
	deviceID      string
	folderID      string
	completion    syncthing.StatusCompletion
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

type FetchedPendingDevices struct {
	err     error
	devices map[string]syncthing.PendingDeviceInfo
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.dump != nil {
		spew.Fdump(m.dump, msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.addDeviceModal.Show {
			var cmd tea.Cmd
			m.addDeviceModal, cmd = m.addDeviceModal.Update(msg)
			return m, cmd
		}

		if m.confirmRevertLocalChangesModal.Show {
			return handleKeyBoardEventsRevertModal(m, msg)
		}

		switch {
		case key.Matches(msg, quitKeys):
			return m, tea.Quit
		default:
			return m, nil
		}
	case tea.MouseMsg:
		if m.addDeviceModal.Show {
			var cmd tea.Cmd
			m.addDeviceModal, cmd = m.addDeviceModal.Update(msg)
			return m, cmd
		}
		if m.confirmRevertLocalChangesModal.Show {
			return handleMouseEventsRevertModal(m, msg)
		}

		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			return handleMouseLeftClick(m, msg)
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
			return m, wait(10*time.Second, fetchEvents(m.httpData, msg.since))
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
			switch data := e.Data.(type) {
			case syncthing.FolderSummaryEventData:
				m.folders = updateFolderStatus(m.folders, lo.T2(data.Folder, data.Summary))
			case syncthing.Config:
				m.folders = updateFolderViewModelConfigs(data, m.folders)
				m.config = data
			case syncthing.FolderScanProgressEventData:
				m.folders = updateFolderScan(m.folders, data)
			case syncthing.StateChangedEventData:
				if data.To == "scanning" {
					m.folders = updateFolderScan(m.folders, syncthing.FolderScanProgressEventData{})
				}
				if data.From == "scanning" && data.To == "idle" {
					cmds = append(cmds, fetchFolderStats(m.httpData))
				}
			case syncthing.FolderCompletionEventData:
				if _, has := m.completion[data.Device]; !has {
					m.completion[data.Device] = make(map[string]syncthing.StatusCompletion)
				}
				m.completion[data.Device][data.Folder] = syncthing.StatusCompletion{
					Completion:  data.Completion,
					GlobalBytes: data.GlobalBytes,
					GlobalItems: data.GlobalItems,
					NeedBytes:   data.NeedBytes,
					NeedDeletes: data.NeedDeletes,
					NeedItems:   data.NeedItems,
					RemoteState: data.RemoteState,
					Sequence:    data.Sequence,
				}
			case syncthing.PendingDevicesChangedEventData:
				for _, added := range data.Added {
					m.pendingDevices[added.DeviceID] = PendingDevice{
						DeviceID: added.DeviceID,
						Name:     added.Name,
						Address:  added.Address,
						At:       e.Time,
					}
				}
				for _, removed := range data.Removed {
					delete(m.pendingDevices, removed.DeviceID)
				}

			default:
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
		m.thisDeviceStatus.ID = msg.status.MyID
		m.thisDeviceStatus.UpTime = msg.status.Uptime
		m.thisDeviceStatus.Name = thisDeviceName(msg.status.MyID, m.config.Devices)
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

		m.thisDeviceStatus.InBytesTotal = msg.connections.Total.InBytesTotal
		m.thisDeviceStatus.OutBytesTotal = msg.prevConnections.Total.OutBytesTotal
		m.thisDeviceStatus.InGoingBytesPerSecond = byteThroughputInSeconds(
			TotalBytes{
				at:    msg.prevConnections.Total.At,
				bytes: msg.prevConnections.Total.InBytesTotal,
			},
			TotalBytes{
				at:    msg.connections.Total.At,
				bytes: msg.connections.Total.InBytesTotal,
			},
		)

		m.thisDeviceStatus.OutGoingBytesPerSecond = byteThroughputInSeconds(
			TotalBytes{
				at:    msg.prevConnections.Total.At,
				bytes: msg.prevConnections.Total.OutBytesTotal,
			},
			TotalBytes{
				at:    msg.connections.Total.At,
				bytes: msg.connections.Total.OutBytesTotal,
			},
		)

		return m, wait(REFETCH_STATUS_INTERVAL, fetchSystemConnections(m.httpData, msg.connections))
	case FetchedFolderStats:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}

		m.folders = updateFolderStats(m.folders, msg.folderStats)
		return m, nil
	case TickedRefetchStatusMsg:
		cmds := tea.Batch(
			fetchSystemStatus(m.httpData), // TODO remove this
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

			for _, d := range f.Devices {
				cmds = append(cmds, fetchCompletion(m.httpData, d.DeviceID, f.ID))
			}
		}

		m.folders = updateFolderViewModelConfigs(msg.config, m.folders)

		return m, tea.Batch(cmds...)
	case FetchedFolderStatus:
		if msg.err != nil {
			m.folders = updateFolderStatus(m.folders, lo.T2(msg.id, syncthing.FolderStatus{}))
			return m, nil
		}

		m.folders = updateFolderStatus(m.folders, lo.T2(msg.id, msg.folderStatus))
		return m, nil
	case FetchedDeviceStats:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}
		m.deviceStats = msg.deviceStats
		return m, nil
	case FetchedCompletion:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}

		if _, has := m.completion[msg.deviceID]; !has {
			m.completion[msg.deviceID] = make(map[string]syncthing.StatusCompletion)
		}

		if msg.hasCompletion {
			m.completion[msg.deviceID][msg.folderID] = msg.completion
		} else {
			delete(m.completion[msg.deviceID], msg.folderID)
		}

		return m, nil
	case FetchedPendingDevices:
		if msg.err != nil {
			// TODO
			panic(msg.err)
		}

		for deviceID, info := range msg.devices {
			m.pendingDevices[deviceID] = PendingDevice{
				DeviceID: deviceID,
				Name:     info.Name,
				At:       info.Time,
				Address:  info.Address,
			}
		}

		return m, nil

	case TickedCurrentTimeMsg:
		m.currentTime = msg.currentTime
		return m, currentTimeCmd()
	case errMsg:
		m.err = msg
		return m, nil
	default:
		var cmd tea.Cmd
		m.addDeviceModal, cmd = m.addDeviceModal.Update(msg)
		return m, cmd
	}
}

func updateFolderViewModelConfigs(
	config syncthing.Config,
	current []FolderViewModel,
) []FolderViewModel {
	newFolderViewModelList := lo.Map(
		config.Folders,
		func(folderConfig syncthing.FolderConfig, index int) FolderViewModel {
			currentFVM, found := lo.Find(
				current,
				func(fvm FolderViewModel) bool { return folderConfig.ID == fvm.Config.ID },
			)
			if found {
				currentFVM.Config = folderConfig
				return currentFVM
			} else {
				return FolderViewModel{Config: folderConfig}
			}
		},
	)

	return newFolderViewModelList
}

func updateFolderStatus(
	folders []FolderViewModel,
	status lo.Tuple2[string, syncthing.FolderStatus],
) []FolderViewModel {
	return lo.Map(folders, func(item FolderViewModel, index int) FolderViewModel {
		if item.Config.ID == status.A {
			item.Status = status.B
			return item
		} else {
			return item
		}
	})
}

func updateFolderStats(
	folders []FolderViewModel,
	statsDict map[string]syncthing.FolderStats,
) []FolderViewModel {
	return lo.Map(folders, func(item FolderViewModel, index int) FolderViewModel {
		stats, has := statsDict[item.Config.ID]
		if has {
			item.ExtraStats = stats
			return item
		} else {
			return item
		}
	})
}

func updateFolderScan(
	folders []FolderViewModel,
	scanProgress syncthing.FolderScanProgressEventData,
) []FolderViewModel {
	return lo.Map(folders, func(item FolderViewModel, index int) FolderViewModel {
		if item.Config.ID == scanProgress.Folder {
			item.ScanProgress = scanProgress
			return item
		} else {
			return item
		}
	})
}

func handleMouseLeftClick(m model, msg tea.MouseMsg) (model, tea.Cmd) {
	if zone.Get(RESCAN_ALL_MARK).InBounds(msg) {
		cmds := make([]tea.Cmd, 0, len(m.config.Folders))
		for _, f := range m.config.Folders {
			cmds = append(cmds, postScan(m.httpData, f.ID))
		}
		return m, tea.Batch(cmds...)
	}

	if zone.Get(PAUSE_ALL_MARK).InBounds(msg) && !m.ongoingUserAction {
		cmds := make([]tea.Cmd, 0, len(m.config.Folders))
		for _, f := range m.config.Folders {
			cmds = append(cmds, updateFolderPause(m.httpData, f.ID, true))
		}
		m.ongoingUserAction = true
		return m, tea.Batch(cmds...)
	}

	if zone.Get(RESUME_ALL_MARK).InBounds(msg) {
		cmds := make([]tea.Cmd, 0, len(m.config.Folders))
		for _, f := range m.config.Folders {
			cmds = append(cmds, updateFolderPause(m.httpData, f.ID, false))
		}
		m.ongoingUserAction = true
		return m, tea.Batch(cmds...)
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
			m.ongoingUserAction = true
			return m, updateFolderPause(m.httpData, folder.ID, !folder.Paused)
		}

		if zone.Get(folder.ID + "/rescan").InBounds(msg) {
			return m, postScan(m.httpData, folder.ID)
		}

		if zone.Get(folder.ID + "/revert-local-additions").InBounds(msg) {
			m.confirmRevertLocalChangesModal.Show = true
			m.confirmRevertLocalChangesModal.folderID = folder.ID
			return m, nil
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
	for pendingDeviceID := range m.pendingDevices {
		if zone.Get(pendingDeviceID + "/dismiss").InBounds(msg) {
			return m, deletePendingDevice(m.httpData, pendingDeviceID)
		}

		if zone.Get(pendingDeviceID + "/ignore").InBounds(msg) {

			m.config.RemoteIgnoredDevices = append(
				m.config.RemoteIgnoredDevices,
				syncthing.RemoteIgnoredDevice{
					DeviceID: pendingDeviceID,
					Name:     m.pendingDevices[pendingDeviceID].Name,
					Address:  m.pendingDevices[pendingDeviceID].Address,
					Time:     m.currentTime,
				},
			)
			return m, putConfig(m.httpData, m.config)
		}

		if zone.Get(pendingDeviceID + "/add-device").InBounds(msg) {
			m.addDeviceModal = NewPendingDevice(
				m.pendingDevices[pendingDeviceID].Name,
				pendingDeviceID,
				m.config.Defaults.Device,
				m.httpData)
			cmd := m.addDeviceModal.Init()

			return m, cmd
		}
	}

	return m, nil
}

// ------------------ VIEW --------------------------

func (m model) View() string {
	if m.httpData.apiKey == "" {
		return "Missing api key to acess syncthing. Env: SYNCTHING_API_KEY"
	}

	if m.err != nil {
		return m.err.Error()
	}

	devices := lo.Map(
		m.config.Devices,
		func(device syncthing.DeviceConfig, index int) GroupedDeviceData {
			completion, hasCompletion := m.completion[device.DeviceID]
			stats, hasStats := m.deviceStats[device.DeviceID]
			folders := lo.Filter(
				m.config.Folders,
				func(folder syncthing.FolderConfig, index int) bool {
					return lo.SomeBy(
						folder.Devices,
						func(sharedDevice syncthing.FolderDevice) bool {
							return device.DeviceID == sharedDevice.DeviceID
						},
					)
				},
			)
			_, expanded := m.expandedFields[device.DeviceID]
			return GroupedDeviceData{
				config:        device,
				completion:    completion,
				hasCompletion: hasCompletion,
				stats:         stats,
				hasStats:      hasStats,
				hasConnection: false,
				folders:       folders,
				expanded:      expanded,
			}
		},
	)

	pendingDevices := lo.Values(m.pendingDevices)
	sort.Sort(PendingDeviceList(pendingDevices))

	main := lipgloss.NewStyle().MaxHeight(m.height).Render(
		lipgloss.JoinVertical(lipgloss.Center,
			viewPendingDevices(pendingDevices),
			lipgloss.JoinHorizontal(lipgloss.Top,
				viewFolders(m.folders, m.config.Devices, m.thisDeviceStatus.ID, m.expandedFields),
				lipgloss.JoinVertical(lipgloss.Left,
					viewStatus(
						m.thisDeviceStatus,
						m.folders,
						m.version,
						m.config.Options,
					),

					viewDevices(devices, m.currentTime),
				))))

	if m.addDeviceModal.Show {
		modal := m.addDeviceModal.View()

		x := lipgloss.Width(main)/2 - lipgloss.Width(modal)/2
		y := 10
		// TODO verify how to remove double zone.Scan
		return zone.Scan(PlaceOverlay(x, y, modal, main, false))
	}

	if m.confirmRevertLocalChangesModal.Show {
		modal := viewConfirmRevertLocalChangesFolder()

		x := lipgloss.Width(main)/2 - lipgloss.Width(modal)/2
		y := 10
		// TODO verify how to remove double zone.Scan
		return zone.Scan(PlaceOverlay(x, y, modal, main, false))
	}

	return zone.Scan(main)
}

func viewConfirmRevertLocalChangesFolder() string {
	width := 60 // TODO VERIFY MODAL WIDTH
	header := lipgloss.NewStyle().
		Padding(1, 1).
		Width(width).
		Background(styles.ErrorColor).
		Render("Revert Local Changes")
	body := lipgloss.NewStyle().Padding(1, 1).Width(width).Render(`Warning!

The folder content on this device will be overwritten to become identical with other devices. Files newly added here will be deleted.

Are you sure you want to revert all local changes?
`)
	var actions string
	{
		layout := lipgloss.NewStyle().Padding(0, 1).Width(width)
		btnConfirm := zone.Mark("confirm-revert-local-changes", styles.NegativeBtn.Render("Revert"))
		btnCancel := zone.Mark("cancel-revert-local-changes", styles.BtnStyleV2.Render("Cancel"))
		gap := strings.Repeat(
			" ",
			layout.GetWidth()-layout.GetHorizontalPadding()-lipgloss.Width(
				btnConfirm,
			)-lipgloss.Width(
				btnCancel,
			),
		)
		actions = layout.Render(lipgloss.JoinHorizontal(lipgloss.Top, btnConfirm, gap, btnCancel))
	}

	return zone.Mark(
		"revert-local-changes-modal",
		lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render(
			lipgloss.JoinVertical(lipgloss.Left, header, body, actions),
		),
	)
}

func handleMouseEventsRevertModal(m model, msg tea.MouseMsg) (model, tea.Cmd) {
	if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	// click out of modal bounds
	if !zone.Get("revert-local-changes-modal").InBounds(msg) {
		m.confirmRevertLocalChangesModal.Show = false
		m.confirmRevertLocalChangesModal.folderID = ""
		return m, nil
	}

	if zone.Get("confirm-revert-local-changes").InBounds(msg) {
		folderID := m.confirmRevertLocalChangesModal.folderID
		m.confirmRevertLocalChangesModal.folderID = ""
		m.confirmRevertLocalChangesModal.Show = false
		cmd := postRevertChanges(m.httpData, folderID)
		return m, cmd
	}

	if zone.Get("cancel-revert-local-changes").InBounds(msg) {
		m.confirmRevertLocalChangesModal.Show = false
		m.confirmRevertLocalChangesModal.folderID = ""
		return m, nil
	}

	return m, nil
}

func handleKeyBoardEventsRevertModal(m model, msg tea.KeyMsg) (model, tea.Cmd) {
	if msg.Type == tea.KeyEscape {
		m.confirmRevertLocalChangesModal.Show = false
		m.confirmRevertLocalChangesModal.folderID = ""
	}

	if msg.String() == "q" || msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyCtrlD {
		return m, tea.Quit
	}

	return m, nil
}

func viewPendingDevices(pendingDevices []PendingDevice) string {
	if len(pendingDevices) == 0 {
		return ""
	}
	const width = 80
	container := lipgloss.
		NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		Padding(0, 1)

	headerStyle := lipgloss.
		NewStyle().
		Width(container.GetWidth()-container.GetHorizontalPadding()).
		Background(styles.WarningColor).
		Padding(0, 1).
		Foreground(lipgloss.Color("#ffffff"))

	descriptionStyle := lipgloss.
		NewStyle().
		Width(width - 2)
	views := make([]string, 0, len(pendingDevices))
	for _, p := range pendingDevices {
		header := headerStyle.Render(
			spaceAroundTable().Width(width-headerStyle.GetHorizontalPadding()).Row(
				"New Device",
				p.At.String(),
			).Render(),
		)

		description := fmt.Sprintf("Device \"%s\" (%s at %s) wants to connect. Add new device?",
			(p.Name),
			(p.DeviceID),
			p.Address,
		)
		btns := lipgloss.JoinHorizontal(lipgloss.Top,
			zone.Mark(p.DeviceID+"/add-device", styles.PositiveBtn.Render("Add Device")),
			" ",
			zone.Mark(p.DeviceID+"/ignore", styles.NegativeBtn.Render("Ignore")),
			" ",
			zone.Mark(p.DeviceID+"/dismiss", styles.BtnStyleV2.Render("Dismiss")),
		)

		views = append(views, container.Render(lipgloss.JoinVertical(lipgloss.Left,
			header,
			"",
			descriptionStyle.Render(description),
			"",
			lipgloss.PlaceHorizontal(width, lipgloss.Right, btns),
		)))
		views = append(views, "")
	}

	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

func viewStatus(
	this ThisDeviceStatus,
	folders []FolderViewModel,
	version syncthing.SystemVersion,
	options syncthing.Options,
) string {
	foo := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		PaddingRight(1).
		PaddingLeft(1).
		Width(50)

	var totalFiles, totalDirectories, totalBytes int64
	for _, f := range folders {
		totalFiles += int64(f.Status.LocalFiles)
		totalDirectories += int64(f.Status.LocalDirectories)
		totalBytes += f.Status.LocalBytes
	}
	italicStyle := lipgloss.NewStyle().Italic(true).Render

	t := spaceAroundTable().
		Row(
			"Download rate",
			fmt.Sprintf("%s/s (%s)",
				humanize.IBytes(uint64(this.InGoingBytesPerSecond)),
				humanize.IBytes(uint64(this.InBytesTotal)),
			),
		)

	if options.MaxSendKbps > 0 {
		t = t.Row("",
			italicStyle(fmt.Sprintf("Limit: %s/s",
				humanize.IBytes(uint64(options.MaxSendKbps)*humanize.KiByte))))
	}

	t = t.Row("Upload rate",
		fmt.Sprintf("%s/s (%s)",
			humanize.IBytes(uint64(this.OutGoingBytesPerSecond)),
			humanize.IBytes(uint64(this.OutBytesTotal)),
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
		Row("Uptime", HumanizeDuration(this.UpTime)).
		Row("Syncthing Version", fmt.Sprintf("%s, %s (%s)", version.Version, osName(version.OS), archName(version.Arch))).
		Row("Version", VERSION)

	header := lipgloss.NewStyle().PaddingBottom(1).Bold(true).Render(this.Name)
	return foo.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			t.Render(),
		),
	)
}

func viewFolders(
	folders []FolderViewModel,
	devices []syncthing.DeviceConfig,
	myID string,
	expandedFolder map[string]struct{},
) string {
	views := lo.Map(folders, func(item FolderViewModel, index int) string {
		_, isExpanded := expandedFolder[item.Config.ID]
		return viewFolder(item, devices, myID, isExpanded)
	})

	btns := make([]string, 0)
	areAllFoldersPaused := lo.EveryBy(
		folders,
		func(item FolderViewModel) bool { return item.Config.Paused },
	)
	anyFolderPaused := lo.SomeBy(
		folders,
		func(item FolderViewModel) bool { return item.Config.Paused },
	)

	if !areAllFoldersPaused {
		btns = append(btns, zone.Mark(PAUSE_ALL_MARK, styles.BtnStyleV2.Render("Pause All")))
	}
	if anyFolderPaused {
		btns = append(btns, zone.Mark(RESUME_ALL_MARK, styles.BtnStyleV2.Render("Resume All")))
	}
	btns = append(btns, zone.Mark(RESCAN_ALL_MARK, styles.BtnStyleV2.Render("Rescan All")))
	btns = append(btns, zone.Mark(ADD_FOLDER_MARK, styles.BtnStyleV2.Render("Add Folder")))

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

func viewFolder(
	folder FolderViewModel,
	devices []syncthing.DeviceConfig,
	myID string,
	expanded bool,
) string {
	status := folderStatus(folder)
	folderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		PaddingLeft(1).
		PaddingRight(1).
		BorderForeground(folderColor(status)).
		Width(60)
	folderStyleInnerWidth := folderStyle.GetWidth() - folderStyle.GetHorizontalPadding()
	boldStyle := lipgloss.NewStyle().Bold(true)
	var label string
	if folder.Status.NeedBytes > 0 && status == Syncing {
		syncPercent := float64(
			folder.Status.GlobalBytes-folder.Status.NeedBytes,
		) / float64(
			folder.Status.GlobalBytes,
		) * 100
		label = fmt.Sprintf(
			"%s (%.0f%%, %s)",
			folderStatusLabel(status),
			syncPercent,
			humanize.IBytes(uint64(folder.Status.NeedBytes)))
	} else if status == Scanning && folder.ScanProgress.Total > 0 {
		scanPercent := float64(folder.ScanProgress.Current) / float64(folder.ScanProgress.Total) * 100
		label = fmt.Sprintf(
			"%s (%.0f%%)",
			folderStatusLabel(status),
			scanPercent,
		)
	} else {
		label = folderStatusLabel(status)
	}
	header := spaceAroundTable().
		Width(folderStyleInnerWidth).
		Row(
			boldStyle.Render(folder.Config.Label),
			lipgloss.NewStyle().Foreground(folderColor(status)).Bold(true).Render(label),
		)

	verticalViews := make([]string, 0)
	verticalViews = append(verticalViews, zone.Mark(folder.Config.ID, header.Render()))
	if expanded {
		foo := lo.Ternary(folder.Config.FsWatcherEnabled, "Enabled", "Disabled")

		sharedDevices := lo.FilterMap(
			folder.Config.Devices,
			func(sharedDevice syncthing.FolderDevice, index int) (string, bool) {
				if sharedDevice.DeviceID == myID {
					// folder devices includes the host device. we want to ignore our device
					return "", false
				}
				d, found := lo.Find(devices, func(d syncthing.DeviceConfig) bool {
					return d.DeviceID == sharedDevice.DeviceID
				})

				return d.Name, found
			},
		)

		var folderType string
		switch folder.Config.Type {
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
			lo.T2("Folder ID", folder.Config.ID),
			lo.T2("Folder Path", folder.Config.Path),
			lo.T2("Global State",
				fmt.Sprintf("📄 %d 📁 %d 📁 %s",
					folder.Status.GlobalFiles,
					folder.Status.GlobalDirectories,
					humanize.IBytes(uint64(folder.Status.GlobalBytes))),
			),
			lo.T2("Local State",
				fmt.Sprintf("📄 %d 📁 %d 📁 %s",
					folder.Status.LocalFiles,
					folder.Status.LocalDirectories,
					humanize.IBytes(uint64(folder.Status.LocalBytes))),
			),
		}

		var middleRows []RowTuple
		switch status {
		case OutOfSync, Syncing, SyncPrepare:
			middleRows = []RowTuple{lo.T2(
				"Out of Sync Items",
				fmt.Sprintf(
					"%d items, %s",
					folder.Status.NeedFiles,
					humanize.IBytes(uint64(folder.Status.NeedBytes)),
				),
			)}
		case LocalAdditions, LocalUnencrypted:
			middleRows = []RowTuple{lo.T2(
				"Locally Changed Items",
				fmt.Sprintf("%d items, %s",
					folder.Status.ReceiveOnlyChangedFiles,
					humanize.IBytes(uint64(folder.Status.ReceiveOnlyChangedBytes))),
			)}
		case Scanning:
			if folder.ScanProgress.Rate > 0 {
				bytesToBeScanned := folder.ScanProgress.Total - folder.ScanProgress.Current
				secondsETA := int64(float64(bytesToBeScanned) / folder.ScanProgress.Rate)
				middleRows = []RowTuple{lo.T2(
					"Scan Time Remaining",
					ScanDuration(secondsETA),
				)}
			}
		case Idle, FailedItems, Error, Paused, Unknown, Unshared:

		}

		bottomRows := []RowTuple{
			lo.T2("Folder Type", folderType),
			lo.T2(
				"Rescans ",
				fmt.Sprintf("%s  %s", HumanizeDuration(int64(folder.Config.RescanIntervalS)), foo),
			),
			lo.T2("File Pull Order", fmt.Sprint(folder.Config.Order)),
			lo.T2("File Versioning", fmt.Sprint(folder.Config.Versioning.Type)),
			lo.T2("Shared With", strings.Join(sharedDevices, ", ")),
			lo.T2("Last Scan", fmt.Sprint(folder.ExtraStats.LastScan)),
			lo.T2("Last File", fmt.Sprint(folder.ExtraStats.LastFile.Filename)),
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

		var footer string
		{
			revertLocalChangesBtn := zone.Mark(folder.Config.ID+"/revert-local-additions",
				styles.NegativeBtn.Render("Revert Local Changes"))

			pauseBtn := zone.
				Mark(folder.Config.ID+"/pause",
					styles.BtnStyleV2.
						Render(lo.Ternary(
							folderStatus(folder) == Paused,
							"Resume",
							"Pause",
						)))
			rescanBtn := zone.
				Mark(folder.Config.ID+"/rescan",
					styles.BtnStyleV2.Render("Rescan"))

			gap := strings.Repeat(
				" ",
				folderStyleInnerWidth-
					lipgloss.Width(revertLocalChangesBtn)-
					lipgloss.Width(pauseBtn)-
					lipgloss.Width(rescanBtn))

			if status == LocalAdditions || status == LocalUnencrypted {
				footer = lipgloss.JoinHorizontal(
					lipgloss.Top,
					revertLocalChangesBtn,
					gap,
					pauseBtn,
					rescanBtn,
				)
			} else {
				alignRight := lipgloss.NewStyle().Align(lipgloss.Right).Width(folderStyleInnerWidth)
				footer = alignRight.Render(lipgloss.JoinHorizontal(lipgloss.Top, pauseBtn, rescanBtn))
			}
		}

		verticalViews = append(verticalViews, "")
		verticalViews = append(verticalViews, footer)
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
	groupedCompletion := groupCompletion(lo.Values(device.completion)...)

	containerInnerWidth := container.GetWidth() - container.GetHorizontalPadding()
	var deviceStatusLabel string
	if groupedCompletion.Completion != 100 && status == DeviceSyncing {
		deviceStatusLabel = fmt.Sprintf(
			"%s (%d%%, %s)",
			deviceLabel(status),
			groupedCompletion.Completion,
			humanize.IBytes(uint64(groupedCompletion.NeedBytes)))
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
			table.Row("Out of Sync Items", fmt.Sprint(groupedCompletion.NeedItems))
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

type GroupedCompletion struct {
	TotalBytes  int64
	NeedBytes   int64
	NeedItems   int
	NeedDeletes int
	Completion  int
}

func groupCompletion(arg ...syncthing.StatusCompletion) GroupedCompletion {
	foo := GroupedCompletion{}
	for _, c := range arg {
		foo.NeedBytes += c.NeedBytes
		foo.NeedItems += c.NeedItems
		foo.NeedDeletes += c.NeedDeletes
		foo.TotalBytes += c.GlobalBytes
	}
	foo.Completion = int(math.Round(100 * (1.0 - float64(foo.NeedBytes)/float64(foo.TotalBytes))))

	return foo
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

func folderStatus(folder FolderViewModel) FolderStatus {
	if folder.Status.State == "syncing" {
		return Syncing
	}

	if folder.Status.State == "sync-preparing" {
		return SyncPrepare
	}

	if folder.Status.State == "scanning" {
		return Scanning
	}

	if len(folder.Status.Invalid) > 0 || len(folder.Status.Error) > 0 {
		return Error
	}

	if folder.Config.Paused {
		return Paused
	}

	if len(folder.Config.Devices) == 1 {
		return Unshared
	}

	if folder.Status.NeedTotalItems > 0 {
		return OutOfSync
	}

	if (folder.Config.Type == "receiveonly" ||
		folder.Config.Type == "receiveencrypted") &&
		folder.Status.ReceiveOnlyTotalItems > 0 {
		return lo.Ternary(folder.Config.Type == "receiveonly", LocalAdditions, LocalUnencrypted)
	}

	if folder.Status.State == "idle" {
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
		groupedCompletion := groupCompletion(lo.Values(device.completion)...)
		// when all folders are paused. completion doesnt have Completion value.
		// We also check that there isnt any needs things to assert that device is in sync
		needsSomething := groupedCompletion.NeedBytes != 0 ||
			groupedCompletion.NeedItems != 0 ||
			groupedCompletion.NeedDeletes != 0
		return lo.Ternary(
			groupedCompletion.Completion == 100 || !needsSomething,
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

func deviceColor(state DeviceStatus) lipgloss.AdaptiveColor {
	switch state {
	case DeviceDisconnected:
		return styles.Purple
	case DeviceUnusedDisconnected:
		return styles.Purple
	case DeviceDisconnectedInactive:
		return styles.Purple
	case DeviceInSync:
		return styles.SuccessColor
	case DeviceUnusedInSync:
		return styles.SuccessColor
	case DevicePaused:
		return lipgloss.AdaptiveColor{}
	case DeviceUnusedPaused:
		return lipgloss.AdaptiveColor{}
	case DeviceUnknown:
		return lipgloss.AdaptiveColor{}
	case DeviceSyncing:
		return styles.AccentColor
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

func folderColor(status FolderStatus) lipgloss.AdaptiveColor {
	switch status {
	case Idle:
		return styles.SuccessColor
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
		return styles.SuccessColor
	case LocalUnencrypted:
		return styles.SuccessColor
	case Unknown:
		return lipgloss.AdaptiveColor{Light: "", Dark: ""}
	}

	return lipgloss.AdaptiveColor{Light: "", Dark: ""}
}

func thisDeviceName(myID string, devices []syncthing.DeviceConfig) string {
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

type GroupedDeviceData struct {
	config         syncthing.DeviceConfig
	completion     map[string]syncthing.StatusCompletion
	hasCompletion  bool
	stats          syncthing.DeviceStats
	hasStats       bool
	connection     syncthing.Connection
	hasConnection  bool
	prevConnection syncthing.Connection
	folders        []syncthing.FolderConfig
	expanded       bool
}
