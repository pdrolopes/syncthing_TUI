package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pdrolopes/syncthing_TUI/syncthing"
)

const (
	CLUSTER_PENDING_DEVICES = "/rest/cluster/pending/devices"
	CLUSTER_PENDING_FOLDERS = "/rest/cluster/pending/folders"
	CONFIG                  = "/rest/config"
	CONFIG_DEVICES          = "/rest/config/devices"
	CONFIG_FOLDERS          = "/rest/config/folders"
	DB_COMPLETION_PATH      = "/rest/db/completion"
	DB_REVERT               = "/rest/db/revert"
	DB_SCAN                 = "/rest/db/scan"
	DB_STATUS               = "/rest/db/status"
	EVENTS                  = "/rest/events"
	STATS_DEVICE            = "/rest/stats/device"
	STATS_FOLDER            = "/rest/stats/folder"
	SYSTEM_CONNECTIONS      = "/rest/system/connections"
	SYSTEM_STATUS           = "/rest/system/status"
	SYSTEM_VERSION          = "/rest/system/version"
)

func fetchFolderStatus(httpData HttpData, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderID)
		url := httpData.url.JoinPath(DB_STATUS)
		url.RawQuery = params.Encode()
		var statusFolder syncthing.FolderStatus
		err := fetchBytes(
			httpData,
			*url,
			&statusFolder)
		if err != nil {
			return FetchedFolderStatus{err: err}
		}

		return FetchedFolderStatus{folderStatus: statusFolder, id: folderID}
	}
}

func wait(waitTime time.Duration, command tea.Cmd) tea.Cmd {
	return tea.Tick(waitTime, func(time.Time) tea.Msg {
		return command()
	})
}

func fetchEvents(httpData HttpData, since int) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("since", fmt.Sprint(since))
		var events []syncthing.Event[json.RawMessage]
		url := httpData.url.JoinPath(EVENTS)
		url.RawQuery = params.Encode()
		err := fetchBytes(
			httpData,
			*url,
			&events,
		)
		if err != nil {
			return FetchedEventsMsg{err: err, since: since}
		}

		parsedEvents := make([]syncthing.Event[any], 0, len(events))
		for _, e := range events {
			switch e.Type {
			case "FolderSummary":
				var data syncthing.FolderSummaryEventData
				err := json.Unmarshal(e.Data, &data)
				if err != nil {
					// TODO figure out how to handle this
					continue
				}

				parsedEvents = append(parsedEvents, syncthing.Event[any]{
					ID:       e.ID,
					GlobalID: e.GlobalID,
					Time:     e.Time,
					Type:     e.Type,
					Data:     data,
				})
			case "ConfigSaved":
				var data syncthing.Config
				err := json.Unmarshal(e.Data, &data)
				if err != nil {
					// TODO figure out how to handle this
					continue
				}

				parsedEvents = append(parsedEvents, syncthing.Event[any]{
					ID:       e.ID,
					GlobalID: e.GlobalID,
					Time:     e.Time,
					Type:     e.Type,
					Data:     data,
				})
			case "FolderScanProgress":
				var data syncthing.FolderScanProgressEventData
				err := json.Unmarshal(e.Data, &data)
				if err != nil {
					// TODO figure out how to handle this
					continue
				}

				parsedEvents = append(parsedEvents, syncthing.Event[any]{
					ID:       e.ID,
					GlobalID: e.GlobalID,
					Time:     e.Time,
					Type:     e.Type,
					Data:     data,
				})
			case "StateChanged":
				var data syncthing.StateChangedEventData
				err := json.Unmarshal(e.Data, &data)
				if err != nil {
					// TODO figure out how to handle this
					continue
				}

				parsedEvents = append(parsedEvents, syncthing.Event[any]{
					ID:       e.ID,
					GlobalID: e.GlobalID,
					Time:     e.Time,
					Type:     e.Type,
					Data:     data,
				})
			case "FolderCompletion":
				var data syncthing.FolderCompletionEventData
				er := json.Unmarshal(e.Data, &data)
				if er != nil {
					// TODO figure out how to handle this
					err = er
					continue
				}

				parsedEvents = append(parsedEvents, syncthing.Event[any]{
					ID:       e.ID,
					GlobalID: e.GlobalID,
					Time:     e.Time,
					Type:     e.Type,
					Data:     data,
				})
			case "PendingDevicesChanged":
				var data syncthing.PendingDevicesChangedEventData
				er := json.Unmarshal(e.Data, &data)
				if er != nil {
					// TODO figure out how to handle this
					err = er
					continue
				}

				parsedEvents = append(parsedEvents, syncthing.Event[any]{
					ID:       e.ID,
					GlobalID: e.GlobalID,
					Time:     e.Time,
					Type:     e.Type,
					Data:     data,
				})
			default:
				parsedEvents = append(parsedEvents, syncthing.Event[any]{
					ID:       e.ID,
					GlobalID: e.GlobalID,
					Time:     e.Time,
					Type:     e.Type,
					Data:     e.Data,
				})
			}
		}

		return FetchedEventsMsg{events: parsedEvents, since: since, err: err}
	}
}

