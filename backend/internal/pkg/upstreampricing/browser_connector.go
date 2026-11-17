package upstreampricing

import (
	_ "embed"
	"encoding/json"
	"net/url"
	"strings"
)

//go:embed browser_connector.user.js
var browserConnectorTemplate string

// BrowserConnectorScript contains public connection configuration only. The
// capability ticket is created separately by an authenticated admin action.
func BrowserConnectorScript(site, origin string) (string, error) {
	c := Config{Provider: "sub2api", BaseURL: site}
	if err := ValidateConfig(&c); err != nil {
		return "", err
	}
	local, err := url.Parse(origin)
	if err != nil || ValidateBrowserOrigin(origin, local.Host, local.Scheme) != nil {
		return "", safeError("连接脚本仅支持本机地址")
	}
	station, _ := url.Parse(c.BaseURL)
	stationOrigin := station.Scheme + "://" + station.Host
	config, _ := json.Marshal(map[string]string{"site": stationOrigin, "receiver": origin})
	return strings.NewReplacer("__STATION_MATCH__", stationOrigin+"/*", "__LOCAL_HOST__", local.Hostname(), "__CONNECTOR_CONFIG__", string(config)).Replace(browserConnectorTemplate), nil
}
