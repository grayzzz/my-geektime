package setting

import "github.com/zkep/my-geektime/internal/config"

type SettingUpdate struct {
	StorageHost   string   `json:"storageHost,omitempty"`
	SiteProxyURL  string   `json:"siteProxyUrl,omitempty"`
	SiteDownload  bool     `json:"siteDownload,omitempty"`
	SiteCache     bool     `json:"siteCache,omitempty"`
	SiteProxyUrls []string `json:"siteProxyUrls,omitempty"`
	SitePlayUrls  []string `json:"sitePlayUrls,omitempty"`
	Cookie        string   `json:"cookie,omitempty"`
	AI            config.AI `json:"ai,omitempty"`
}
