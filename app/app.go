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

// ------------------ constants -----------------------
const (
	DEFAULT_SYNCTHING_URL            = "http://localhost:8384"
	REFETCH_STATUS_INTERVAL          = 10 * time.Second
	REFETCH_CURRENT_TIME_INTERVAL    = time.Second
	PAUSE_ALL_MARK                   = "pause-all"
	RESUME_ALL_MARK                  = "resume-all"
	RESCAN_ALL_MARK                  = "rescan-all"
	ADD_FOLDER_MARK                  = "add-folder"
	REVERT_LOCAL_CHANGES_MODAL_AREA  = "revert-local-changes-modal"
	REVERT_LOCAL_CHANGES_CONFIRM_BTN = "confirm-revert-local-changes"
	REVERT_LOCAL_CHANGES_CANCEL_BTN  = "cancel-revert-local-changes"
)

var VERSION = "unknown"

type errMsg error

// # Useful links
// https://docs.syncthing.net/dev/rest.html#rest-pagination

// TODO when there a no more bytes to be transfered but still have files to be delete. show as 95%

type model struct {
	dump                           io.Writer
	err                            error
	width                          int
	height                         int
	httpData                       HttpData
	expandedFields                 map[string]struct{}
	ongoingUserAction              bool
	currentTime                    time.Time
	addDeviceModal                 AddDeviceModel
	confirmRevertLocalChangesModal ConfirmRevertLocalAdditions
	putConfig                      PutConfig

	thisDeviceStatus ThisDeviceStatus
	folders          []FolderViewModel
	devices          []DeviceViewModel

	// Syncthing DATA
	configDefaults syncthing.Defaults
	pendingDevices map[string]PendingDevice
	version        syncthing.SystemVersion
}

type FolderViewModel struct {
	Config        syncthing.FolderConfig
	Status        syncthing.FolderStatus
	ExtraStats    syncthing.FolderStats
	ScanProgress  syncthing.FolderScanProgressEventData
	SharedDevices []string
}

func (fvm FolderViewModel) TogglePauseMark() string {
	return fvm.Config.ID + "-toggle-pause"
}

func (fvm FolderViewModel) RescanMark() string {
	return fvm.Config.ID + "-rescan"
}

func (fvm FolderViewModel) HeaderMark() string {
	return fvm.Config.ID + "-header"
}

func (fvm FolderViewModel) RevertLocalAdditionsMark() string {
	return fvm.Config.ID + "-revert-local-additions"
}

type DeviceViewModel struct {
	Config                 syncthing.DeviceConfig
	ExtraStats             syncthing.DeviceStats
	Connection             lo.Tuple2[bool, syncthing.Connection]
	StatusCompletion       map[string]syncthing.StatusCompletion
	Folders                []lo.Tuple2[string, string]
	InGoingBytesPerSecond  int64
	OutGoingBytesPerSecond int64
}

func (fvm DeviceViewModel) HeaderMark() string {
	return fvm.Config.DeviceID + "-header"
}

type ThisDeviceStatus struct {
	ID                     string
	Name                   string
	InGoingBytesPerSecond  int64
	OutGoingBytesPerSecond int64
	InBytesTotal           int64
	OutBytesTotal          int64
	UpTime                 int64
	MaxSendKbps            int
	MaxRecvKbps            int
}

type PendingDevice struct {
	Address  string
	DeviceID string
	Name     string
	At       time.Time
}

func (pd PendingDevice) DismissMark() string {
	return pd.DeviceID + "/dismiss"
}

func (pd PendingDevice) IgnoreMark() string {
	return pd.DeviceID + "/ignore"
}