func fetchSystemStatus(httpData HttpData) tea.Cmd {
	return func() tea.Msg {
		var status syncthing.SystemStatus
		err := fetchBytes(httpData, *httpData.url.JoinPath(SYSTEM_STATUS), &status)
		if err != nil {
			return FetchedSystemStatusMsg{err: err}
		}

		return FetchedSystemStatusMsg{status: status}
	}
}

func fetchSystemVersion(httpData HttpData) tea.Cmd {
	return func() tea.Msg {
		var version syncthing.SystemVersion
		err := fetchBytes(httpData, *httpData.url.JoinPath(SYSTEM_VERSION), &version)
		if err != nil {
			return FetchedSystemVersionMsg{err: err}
		}

		return FetchedSystemVersionMsg{version: version}
	}
}

func fetchSystemConnections(httpData HttpData, prevConnections syncthing.SystemConnection) tea.Cmd {
	return func() tea.Msg {
		var connections syncthing.SystemConnection
		err := fetchBytes(httpData, *httpData.url.JoinPath(SYSTEM_CONNECTIONS), &connections)
		if err != nil {
			return FetchedSystemConnectionsMsg{err: err}
		}

		return FetchedSystemConnectionsMsg{
			connections:     connections,
			prevConnections: prevConnections,
		}
	}
}

func fetchConfig(httpData HttpData) tea.Cmd {
	return func() tea.Msg {
		var config syncthing.Config
		err := fetchBytes(httpData, *httpData.url.JoinPath(CONFIG), &config)
		if err != nil {
			return FetchedConfig{err: err}
		}

		return FetchedConfig{config: config}
	}
}

func fetchFolderStats(httpData HttpData) tea.Cmd {
	return func() tea.Msg {
		var folderStats map[string]syncthing.FolderStats
		err := fetchBytes(httpData, *httpData.url.JoinPath(STATS_FOLDER), &folderStats)
		if err != nil {
			return FetchedFolderStats{err: err}
		}

		return FetchedFolderStats{folderStats: folderStats}
	}
}

func fetchDeviceStats(httpData HttpData) tea.Cmd {
	return func() tea.Msg {
		var deviceStats map[string]syncthing.DeviceStats
		err := fetchBytes(httpData, *httpData.url.JoinPath(STATS_DEVICE), &deviceStats)
		if err != nil {
			return FetchedDeviceStats{err: err}
		}

		return FetchedDeviceStats{deviceStats: deviceStats}
	}
}

func fetchCompletion(httpData HttpData, deviceID, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("device", deviceID)
		params.Add("folder", folderID)
		url := httpData.url.JoinPath(DB_COMPLETION_PATH)
		url.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodGet, url.String(), nil)
		if err != nil {
			return FetchedCompletion{
				deviceID: deviceID,
				folderID: folderID,
				err:      err,
			}
		}

		req.Header.Set("X-API-Key", httpData.apiKey)
		resp, err := httpData.client.Do(req)
		if err != nil {
			return FetchedCompletion{
				deviceID: deviceID,
				folderID: folderID,
				err:      err,
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return FetchedCompletion{
				deviceID: deviceID,
				folderID: folderID,
			}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return FetchedCompletion{
				deviceID: deviceID,
				folderID: folderID,
				err:      err,
			}
		}

		var deviceCompletion syncthing.StatusCompletion
		err = json.Unmarshal(body, &deviceCompletion)
		if err != nil {
			err = fmt.Errorf("error unmarshalling JSON: %w", err)
			return FetchedCompletion{
				deviceID: deviceID,
				folderID: folderID,
				err:      err,
			}
		}

		return FetchedCompletion{
			deviceID:      deviceID,
			folderID:      folderID,
			completion:    deviceCompletion,
			hasCompletion: true,
		}
	}
}

func postScan(httpData HttpData, folderId string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderId)
		url := httpData.url.JoinPath(DB_SCAN)
		url.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodPost, url.String(), nil)
		if err != nil {
			return nil
		}

		req.Header.Set("X-API-Key", httpData.apiKey)
		resp, err := httpData.client.Do(req)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return nil
		}

		return nil
	}
}

