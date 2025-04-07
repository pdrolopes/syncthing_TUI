package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/pdrolopes/syncthing_TUI/styles"
	"github.com/pdrolopes/syncthing_TUI/syncthing"
)

var tabLabels = []string{"General", "Sharing", "Advanced"}

type AddDeviceModel struct {
	Show            bool
	existingDevice  bool
	activeTab       int
	deviceIdInput   textinput.Model
	deviceNameInput textinput.Model
	zonePrefix      string

	httpData            HttpData
	width               int
	height              int
	introducer          bool
	autoAccept          bool
	addresses           []string
	maxRecvKbps         int64
	maxSendKbps         int64
	untrusted           bool
	numberOfConnections int
	compression         string
}

func NewPendingDevice(
	deviceName, deviceID string,
	deviceDefaults syncthing.DeviceDefaults,
	httpData HttpData,
) AddDeviceModel {
	deviceIdInput := textinput.New()
	deviceIdInput.SetValue(deviceID)
	deviceIdInput.CharLimit = 63

	deviceNameInput := textinput.New()
	deviceNameInput.SetValue(deviceName)
	deviceNameInput.Focus()
	deviceNameInput.CharLimit = 50
	return AddDeviceModel{
		Show:           true,
		existingDevice: true,
		zonePrefix:     zone.NewPrefix(),
		httpData:       httpData,

		// TODO figure out good values for dimensions, reflect terminal size?
		width:               80,
		height:              16,
		deviceNameInput:     deviceNameInput,
		deviceIdInput:       deviceIdInput,
		untrusted:           false,
		autoAccept:          deviceDefaults.AutoAcceptFolders,
		introducer:          deviceDefaults.Introducer,
		compression:         deviceDefaults.Compression,
		addresses:           deviceDefaults.Addresses,
		maxSendKbps:         deviceDefaults.MaxSendKbps,
		maxRecvKbps:         deviceDefaults.MaxRecvKbps,
		numberOfConnections: deviceDefaults.NumConnections,
	}
}

func (m AddDeviceModel) Init() tea.Cmd {
	return tea.Batch(
		m.deviceNameInput.Focus(),
		m.deviceNameInput.Cursor.BlinkCmd(),
	)
}

func (m AddDeviceModel) Update(msg tea.Msg) (AddDeviceModel, tea.Cmd) {
	// dont accept any msgs when not shown
	if !m.Show {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case msg.String() == "q":
			if !m.deviceIdInput.Focused() && !m.deviceNameInput.Focused() {
				m.Show = false
				return m, nil
			}
		case msg.Type == tea.KeyEsc:
			m.Show = false
			return m, nil
		}

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionRelease || msg.Button != tea.MouseButtonLeft {
			return m, nil
		}

		// handle clicks
		if zone.Get(m.zonePrefix + "deviceIdInput").InBounds(msg) {
			m.deviceNameInput.Blur()
			return m, m.deviceIdInput.Focus()
		}

		if zone.Get(m.zonePrefix + "deviceNameInput").InBounds(msg) {
			m.deviceIdInput.Blur()
			return m, m.deviceNameInput.Focus()
		}

		if zone.Get(m.zonePrefix + "close").InBounds(msg) {
			m.Show = false
			return m, nil
		}

		if zone.Get(m.zonePrefix + "save").InBounds(msg) {
			m.Show = false
			cmd := PostDeviceConfig(m.httpData, syncthing.DeviceConfig{
				DeviceID:          strings.TrimSpace(m.deviceIdInput.Value()),
				Name:              strings.TrimSpace(m.deviceNameInput.Value()),
				AutoAcceptFolders: m.autoAccept,
				Addresses:         m.addresses,
				Compression:       m.compression,
				Introducer:        m.introducer,
				MaxRecvKbps:       m.maxRecvKbps,
				MaxSendKbps:       m.maxSendKbps,
				NumConnections:    m.numberOfConnections,
				Untrusted:         m.untrusted,
			})
			return m, cmd
		}

		for i := range tabLabels {
			if zone.Get(fmt.Sprintf("tab-click/%d", i)).InBounds(msg) {
				m.activeTab = i
				break
			}
		}

		return m, nil
	}
	var cmd1 tea.Cmd
	var cmd2 tea.Cmd
	m.deviceIdInput, cmd1 = m.deviceIdInput.Update(msg)
	m.deviceNameInput, cmd2 = m.deviceNameInput.Update(msg)
	return m, tea.Batch(cmd1, cmd2)
}

func (m AddDeviceModel) View() string {
	tabViews := make([]string, 0, len(tabLabels))
	for i, l := range tabLabels {
		if i == m.activeTab {
			tabViews = append(
				tabViews,
				zone.Mark(fmt.Sprintf("tab-click/%d", i), activeTab.Render(l)),
			)
		} else {
			tabViews = append(tabViews, zone.Mark(fmt.Sprintf("tab-click/%d", i), tab.Render(l)))
		}
	}

	tabs := lipgloss.JoinHorizontal(lipgloss.Top,
		tabViews...,
	)

	gap := tabGap.Render(strings.Repeat(" ", max(0, m.width-lipgloss.Width(tabs))))

	header := lipgloss.JoinHorizontal(lipgloss.Bottom, tabs, gap)

	containerRest := tab.BorderTop(false).Padding(1, 1).Width(m.width).Height(m.height)
	actions := lipgloss.PlaceHorizontal(
		containerRest.GetWidth()-containerRest.GetHorizontalPadding(),
		lipgloss.Right,
		m.viewActions(),
	)
	contentHeight := m.height - lipgloss.Height(header) + lipgloss.Height(actions)
	var content string
	switch m.activeTab {
	case 0:
		content = lipgloss.PlaceVertical(contentHeight, lipgloss.Top, m.viewGeneral())
	case 1:
		content = lipgloss.PlaceVertical(contentHeight, lipgloss.Top, m.viewSharing())
	case 2:
		content = lipgloss.PlaceVertical(contentHeight, lipgloss.Top, m.viewAdvanced())
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		containerRest.Render(lipgloss.JoinVertical(lipgloss.Left,
			content,
			actions,
		)),
	)
}

func (m AddDeviceModel) viewGeneral() string {
	var doc strings.Builder

	doc.WriteString("Device ID")
	doc.WriteString("\n")
	doc.WriteString(
		zone.Mark(m.zonePrefix+"deviceIdInput", m.deviceIdInput.View()),
	)
	doc.WriteString("\n\n")
	doc.WriteString("Device Name")
	doc.WriteString("\n")
	doc.WriteString(
		zone.Mark(m.zonePrefix+"deviceNameInput", m.deviceNameInput.View()),
	)

	return doc.String()
}

func (m AddDeviceModel) viewSharing() string {
	return "todo"
}

func (m AddDeviceModel) viewAdvanced() string {
	return "todo"
}

func (m AddDeviceModel) viewActions() string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		zone.Mark(m.zonePrefix+"save", styles.BtnStyleV2.Render("Save")),
		"  ",
		zone.Mark(m.zonePrefix+"close", styles.BtnStyleV2.Render("Close")),
	)
}
