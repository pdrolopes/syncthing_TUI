package main

import "time"

// SYNCTHING DATA STRUCTURES
type SyncthingFolderConfig struct {
	ID                      string         `json:"id"`
	Label                   string         `json:"label"`
	FilesystemType          string         `json:"filesystemType"`
	Path                    string         `json:"path"`
	Type                    string         `json:"type"`
	Devices                 []FolderDevice `json:"devices"`
	RescanIntervalS         int            `json:"rescanIntervalS"`
	FsWatcherEnabled        bool           `json:"fsWatcherEnabled"`
	FsWatcherDelayS         int            `json:"fsWatcherDelayS"`
	FsWatcherTimeoutS       int            `json:"fsWatcherTimeoutS"`
	IgnorePerms             bool           `json:"ignorePerms"`
	AutoNormalize           bool           `json:"autoNormalize"`
	MinDiskFree             MinDiskFree    `json:"minDiskFree"`
	Versioning              Versioning     `json:"versioning"`
	Copiers                 int            `json:"copiers"`
	PullerMaxPendingKiB     int            `json:"pullerMaxPendingKiB"`
	Hashers                 int            `json:"hashers"`
	Order                   string         `json:"order"`
	IgnoreDelete            bool           `json:"ignoreDelete"`
	ScanProgressIntervalS   int            `json:"scanProgressIntervalS"`
	PullerPauseS            int            `json:"pullerPauseS"`
	MaxConflicts            int            `json:"maxConflicts"`
	DisableSparseFiles      bool           `json:"disableSparseFiles"`
	DisableTempIndexes      bool           `json:"disableTempIndexes"`
	Paused                  bool           `json:"paused"`
	WeakHashThresholdPct    int            `json:"weakHashThresholdPct"`
	MarkerName              string         `json:"markerName"`
	CopyOwnershipFromParent bool           `json:"copyOwnershipFromParent"`
	ModTimeWindowS          int            `json:"modTimeWindowS"`
	MaxConcurrentWrites     int            `json:"maxConcurrentWrites"`
	DisableFsync            bool           `json:"disableFsync"`
	BlockPullOrder          string         `json:"blockPullOrder"`
	CopyRangeMethod         string         `json:"copyRangeMethod"`
	CaseSensitiveFS         bool           `json:"caseSensitiveFS"`
	JunctionsAsDirs         bool           `json:"junctionsAsDirs"`
	SyncOwnership           bool           `json:"syncOwnership"`
	SendOwnership           bool           `json:"sendOwnership"`
	SyncXattrs              bool           `json:"syncXattrs"`
	SendXattrs              bool           `json:"sendXattrs"`
	XattrFilter             XattrFilter    `json:"xattrFilter"`
}

type SyncthingFolderStatus struct {
	Errors                        int            `json:"errors"`
	PullErrors                    int            `json:"pullErrors"`
	Invalid                       string         `json:"invalid"`
	GlobalFiles                   int            `json:"globalFiles"`
	GlobalDirectories             int            `json:"globalDirectories"`
	GlobalSymlinks                int            `json:"globalSymlinks"`
	GlobalDeleted                 int            `json:"globalDeleted"`
	GlobalBytes                   int64          `json:"globalBytes"`
	GlobalTotalItems              int            `json:"globalTotalItems"`
	LocalFiles                    int            `json:"localFiles"`
	LocalDirectories              int            `json:"localDirectories"`
	LocalSymlinks                 int            `json:"localSymlinks"`
	LocalDeleted                  int            `json:"localDeleted"`
	LocalBytes                    int64          `json:"localBytes"`
	LocalTotalItems               int            `json:"localTotalItems"`
	NeedFiles                     int            `json:"needFiles"`
	NeedDirectories               int            `json:"needDirectories"`
	NeedSymlinks                  int            `json:"needSymlinks"`
	NeedDeletes                   int            `json:"needDeletes"`
	NeedBytes                     int64          `json:"needBytes"`
	NeedTotalItems                int            `json:"needTotalItems"`
	ReceiveOnlyChangedFiles       int            `json:"receiveOnlyChangedFiles"`
	ReceiveOnlyChangedDirectories int            `json:"receiveOnlyChangedDirectories"`
	ReceiveOnlyChangedSymlinks    int            `json:"receiveOnlyChangedSymlinks"`
	ReceiveOnlyChangedDeletes     int            `json:"receiveOnlyChangedDeletes"`
	ReceiveOnlyChangedBytes       int64          `json:"receiveOnlyChangedBytes"`
	ReceiveOnlyTotalItems         int            `json:"receiveOnlyTotalItems"`
	InSyncFiles                   int            `json:"inSyncFiles"`
	InSyncBytes                   int64          `json:"inSyncBytes"`
	State                         string         `json:"state"`
	StateChanged                  time.Time      `json:"stateChanged"`
	Error                         string         `json:"error"`
	Version                       int            `json:"version"`
	Sequence                      int            `json:"sequence"`
	RemoteSequence                map[string]int `json:"remoteSequence"`
	IgnorePatterns                bool           `json:"ignorePatterns"`
	WatchError                    string         `json:"watchError"`
}

