package adddevice

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	st "github.com/pdrolopes/syncthing_TUI/syncthing"
)

var tabLabels = []string{"General", "Sharing", "Advanced"}

type AddDeviceModel struct {
	Show           bool
	existingDevice bool
	activeTab      int

	deviceID            string
	deviceName          string
	introducer          bool
	autoAccept          bool
	addresses           []string
	incomingRateLimit   int64
	outgoingRateLimit   int64
	untrusted           bool
	numberOfConnections int
	compression         string
}

func New(deviceDefaults st.DeviceDefaults) AddDeviceModel {
	return AddDeviceModel{
		Show:           true,
		existingDevice: false,

		deviceID:            "",
		deviceName:          "",
		untrusted:           false,
		autoAccept:          deviceDefaults.AutoAcceptFolders,
		introducer:          deviceDefaults.Introducer,
		compression:         deviceDefaults.Compression,
		addresses:           deviceDefaults.Addresses,
		outgoingRateLimit:   deviceDefaults.MaxSendKbps,
		incomingRateLimit:   deviceDefaults.MaxRecvKbps,
		numberOfConnections: deviceDefaults.NumConnections,
	}
}

func NewPendingDevice(deviceName, deviceID string, deviceDefaults st.DeviceDefaults) AddDeviceModel {
	return AddDeviceModel{
		Show:           true,
		existingDevice: true,

		deviceID:            "",
		deviceName:          "",
		untrusted:           false,
		autoAccept:          deviceDefaults.AutoAcceptFolders,
		introducer:          deviceDefaults.Introducer,
		compression:         deviceDefaults.Compression,
		addresses:           deviceDefaults.Addresses,
		outgoingRateLimit:   deviceDefaults.MaxSendKbps,
		incomingRateLimit:   deviceDefaults.MaxRecvKbps,
		numberOfConnections: deviceDefaults.NumConnections,
	}
}

func (ad AddDeviceModel) Init() tea.Cmd {
	return nil
}

var quitKeys = key.NewBinding(
	key.WithKeys("q", "esc", "ctrl+c"),
	key.WithHelp("", "press q to quit"),
)

func (m AddDeviceModel) Update(msg tea.Msg) (AddDeviceModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, quitKeys):
			m.Show = false
			return m, nil
		default:
			return m, nil
		}
	case tea.MouseMsg:
		if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
			return m, nil
		}

		for i := range tabLabels {
			if zone.Get(fmt.Sprintf("tab-click/%d", i)).InBounds(msg) {
				m.activeTab = i
				return m, nil
			}
		}
	}

	return m, nil
}

func (m AddDeviceModel) View() string {
	tabViews := make([]string, 0, len(tabLabels))
	for i, l := range tabLabels {
		if i == m.activeTab {
			tabViews = append(tabViews, zone.Mark(fmt.Sprintf("tab-click/%d", i), activeTab.Render(l)))
		} else {
			// TODO verify this
			tabViews = append(tabViews, zone.Mark(fmt.Sprintf("tab-click/%d", i), tab.Render(l)))
		}
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Top,
		tabViews...,
	)

	return lipgloss.JoinVertical(lipgloss.Left, tabs, "foo")
}
