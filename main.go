package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errMsg error

// # Useful links
// https://docs.syncthing.net/dev/rest.html#rest-pagination
// https://github.com/76creates/stickers verify flexbox

// TODO create const for syncthing ports paths
// TODO currently we are skipping tls verification for the https://localhost path. What can we do about it

type model struct {
	spinner         spinner.Model
	loading         bool
	err             error
	syncthingApiKey string
	folders         []SyncthingFolder
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

	return model{spinner: s, syncthingApiKey: syncthingApiKey, loading: true}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, fetchData("http://localhost:8384/rest/config/folders", m.syncthingApiKey))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		if key.Matches(msg, quitKeys) {
			return m, tea.Quit

		}
		return m, nil
	case fetchedSyncthingFoldersMsg:
		foo := fetchedSyncthingFoldersMsg(msg)
		m.loading = false
		if foo.err != nil {
			m.err = foo.err
			return m, nil
		}

		m.folders = fetchedSyncthingFoldersMsg(msg).body
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

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
	foo := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		PaddingTop(2).
		PaddingBottom(2).
		PaddingRight(2).
		PaddingLeft(4).
		Width(60).Align(lipgloss.Right)

	return foo.Render(fmt.Sprintf("You have %d folders", len(m.folders)))
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// Message for HTTP response
type fetchedSyncthingFoldersMsg struct {
	body []SyncthingFolder
	err  error
}

func fetchData(url string, apiKey string) tea.Cmd {
	return func() tea.Msg {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return fetchedSyncthingFoldersMsg{err: err}
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
			return fetchedSyncthingFoldersMsg{err: err}
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fetchedSyncthingFoldersMsg{err: err}
		}
		// Parse the JSON body into the struct
		var folders []SyncthingFolder
		err = json.Unmarshal(body, &folders)
		if err != nil {
			fmt.Println("Error unmarshalling JSON:", err)
			return fetchedSyncthingFoldersMsg{err: err}
		}

		return fetchedSyncthingFoldersMsg{body: folders}
	}
}

// SYNCTHING DATA STRUCTURES
type SyncthingFolder struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	FilesystemType string `json:"filesystemType"`
	Path           string `json:"path"`
	Type           string `json:"type"`
}

// if len(os.Getenv("DEBUG")) > 0 {
// 	f, err := tea.LogToFile("debug.log", "debug")
// 	if err != nil {
// 		fmt.Println("fatal:", err)
// 		os.Exit(1)
// 	}
// 	defer f.Close()
// }
