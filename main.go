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

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/davecgh/go-spew/spew"
)

type errMsg error

// # Useful links
// https://docs.syncthing.net/dev/rest.html#rest-pagination
// https://leg100.github.io/en/posts/building-bubbletea-programs/#2-dump-messages-to-a-file

// TODO create const for syncthing ports paths
// TODO currently we are skipping tls verification for the https://localhost path. What can we do about it

type model struct {
	dump               io.Writer
	list               list.Model
	loading            bool
	err                error
	folders            []SyncthingFolder
	width              int
	height             int
	syncthingApiKey    string
	synchingEventSince int
}

var quitKeys = key.NewBinding(
	key.WithKeys("q", "esc", "ctrl+c"),
	key.WithHelp("", "press q to quit"),
)

var down = key.NewBinding(
	key.WithKeys("down", "j"),
	key.WithHelp("", "press q to quit"),
)
var up = key.NewBinding(
	key.WithKeys("up", "k"),
	key.WithHelp("", "press q to quit"),
)

func initialModel() model {
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
		list:            list.New(make([]list.Item, 0), list.NewDefaultDelegate(), 30, 20),
		loading:         true,
		syncthingApiKey: syncthingApiKey,
		dump:            dump,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		fetchFolders(m.syncthingApiKey),
		fetchEvents(m.syncthingApiKey, m.synchingEventSince),
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
		case key.Matches(msg, down):
			m.list.CursorDown()
			return m, nil
		case key.Matches(msg, up):
			m.list.CursorUp()
			return m, nil
		default:
			return m, nil
		}

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

		items := make([]list.Item, 0)
		for _, f := range msg.folders {
			items = append(items, f)
		}
		// m.list = list.New(items, list.NewDefaultDelegate(), 30, 20)
		m.list.Title = fmt.Sprintf("Folders (%d)", len(msg.folders))
		m.list.SetItems(items)
		m.list.Styles.Title = titleStyle
		m.list.SetShowHelp(false)
		m.folders = msg.folders
		return m, nil
	case FetchedEventsMsg:
		if msg.err != nil {
			// TODO figure out what to do if event errors
			m.err = msg.err
		}
		if len(msg.events) > 0 {
			m.synchingEventSince = msg.events[len(msg.events)-1].ID
		}
		cmds := make([]tea.Cmd, 0)
		for _, e := range msg.events {
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
		var cmds []tea.Cmd
		newListModel, cmd := m.list.Update(msg)
		m.list = newListModel
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}
}

// ------------------ VIEW --------------------------
var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)

	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}).
				Render
)

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
	return appStyle.Render(m.list.View())

	// return ViewFolders(slices.Collect(m.folders.Values()))
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
		return "Syncing"
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
		Render("Up to Date")
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// Message for HTTP response
type FetchedFoldersMsg struct {
	folders []SyncthingFolder
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