type FolderDevice struct {
	DeviceID           string `json:"deviceID"`
	IntroducedBy       string `json:"introducedBy"`
	EncryptionPassword string `json:"encryptionPassword"`
}

type MinDiskFree struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type VersioningParams struct {
	CleanoutDays string `json:"cleanoutDays"`
}

type Versioning struct {
	Type             string           `json:"type"`
	Params           VersioningParams `json:"params"`
	CleanupIntervalS int              `json:"cleanupIntervalS"`
	FsPath           string           `json:"fsPath"`
	FsType           string           `json:"fsType"`
}

type XattrFilter struct {
	Entries            []string `json:"entries"`
	MaxSingleEntrySize int      `json:"maxSingleEntrySize"`
	MaxTotalSize       int      `json:"maxTotalSize"`
}

type SyncthingEvent struct {
	ID       int       `json:"id"`
	GlobalID int       `json:"globalID"`
	Time     time.Time `json:"time"`
	Type     string    `json:"type"`
}

type SyncthingSystemStatus struct {
	Alloc                   int64                       `json:"alloc"`
	ConnectionServiceStatus map[string]ConnectionStatus `json:"connectionServiceStatus"`
	CPUPercent              float64                     `json:"cpuPercent"`
	DiscoveryEnabled        bool                        `json:"discoveryEnabled"`
	DiscoveryErrors         map[string]string           `json:"discoveryErrors"`
	DiscoveryMethods        int                         `json:"discoveryMethods"`
	DiscoveryStatus         map[string]DiscoveryStatus  `json:"discoveryStatus"`
	Goroutines              int                         `json:"goroutines"`
	GUIAddressOverridden    bool                        `json:"guiAddressOverridden"`
	GUIAddressUsed          string                      `json:"guiAddressUsed"`
	LastDialStatus          map[string]DialStatus       `json:"lastDialStatus"`
	MyID                    string                      `json:"myID"`
	PathSeparator           string                      `json:"pathSeparator"`
	StartTime               time.Time                   `json:"startTime"`
	Sys                     int64                       `json:"sys"`
	Tilde                   string                      `json:"tilde"`
	Uptime                  int64                       `json:"uptime"`
	URVersionMax            int                         `json:"urVersionMax"`
}

type ConnectionStatus struct {
	Error        *string  `json:"error"`
	LANAddresses []string `json:"lanAddresses"`
	WANAddresses []string `json:"wanAddresses"`
}

type DiscoveryStatus struct {
	Error *string `json:"error"`
}

type DialStatus struct {
	When  time.Time `json:"when"`
	Error *string   `json:"error"`
}

type Connection struct {
	At            time.Time   `json:"at"`
	InBytesTotal  int64       `json:"inBytesTotal"`
	OutBytesTotal int64       `json:"outBytesTotal"`
	StartedAt     time.Time   `json:"startedAt"`
	Connected     bool        `json:"connected"`
	Paused        bool        `json:"paused"`
	ClientVersion string      `json:"clientVersion"`
	Address       string      `json:"address"`
	Type          string      `json:"type"`
	IsLocal       bool        `json:"isLocal"`
	Crypto        string      `json:"crypto"`
	Primary       *Connection `json:"primary"`
}

type Total struct {
	At            time.Time `json:"at"`
	InBytesTotal  int64     `json:"inBytesTotal"`
	OutBytesTotal int64     `json:"outBytesTotal"`
}

