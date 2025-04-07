package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pdrolopes/syncthing_TUI/syncthing"
	"github.com/samber/lo"
)

func fetchFolderStatus(foo HttpData, folderID string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderID)
		var statusFolder syncthing.FolderStatus
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
		err := fetchBytes("http://localhost:8384/rest/events?"+params.Encode(), httpData.apiKey, &events)
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
		err := fetchBytes("http://localhost:8384/rest/system/status", httpData.apiKey, &status)
		if err != nil {
			return FetchedSystemStatusMsg{err: err}
		}

		return FetchedSystemStatusMsg{status: status}
	}
}

func fetchSystemVersion(httpData HttpData) tea.Cmd {
	return func() tea.Msg {
		var version syncthing.SystemVersion
		err := fetchBytes("http://localhost:8384/rest/system/version", httpData.apiKey, &version)
		if err != nil {
			return FetchedSystemVersionMsg{err: err}
		}

		return FetchedSystemVersionMsg{version: version}
	}
}

func fetchSystemConnections(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var connections syncthing.SystemConnection
		err := fetchBytes("http://localhost:8384/rest/system/connections", foo.apiKey, &connections)
		if err != nil {
			return FetchedSystemConnectionsMsg{err: err}
		}

		return FetchedSystemConnectionsMsg{connections: connections}
	}
}

func fetchConfig(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var config syncthing.Config
		err := fetchBytes("http://localhost:8384/rest/config", foo.apiKey, &config)
		if err != nil {
			return FetchedConfig{err: err}
		}

		return FetchedConfig{config: config}
	}
}

func fetchFolderStats(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var folderStats map[string]syncthing.FolderStats
		err := fetchBytes("http://localhost:8384/rest/stats/folder", foo.apiKey, &folderStats)
		if err != nil {
			return FetchedFolderStats{err: err}
		}

		return FetchedFolderStats{folderStats: folderStats}
	}
}

func fetchDeviceStats(foo HttpData) tea.Cmd {
	return func() tea.Msg {
		var deviceStats map[string]syncthing.DeviceStats
		err := fetchBytes("http://localhost:8384/rest/stats/device", foo.apiKey, &deviceStats)
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

func postScan(foo HttpData, folderId string) tea.Cmd {
	return func() tea.Msg {
		params := url.Values{}
		params.Add("folder", folderId)
		url := foo.url.JoinPath(DB_SCAN)
		url.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodPost, url.String(), nil)
		if err != nil {
			return nil
		}

		req.Header.Set("X-API-Key", foo.apiKey)
		resp, err := foo.client.Do(req)
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

func putFolder(foo HttpData, folders ...syncthing.FolderConfig) tea.Cmd {
	return func() tea.Msg {
		err := put("http://localhost:8384/rest/config/folders/", foo.apiKey, folders)
		ids := strings.Join(lo.Map(folders, func(item syncthing.FolderConfig, index int) string { return item.ID }), ", ")
		return UserPostPutEndedMsg{err: err, action: "putFolder: " + ids}
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

func putConfig(httpData HttpData, config syncthing.Config) tea.Cmd {
	return func() tea.Msg {
		jsonData, err := json.Marshal(config)
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

func currentTimeCmd() tea.Cmd {
	return tea.Every(REFETCH_CURRENT_TIME_INTERVAL, func(currentTime time.Time) tea.Msg { return TickedCurrentTimeMsg{currentTime: currentTime} })
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