func PostDeviceConfig(httpData HttpData, device syncthing.DeviceConfig) tea.Cmd {
	return func() tea.Msg {
		deviceData, err := json.Marshal(device)
		if err != nil {
			return UserPostPutEndedMsg{
				err: fmt.Errorf("PostDeviceConfig error marshalling JSON: %w", err),
			}
		}
		url := httpData.url.JoinPath(CONFIG_DEVICES)
		req, err := http.NewRequest(http.MethodPost, url.String(), bytes.NewBuffer(deviceData))
		if err != nil {
			return UserPostPutEndedMsg{
				err: err,
			}
		}

		req.Header.Set("X-API-Key", httpData.apiKey)
		resp, err := httpData.client.Do(req)
		if err != nil {
			return UserPostPutEndedMsg{
				err: err,
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return UserPostPutEndedMsg{
				err: fmt.Errorf("error while trying to post new device config"),
			}
		}

		// TODO figure out what to do when post fails
		return nil
	}
}

type (
	ChangeConfig func(config syncthing.Config) syncthing.Config
	PutConfig    func(httpData HttpData, foo ChangeConfig) tea.Cmd
)

/*
The /rest/config endpoint doesnt support a PATCH request to update a single field.
This was created so that we can PUT a change to a config without saving the syncthing.Config in the state.
We only save the PutConfig function that has config closured
*/
func createPutConfig(config syncthing.Config) PutConfig {
	return func(httpData HttpData, updateConfig ChangeConfig) tea.Cmd {
		return func() tea.Msg {
			jsonData, err := json.Marshal(updateConfig(config))
			if err != nil {
				return fmt.Errorf("error marshalling JSON: %w", err)
			}

			url := httpData.url.JoinPath(CONFIG)
			req, err := http.NewRequest(http.MethodPut, url.String(), bytes.NewBuffer(jsonData))
			if err != nil {
				return err
			}

			req.Header.Set("X-API-Key", httpData.apiKey)
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpData.client.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			return nil
		}
	}
}

func currentTimeCmd() tea.Cmd {
	return tea.Every(
		REFETCH_CURRENT_TIME_INTERVAL,
		func(currentTime time.Time) tea.Msg { return TickedCurrentTimeMsg{currentTime: currentTime} },
	)
}

func fetchPendingDevices(httpData HttpData) tea.Cmd {
	return func() tea.Msg {
		url := httpData.url.JoinPath(CLUSTER_PENDING_DEVICES)
		req, err := http.NewRequest(http.MethodGet, url.String(), nil)
		if err != nil {
			return nil
		}

		req.Header.Set("X-API-Key", httpData.apiKey)
		resp, err := httpData.client.Do(req)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return FetchedPendingDevices{
				err: err,
			}
		}

		var pendingDevices map[string]syncthing.PendingDeviceInfo
		err = json.Unmarshal(body, &pendingDevices)
		if err != nil {
			err = fmt.Errorf("error unmarshalling JSON: %w", err)
			return FetchedPendingDevices{
				err: err,
			}
		}

		return FetchedPendingDevices{
			devices: pendingDevices,
		}
	}
}

func deletePendingDevice(httpData HttpData, deviceID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("device", deviceID)
		url := httpData.url.JoinPath(CLUSTER_PENDING_DEVICES)
		url.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodDelete, url.String(), nil)
		if err != nil {
			return nil
		}

		req.Header.Set("X-API-Key", httpData.apiKey)
		resp, err := httpData.client.Do(req)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()

		return nil
	}
}

func postRevertChanges(httpData HttpData, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderID)
		url := httpData.url.JoinPath(DB_REVERT)
		url.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodPost, url.String(), nil)
		if err != nil {
			return nil
		}

		req.Header.Set("X-API-Key", httpData.apiKey)
		resp, err := httpData.client.Do(req)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()

		return nil
	}
}

func updateFolderPause(httpData HttpData, folderID string, paused bool) tea.Cmd {
	return func() tea.Msg {
		type PatchData struct {
			Paused bool `json:"paused"`
		}
		err := patchFolder(httpData, folderID, PatchData{paused})

		return UserPostPutEndedMsg{err: err, action: "updateFolderPause: " + folderID}
	}
}

func patchFolder(httpData HttpData, folderID string, patchData any) error {
	json, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	url := httpData.url.JoinPath(CONFIG_FOLDERS)
	url = url.JoinPath(folderID)
	req, err := http.NewRequest(http.MethodPatch, url.String(), bytes.NewBuffer(json))
	if err != nil {
		return fmt.Errorf("failed folder patch request: %w", err)
	}

	req.Header.Set("X-API-Key", httpData.apiKey)
	resp, err := httpData.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed folder patch request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"patchFolder \"%s\" failed. Got status code %d",
			folderID,
			resp.StatusCode,
		)
	}

	defer resp.Body.Close()

	return nil
}

func fetchBytes(httpData HttpData, url url.URL, bodyType any) error {
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return err
	}

	req.Header.Set("X-API-Key", httpData.apiKey)
	resp, err := httpData.client.Do(req)
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