type Connections map[string]Connection

type SyncthingSystemConnections struct {
	Connections Connections `json:"connections"`
	Total       Total       `json:"total"`
}

type DeviceConfig struct {
	DeviceID                 string          `json:"deviceID"`
	Name                     string          `json:"name"`
	Addresses                []string        `json:"addresses"`
	Compression              string          `json:"compression"`
	CertName                 string          `json:"certName"`
	Introducer               bool            `json:"introducer"`
	SkipIntroductionRemovals bool            `json:"skipIntroductionRemovals"`
	IntroducedBy             string          `json:"introducedBy"`
	Paused                   bool            `json:"paused"`
	AllowedNetworks          []string        `json:"allowedNetworks"`
	AutoAcceptFolders        bool            `json:"autoAcceptFolders"`
	MaxSendKbps              int             `json:"maxSendKbps"`
	MaxRecvKbps              int             `json:"maxRecvKbps"`
	IgnoredFolders           []IgnoredFolder `json:"ignoredFolders"`
	MaxRequestKiB            int             `json:"maxRequestKiB"`
	Untrusted                bool            `json:"untrusted"`
	RemoteGUIPort            int             `json:"remoteGUIPort"`
	NumConnections           int             `json:"numConnections"`
}

type IgnoredFolder struct {
	Time  time.Time `json:"time"`
	ID    string    `json:"id"`
	Label string    `json:"label"`
}

type SyncthingSystemVersion struct {
	Arch        string    `json:"arch"`
	Codename    string    `json:"codename"`
	Container   bool      `json:"container"`
	Date        time.Time `json:"date"`
	Extra       string    `json:"extra"`
	IsBeta      bool      `json:"isBeta"`
	IsCandidate bool      `json:"isCandidate"`
	IsRelease   bool      `json:"isRelease"`
	LongVersion string    `json:"longVersion"`
	OS          string    `json:"os"`
	Stamp       string    `json:"stamp"`
	Tags        []string  `json:"tags"`
	User        string    `json:"user"`
	Version     string    `json:"version"`
}

type LastFile struct {
	At       time.Time `json:"at"`
	Filename string    `json:"filename"`
	Deleted  bool      `json:"deleted"`
}

type FolderStats struct {
	LastFile LastFile  `json:"lastFile"`
	LastScan time.Time `json:"lastScan"`
}

type Config struct {
	Version  int                     `json:"version"`
	Folders  []SyncthingFolderConfig `json:"folders"`
	Devices  []DeviceConfig          `json:"devices"`
	GUI      GUI                     `json:"gui"`
	LDAP     LDAP                    `json:"ldap"`
	Options  Options                 `json:"options"`
	Defaults Defaults                `json:"defaults"`
}

type GUI struct {
	Enabled                   bool   `json:"enabled"`
	Address                   string `json:"address"`
	UnixSocketPermissions     string `json:"unixSocketPermissions"`
	User                      string `json:"user"`
	Password                  string `json:"password"`
	AuthMode                  string `json:"authMode"`
	UseTLS                    bool   `json:"useTLS"`
	APIKey                    string `json:"apiKey"`
	InsecureAdminAccess       bool   `json:"insecureAdminAccess"`
	Theme                     string `json:"theme"`
	Debugging                 bool   `json:"debugging"`
	InsecureSkipHostcheck     bool   `json:"insecureSkipHostcheck"`
	InsecureAllowFrameLoading bool   `json:"insecureAllowFrameLoading"`
	SendBasicAuthPrompt       bool   `json:"sendBasicAuthPrompt"`
}