func (pd PendingDevice) AddMark() string {
	return pd.DeviceID + "/add-device"
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
		pendingDevices: make(map[string]PendingDevice),
		currentTime:    time.Now(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Sequence(
		tea.SetWindowTitle("tui-syncthing"),
		fetchSystemStatus(m.httpData),
		fetchConfig(m.httpData),
		tea.Batch(
			fetchSystemConnections(m.httpData, syncthing.SystemConnection{}),
			fetchSystemVersion(m.httpData),
			fetchEvents(m.httpData, 0),
			fetchDeviceStats(m.httpData),
			fetchFolderStats(m.httpData),
			fetchPendingDevices(m.httpData),
			currentTimeCmd(),
		))
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
			return m, wait(time.Second, fetchEvents(m.httpData, msg.since))
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
				m.putConfig = createPutConfig(data)
				m.folders = updateFolderViewModelConfigs(data, m.folders, m.thisDeviceStatus.ID)
				m.devices = updateDeviceViewModelConfigs(data, m.devices, m.thisDeviceStatus.ID)
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
				updateDeviceStatusCompletion(m.devices, data.Device, data.Folder,
					syncthing.StatusCompletion{
						Completion:  data.Completion,
						GlobalBytes: data.GlobalBytes,
						GlobalItems: data.GlobalItems,
						NeedBytes:   data.NeedBytes,
						NeedDeletes: data.NeedDeletes,
						NeedItems:   data.NeedItems,
						RemoteState: data.RemoteState,
						Sequence:    data.Sequence,
					},
				)
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
			return m, wait(REFETCH_STATUS_INTERVAL, fetchSystemStatus(m.httpData))
		}
		m.thisDeviceStatus.ID = msg.status.MyID
		m.thisDeviceStatus.UpTime = msg.status.Uptime
		return m, wait(REFETCH_STATUS_INTERVAL, fetchSystemStatus(m.httpData))
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
		m.thisDeviceStatus.InGoingBytesPerSecond, m.thisDeviceStatus.OutGoingBytesPerSecond = calcInOutBytes(
			msg.prevConnections.Total,
			msg.connections.Total,
		)

		{
			devices := make([]DeviceViewModel, 0, len(m.devices))
			for _, device := range m.devices {
				device.InGoingBytesPerSecond, device.OutGoingBytesPerSecond = calcInOutBytes(
					msg.prevConnections.Connections[device.Config.DeviceID],
					msg.connections.Connections[device.Config.DeviceID])
				connection, has := msg.connections.Connections[device.Config.DeviceID]
				device.Connection = lo.T2(has, connection)
				devices = append(devices, device)
			}
			m.devices = devices
		}

		return m, wait(REFETCH_STATUS_INTERVAL, fetchSystemConnections(m.httpData, msg.connections))
	case FetchedFolderStats:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}

		m.folders = updateFolderStats(m.folders, msg.folderStats)
		return m, nil
	case UserPostPutEndedMsg:
		m.err = msg.err
		m.ongoingUserAction = false

		return m, nil
	case FetchedConfig:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		cmds := make([]tea.Cmd, 0)
		for _, f := range msg.config.Folders {
			cmds = append(cmds, fetchFolderStatus(m.httpData, f.ID))

			for _, d := range f.Devices {
				cmds = append(cmds, fetchCompletion(m.httpData, d.DeviceID, f.ID))
			}
		}

		m.putConfig = createPutConfig(msg.config)
		m.folders = updateFolderViewModelConfigs(msg.config, m.folders, m.thisDeviceStatus.ID)
		m.devices = updateDeviceViewModelConfigs(msg.config, m.devices, m.thisDeviceStatus.ID)
		m.thisDeviceStatus.Name = thisDeviceName(m.thisDeviceStatus.ID, msg.config)
		m.thisDeviceStatus.MaxSendKbps = msg.config.Options.MaxSendKbps
		m.thisDeviceStatus.MaxRecvKbps = msg.config.Options.MaxRecvKbps

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
		m.devices = updateDeviceExtraStats(m.devices, msg.deviceStats)
		return m, nil
	case FetchedCompletion:
		if msg.err != nil {
			// TODO create system status error ux
			m.err = msg.err
			return m, nil
		}

		if msg.hasCompletion {
			updateDeviceStatusCompletion(m.devices, msg.deviceID, msg.folderID, msg.completion)
		} else {
			updateDeviceStatusCompletion(m.devices, msg.deviceID, msg.folderID, syncthing.StatusCompletion{})
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
	thisDeviceID string,
) []FolderViewModel {
	newFolderViewModelList := lo.Map(
		config.Folders,
		func(folderConfig syncthing.FolderConfig, index int) FolderViewModel {
			currentFVM, found := lo.Find(
				current,
				func(fvm FolderViewModel) bool { return folderConfig.ID == fvm.Config.ID },
			)

			sharedDevices := lo.FilterMap(
				folderConfig.Devices,
				func(device syncthing.FolderDevice, index int) (string, bool) {
					if device.DeviceID == thisDeviceID {
						return "", false
					}

					for _, d := range config.Devices {
						if d.DeviceID == device.DeviceID {
							return d.Name, true
						}
					}
					return "", false
				},
			)

			if found {
				currentFVM.Config = folderConfig
				currentFVM.SharedDevices = sharedDevices
				return currentFVM
			} else {
				return FolderViewModel{Config: folderConfig, SharedDevices: sharedDevices}
			}
		},
	)

	return newFolderViewModelList
}