type LDAP struct {
	Address            string `json:"address"`
	BindDN             string `json:"bindDN"`
	Transport          string `json:"transport"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	SearchBaseDN       string `json:"searchBaseDN"`
	SearchFilter       string `json:"searchFilter"`
}

type Options struct {
	ListenAddresses                     []string  `json:"listenAddresses"`
	GlobalAnnounceServers               []string  `json:"globalAnnounceServers"`
	GlobalAnnounceEnabled               bool      `json:"globalAnnounceEnabled"`
	LocalAnnounceEnabled                bool      `json:"localAnnounceEnabled"`
	LocalAnnouncePort                   int       `json:"localAnnouncePort"`
	LocalAnnounceMCAddr                 string    `json:"localAnnounceMCAddr"`
	MaxSendKbps                         int       `json:"maxSendKbps"`
	MaxRecvKbps                         int       `json:"maxRecvKbps"`
	ReconnectionIntervalS               int       `json:"reconnectionIntervalS"`
	RelaysEnabled                       bool      `json:"relaysEnabled"`
	RelayReconnectIntervalM             int       `json:"relayReconnectIntervalM"`
	StartBrowser                        bool      `json:"startBrowser"`
	NatEnabled                          bool      `json:"natEnabled"`
	NatLeaseMinutes                     int       `json:"natLeaseMinutes"`
	NatRenewalMinutes                   int       `json:"natRenewalMinutes"`
	NatTimeoutSeconds                   int       `json:"natTimeoutSeconds"`
	UrAccepted                          int       `json:"urAccepted"`
	UrSeen                              int       `json:"urSeen"`
	UrUniqueId                          string    `json:"urUniqueId"`
	UrURL                               string    `json:"urURL"`
	UrPostInsecurely                    bool      `json:"urPostInsecurely"`
	UrInitialDelayS                     int       `json:"urInitialDelayS"`
	AutoUpgradeIntervalH                int       `json:"autoUpgradeIntervalH"`
	UpgradeToPreReleases                bool      `json:"upgradeToPreReleases"`
	KeepTemporariesH                    int       `json:"keepTemporariesH"`
	CacheIgnoredFiles                   bool      `json:"cacheIgnoredFiles"`
	ProgressUpdateIntervalS             int       `json:"progressUpdateIntervalS"`
	LimitBandwidthInLan                 bool      `json:"limitBandwidthInLan"`
	MinHomeDiskFree                     DiskSpace `json:"minHomeDiskFree"`
	ReleasesURL                         string    `json:"releasesURL"`
	AlwaysLocalNets                     []string  `json:"alwaysLocalNets"`
	OverwriteRemoteDeviceNamesOnConnect bool      `json:"overwriteRemoteDeviceNamesOnConnect"`
	TempIndexMinBlocks                  int       `json:"tempIndexMinBlocks"`
	UnackedNotificationIDs              []string  `json:"unackedNotificationIDs"`
	TrafficClass                        int       `json:"trafficClass"`
	SetLowPriority                      bool      `json:"setLowPriority"`
	MaxFolderConcurrency                int       `json:"maxFolderConcurrency"`
	CrURL                               string    `json:"crURL"`
	CrashReportingEnabled               bool      `json:"crashReportingEnabled"`
	StunKeepaliveStartS                 int       `json:"stunKeepaliveStartS"`
	StunKeepaliveMinS                   int       `json:"stunKeepaliveMinS"`
	StunServers                         []string  `json:"stunServers"`
	DatabaseTuning                      string    `json:"databaseTuning"`
	MaxConcurrentIncomingRequestKiB     int       `json:"maxConcurrentIncomingRequestKiB"`
	AnnounceLANAddresses                bool      `json:"announceLANAddresses"`
	SendFullIndexOnUpgrade              bool      `json:"sendFullIndexOnUpgrade"`
	FeatureFlags                        []string  `json:"featureFlags"`
	ConnectionLimitEnough               int       `json:"connectionLimitEnough"`
	ConnectionLimitMax                  int       `json:"connectionLimitMax"`
	InsecureAllowOldTLSVersions         bool      `json:"insecureAllowOldTLSVersions"`
	ConnectionPriorityTcpLan            int       `json:"connectionPriorityTcpLan"`
	ConnectionPriorityQuicLan           int       `json:"connectionPriorityQuicLan"`
	ConnectionPriorityTcpWan            int       `json:"connectionPriorityTcpWan"`
	ConnectionPriorityQuicWan           int       `json:"connectionPriorityQuicWan"`
	ConnectionPriorityRelay             int       `json:"connectionPriorityRelay"`
	ConnectionPriorityUpgradeThreshold  int       `json:"connectionPriorityUpgradeThreshold"`
}

type Defaults struct {
	Folder  FolderDefaults  `json:"folder"`
	Device  DeviceDefaults  `json:"device"`
	Ignores IgnoresDefaults `json:"ignores"`
}

type FolderDefaults struct {
	ID                      string         `json:"id"`
	Label                   string         `json:"label"`
	FilesystemType          string         `json:"filesystemType"`
	Path                    string         `json:"path"`
	Type                    string         `json:"type"`
	Devices                 []FolderDevice `json:"devices"`
	RescanIntervalS         int            `json:"rescanIntervalS"`
	FsWatcherEnabled        bool           `json:"fsWatcherEnabled"`
	FsWatcherDelayS         int            `json:"fsWatcherDelayS"`
	FsWatcherTimeoutS       int            `json:"fsWatcherTimeoutS"`
	IgnorePerms             bool           `json:"ignorePerms"`
	AutoNormalize           bool           `json:"autoNormalize"`
	MinDiskFree             DiskSpace      `json:"minDiskFree"`
	Versioning              Versioning     `json:"versioning"`
	Copiers                 int            `json:"copiers"`
	PullerMaxPendingKiB     int            `json:"pullerMaxPendingKiB"`
	Hashers                 int            `json:"hashers"`
	Order                   string         `json:"order"`
	IgnoreDelete            bool           `json:"ignoreDelete"`
	ScanProgressIntervalS   int            `json:"scanProgressIntervalS"`
	PullerPauseS            int            `json:"pullerPauseS"`
	MaxConflicts            int            `json:"maxConflicts"`
	DisableSparseFiles      bool           `json:"disableSparseFiles"`
	DisableTempIndexes      bool           `json:"disableTempIndexes"`
	Paused                  bool           `json:"paused"`
	WeakHashThresholdPct    int            `json:"weakHashThresholdPct"`
	MarkerName              string         `json:"markerName"`
	CopyOwnershipFromParent bool           `json:"copyOwnershipFromParent"`
	ModTimeWindowS          int            `json:"modTimeWindowS"`
	MaxConcurrentWrites     int            `json:"maxConcurrentWrites"`
	DisableFsync            bool           `json:"disableFsync"`
	BlockPullOrder          string         `json:"blockPullOrder"`
	CopyRangeMethod         string         `json:"copyRangeMethod"`
	CaseSensitiveFS         bool           `json:"caseSensitiveFS"`
	JunctionsAsDirs         bool           `json:"junctionsAsDirs"`
	SyncOwnership           bool           `json:"syncOwnership"`
	SendOwnership           bool           `json:"sendOwnership"`
	SyncXattrs              bool           `json:"syncXattrs"`
	SendXattrs              bool           `json:"sendXattrs"`
	XattrFilter             XattrFilter    `json:"xattrFilter"`
}

type DeviceDefaults struct {
	DeviceID                 string   `json:"deviceID"`
	Name                     string   `json:"name"`
	Addresses                []string `json:"addresses"`
	Compression              string   `json:"compression"`
	CertName                 string   `json:"certName"`
	Introducer               bool     `json:"introducer"`
	SkipIntroductionRemovals bool     `json:"skipIntroductionRemovals"`
	IntroducedBy             string   `json:"introducedBy"`
	Paused                   bool     `json:"paused"`
	AllowedNetworks          []string `json:"allowedNetworks"`
	AutoAcceptFolders        bool     `json:"autoAcceptFolders"`
	MaxSendKbps              int      `json:"maxSendKbps"`
	MaxRecvKbps              int      `json:"maxRecvKbps"`
	IgnoredFolders           []string `json:"ignoredFolders"`
	MaxRequestKiB            int      `json:"maxRequestKiB"`
	Untrusted                bool     `json:"untrusted"`
	RemoteGUIPort            int      `json:"remoteGUIPort"`
	NumConnections           int      `json:"numConnections"`
}

type IgnoresDefaults struct {
	Lines []string `json:"lines"`
}

type DiskSpace struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type DeviceStats struct {
	LastSeen                time.Time `json:"lastSeen"`
	LastConnectionDurationS float64   `json:"lastConnectionDurationS"`
}

type SyncStatusCompletion struct {
	Completion  float64 `json:"completion"`
	GlobalBytes int64   `json:"globalBytes"`
	NeedBytes   int64   `json:"needBytes"`
	GlobalItems int     `json:"globalItems"`
	NeedItems   int     `json:"needItems"`
	NeedDeletes int     `json:"needDeletes"`
	RemoteState string  `json:"remoteState"`
	Sequence    int     `json:"sequence"`
}