func updateDeviceViewModelConfigs(
	config syncthing.Config,
	current []DeviceViewModel,
	thisDeviceID string,
) []DeviceViewModel {
	newDeviceViewModelList := lo.FilterMap(
		config.Devices,
		func(deviceConfig syncthing.DeviceConfig, index int) (DeviceViewModel, bool) {
			if thisDeviceID == deviceConfig.DeviceID {
				return DeviceViewModel{}, false
			}

			currentDVM, found := lo.Find(
				current,
				func(fvm DeviceViewModel) bool { return deviceConfig.DeviceID == fvm.Config.DeviceID },
			)

			folders := lo.FilterMap(
				config.Folders,
				func(folderConfig syncthing.FolderConfig, index int) (lo.Tuple2[string, string], bool) {
					foo := lo.SomeBy(
						folderConfig.Devices,
						func(item syncthing.FolderDevice) bool { return item.DeviceID == deviceConfig.DeviceID },
					)

					if foo {
						return lo.T2(folderConfig.ID, folderConfig.Label), true
					} else {
						return lo.T2("", ""), false
					}
				},
			)

			if found {
				currentDVM.Config = deviceConfig
				currentDVM.Folders = folders
				return currentDVM, true
			} else {
				return DeviceViewModel{
					Config:           deviceConfig,
					Folders:          folders,
					StatusCompletion: make(map[string]syncthing.StatusCompletion),
				}, true
			}
		},
	)

	return newDeviceViewModelList
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

func updateDeviceExtraStats(
	devices []DeviceViewModel,
	statsDict map[string]syncthing.DeviceStats,
) []DeviceViewModel {
	return lo.Map(devices, func(item DeviceViewModel, index int) DeviceViewModel {
		stats, has := statsDict[item.Config.DeviceID]
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

func updateDeviceStatusCompletion(
	devices []DeviceViewModel,
	deviceID string,
	folderID string,
	statusCompletion syncthing.StatusCompletion,
) {
	device, has := lo.Find(
		devices,
		func(item DeviceViewModel) bool { return item.Config.DeviceID == deviceID },
	)

	if !has {
		return
	}

	device.StatusCompletion[folderID] = statusCompletion
}

func handleMouseLeftClick(m model, msg tea.MouseMsg) (model, tea.Cmd) {
	if zone.Get(RESCAN_ALL_MARK).InBounds(msg) {
		cmds := make([]tea.Cmd, 0, len(m.folders))
		for _, f := range m.folders {
			cmds = append(cmds, postScan(m.httpData, f.Config.ID))
		}
		return m, tea.Batch(cmds...)
	}

	if zone.Get(PAUSE_ALL_MARK).InBounds(msg) && !m.ongoingUserAction {
		cmds := make([]tea.Cmd, 0, len(m.folders))
		for _, f := range m.folders {
			cmds = append(cmds, updateFolderPause(m.httpData, f.Config.ID, true))
		}
		m.ongoingUserAction = true
		return m, tea.Batch(cmds...)
	}

	if zone.Get(RESUME_ALL_MARK).InBounds(msg) {
		cmds := make([]tea.Cmd, 0, len(m.folders))
		for _, f := range m.folders {
			cmds = append(cmds, updateFolderPause(m.httpData, f.Config.ID, false))
		}
		m.ongoingUserAction = true
		return m, tea.Batch(cmds...)
	}

	for _, folder := range m.folders {
		if zone.Get(folder.HeaderMark()).InBounds(msg) {
			if _, exists := m.expandedFields[folder.Config.ID]; exists {
				delete(m.expandedFields, folder.Config.ID)
			} else {
				m.expandedFields[folder.Config.ID] = struct{}{}
			}
			return m, nil
		}

		if zone.Get(folder.TogglePauseMark()).InBounds(msg) && !m.ongoingUserAction {
			m.ongoingUserAction = true
			return m, updateFolderPause(m.httpData, folder.Config.ID, !folder.Config.Paused)
		}

		if zone.Get(folder.RescanMark()).InBounds(msg) {
			return m, postScan(m.httpData, folder.Config.ID)
		}

		if zone.Get(folder.RevertLocalAdditionsMark()).InBounds(msg) {
			m.confirmRevertLocalChangesModal.Show = true
			m.confirmRevertLocalChangesModal.folderID = folder.Config.ID
			return m, nil
		}
	}

	for _, device := range m.devices {
		if zone.Get(device.HeaderMark()).InBounds(msg) {
			if _, exists := m.expandedFields[device.Config.DeviceID]; exists {
				delete(m.expandedFields, device.Config.DeviceID)
			} else {
				m.expandedFields[device.Config.DeviceID] = struct{}{}
			}
			return m, nil
		}
	}
	for _, pendingDevice := range m.pendingDevices {
		if zone.Get(pendingDevice.DismissMark()).InBounds(msg) {
			return m, deletePendingDevice(m.httpData, pendingDevice.DeviceID)
		}

		if zone.Get(pendingDevice.IgnoreMark()).InBounds(msg) {

			cmd := m.putConfig(m.httpData, func(oldConfig syncthing.Config) syncthing.Config {
				oldConfig.RemoteIgnoredDevices = append(
					oldConfig.RemoteIgnoredDevices,
					syncthing.RemoteIgnoredDevice{
						DeviceID: pendingDevice.DeviceID,
						Name:     m.pendingDevices[pendingDevice.DeviceID].Name,
						Address:  m.pendingDevices[pendingDevice.DeviceID].Address,
						Time:     m.currentTime,
					},
				)
				return oldConfig
			})
			return m, cmd
		}

		if zone.Get(pendingDevice.AddMark()).InBounds(msg) {
			m.addDeviceModal = NewPendingDevice(
				m.pendingDevices[pendingDevice.DeviceID].Name,
				pendingDevice.DeviceID,
				m.configDefaults.Device,
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

	pendingDevices := lo.Values(m.pendingDevices)
	sort.Sort(PendingDeviceList(pendingDevices))

	main := lipgloss.NewStyle().MaxHeight(m.height).Render(
		lipgloss.JoinVertical(lipgloss.Center,
			viewPendingDevices(pendingDevices),
			lipgloss.JoinHorizontal(lipgloss.Top,
				viewFolders(m.folders, m.expandedFields),
				lipgloss.JoinVertical(lipgloss.Left,
					viewStatus(
						m.thisDeviceStatus,
						m.folders,
						m.version,
					),

					viewDevices(m.devices, m.currentTime, m.expandedFields),
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
		btnConfirm := zone.Mark(
			REVERT_LOCAL_CHANGES_CONFIRM_BTN,
			styles.NegativeBtn.Render("Revert"),
		)
		btnCancel := zone.Mark(REVERT_LOCAL_CHANGES_CANCEL_BTN, styles.BtnStyleV2.Render("Cancel"))
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
		REVERT_LOCAL_CHANGES_MODAL_AREA,
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
	if !zone.Get(REVERT_LOCAL_CHANGES_MODAL_AREA).InBounds(msg) {
		m.confirmRevertLocalChangesModal.Show = false
		m.confirmRevertLocalChangesModal.folderID = ""
		return m, nil
	}

	if zone.Get(REVERT_LOCAL_CHANGES_CONFIRM_BTN).InBounds(msg) {
		folderID := m.confirmRevertLocalChangesModal.folderID
		m.confirmRevertLocalChangesModal.folderID = ""
		m.confirmRevertLocalChangesModal.Show = false
		cmd := postRevertChanges(m.httpData, folderID)
		return m, cmd
	}

	if zone.Get(REVERT_LOCAL_CHANGES_CANCEL_BTN).InBounds(msg) {
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
				p.At.Format(time.DateTime),
			).Render(),
		)

		description := fmt.Sprintf("Device \"%s\" (%s at %s) wants to connect. Add new device?",
			(p.Name),
			(p.DeviceID),
			p.Address,
		)
		btns := lipgloss.JoinHorizontal(lipgloss.Top,
			zone.Mark(p.AddMark(), styles.PositiveBtn.Render("Add Device")),
			" ",
			zone.Mark(p.IgnoreMark(), styles.NegativeBtn.Render("Ignore")),
			" ",
			zone.Mark(p.DismissMark(), styles.BtnStyleV2.Render("Dismiss")),
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

	if this.MaxSendKbps > 0 {
		t = t.Row("",
			italicStyle(fmt.Sprintf("Limit: %s/s",
				humanize.IBytes(uint64(this.MaxSendKbps)*humanize.KiByte))))
	}

	t = t.Row("Upload rate",
		fmt.Sprintf("%s/s (%s)",
			humanize.IBytes(uint64(this.OutGoingBytesPerSecond)),
			humanize.IBytes(uint64(this.OutBytesTotal)),
		),
	)

	if this.MaxRecvKbps > 0 {
		t = t.Row("",
			italicStyle(
				fmt.Sprintf("Limit: %s/s",
					humanize.IBytes(uint64(this.MaxRecvKbps)*humanize.KiByte))))
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
	expandedFolder map[string]struct{},
) string {
	views := lo.Map(folders, func(item FolderViewModel, index int) string {
		_, isExpanded := expandedFolder[item.Config.ID]
		return viewFolder(item, isExpanded)
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
	verticalViews = append(verticalViews, zone.Mark(folder.HeaderMark(), header.Render()))
	if expanded {
		foo := lo.Ternary(folder.Config.FsWatcherEnabled, "Enabled", "Disabled")

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
			lo.T2("Shared With", strings.Join(folder.SharedDevices, ", ")),
			lo.T2("Last Scan", fmt.Sprint(folder.ExtraStats.LastScan.Format(time.DateTime))),
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
			revertLocalChangesBtn := zone.Mark(folder.RevertLocalAdditionsMark(),
				styles.NegativeBtn.Render("Revert Local Changes"))

			pauseBtn := zone.
				Mark(folder.TogglePauseMark(),
					styles.BtnStyleV2.
						Render(lo.Ternary(
							folderStatus(folder) == Paused,
							"Resume",
							"Pause",
						)))
			rescanBtn := zone.
				Mark(folder.RescanMark(),
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

func viewDevices(devices []DeviceViewModel, currentTime time.Time,
	expandedFields map[string]struct{},
) string {
	views := lo.Map(devices, func(device DeviceViewModel, index int) string {
		_, has := expandedFields[device.Config.DeviceID]
		return viewDevice(device, currentTime, has)
	})

	return lipgloss.JoinVertical(lipgloss.Left, views...)
}

func viewDevice(device DeviceViewModel, currentTime time.Time, expanded bool) string {
	status := deviceStatus(device, currentTime)
	color := deviceColor(status)
	container := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		PaddingLeft(1).
		PaddingRight(1).
		Width(50).
		BorderForeground(color)
	groupedCompletion := groupCompletion(device.StatusCompletion)

	containerInnerWidth := container.GetWidth() - container.GetHorizontalPadding()
	var deviceStatusLabel string
	if groupedCompletion.Completion != 100 && status == DeviceSyncing {
		deviceStatusLabel = fmt.Sprintf(
			"%s (%0.f%%, %s)",
			deviceLabel(status),
			groupedCompletion.Completion,
			humanize.IBytes(uint64(groupedCompletion.NeedBytes)))
	} else {
		deviceStatusLabel = deviceLabel(status)
	}

	header := lipgloss.NewStyle().Bold(true).Render(
		zone.Mark(device.HeaderMark(), spaceAroundTable().Width(containerInnerWidth).
			Row(device.Config.Name,
				lipgloss.
					NewStyle().
					Foreground(color).
					Render(deviceStatusLabel)).
			Render()),
	)

	if !expanded {
		return container.Render(header)
	}

	sharedFolders := make([]string, 0, len(device.Folders))
	for _, f := range device.Folders {
		sharedFolders = append(sharedFolders, f.B)
	}

	table := spaceAroundTable().
		Width(containerInnerWidth)
	if device.Connection.B.Connected {
		table.Row("Download Rate",
			fmt.Sprintf("%s/s (%s)",
				humanize.IBytes(uint64(device.InGoingBytesPerSecond)),
				humanize.IBytes(uint64(device.Connection.B.InBytesTotal)),
			),
		).
			Row("Upload Rate",
				fmt.Sprintf("%s/s (%s)",
					humanize.IBytes(uint64(device.OutGoingBytesPerSecond)),
					humanize.IBytes(uint64(device.Connection.B.OutBytesTotal)),
				),
			)
		if status == DeviceSyncing {
			table.Row("Out of Sync Items", fmt.Sprint(groupedCompletion.NeedItems))
		}
	} else {
		table.
			Row("Last Seen", device.ExtraStats.LastSeen.Format(time.DateTime))

		if groupedCompletion.NeedBytes > 0 {
			table.Row("Sync Status", fmt.Sprintf("%0.f%%", groupedCompletion.Completion))
			table.Row("Out of Sync Items",
				fmt.Sprintf("%d items, ~%s",
					groupedCompletion.NeedItems,
					humanize.IBytes(uint64(groupedCompletion.NeedBytes))))
		} else {
			table.Row("Sync Status", "Up to Date")
		}

	}
	table.Row("Address", device.Connection.B.Address).
		Row("Compresson", device.Config.Compression).
		Row("Identification", shortIdentification(device.Config.DeviceID)).
		Row("Version", (device.Connection.B.ClientVersion)).
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
	Completion  float64
}

func groupCompletion(arg map[string]syncthing.StatusCompletion) GroupedCompletion {
	grouped := GroupedCompletion{}
	for _, c := range arg {
		grouped.NeedBytes += c.NeedBytes
		grouped.NeedItems += c.NeedItems
		grouped.NeedDeletes += c.NeedDeletes
		grouped.TotalBytes += c.GlobalBytes
	}
	grouped.Completion = math.Floor(
		100 * (1.0 - float64(grouped.NeedBytes)/float64(grouped.TotalBytes)),
	)

	return grouped
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

func deviceStatus(device DeviceViewModel, currentTime time.Time) DeviceStatus {
	isUnused := len(device.Folders) == 0

	if !device.Connection.A {
		return DeviceUnknown
	}

	if device.Config.Paused {
		return lo.Ternary(isUnused, DeviceUnusedPaused, DevicePaused)
	}
	if device.Connection.B.Connected {
		insync := lo.Ternary(isUnused, DeviceUnusedInSync, DeviceInSync)
		groupedCompletion := groupCompletion(device.StatusCompletion)
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

	lastSeenDays := currentTime.Sub(device.ExtraStats.LastSeen).Hours() / 24

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

func thisDeviceName(myID string, config syncthing.Config) string {
	for _, device := range config.Devices {
		if device.DeviceID == myID {
			return device.Name
		}
	}

	return "no-name"
}

type Connection interface {
	When() time.Time
	InBytes() int64
	OutBytes() int64
}

func calcInOutBytes(before, after Connection) (int64, int64) {
	inBytesPerSecond := byteThroughputInSeconds(
		TotalBytes{
			bytes: before.InBytes(),
			at:    before.When(),
		},
		TotalBytes{
			bytes: after.InBytes(),
			at:    after.When(),
		})

	outBytesPerSecond := byteThroughputInSeconds(
		TotalBytes{
			bytes: before.OutBytes(),
			at:    before.When(),
		},
		TotalBytes{
			bytes: after.OutBytes(),
			at:    after.When(),
		})

	return inBytesPerSecond, outBytesPerSecond
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
