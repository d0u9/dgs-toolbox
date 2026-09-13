package geo

import (
	"fmt"
	"net"
	"strconv"

	"dgs-toolbox/internal/apps/geo/gpx"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Geo app definition. Map work that a terminal cannot do is
// served by each command's local web page; the TUI stays the entry point.
func New() tui.App {
	return tui.App{
		ID:          "geo",
		Name:        "Geo",
		Description: "Geographic tools",
		Commands: []tui.Command{
			{
				ID:          "gpx",
				Name:        "GPX",
				Description: "GPX tracks in the terminal and a local map page.",
				New: func() tui.CommandModel {
					return gpx.New(gpx.SettingsFrom(config.Default()))
				},
				NewWithConfig: func(global config.Config) tui.CommandModel {
					return gpx.New(gpx.SettingsFrom(global))
				},
				Flags: []tui.Flag{
					{
						Name:  "dir",
						Usage: "folder the GPX browser opens at (default: home directory)",
						Apply: func(global *config.Config, value string) error {
							global.Geo.GPX.Root = value
							return nil
						},
					},
					{
						Name:  "host",
						Usage: "web server IP to listen on (0.0.0.0 allows other hosts)",
						Apply: func(global *config.Config, value string) error {
							if value != "" && value != "localhost" && net.ParseIP(value) == nil {
								return fmt.Errorf("%q is not an IP address", value)
							}
							global.Geo.GPX.Host = value
							return nil
						},
					},
					{
						Name:  "port",
						Usage: fmt.Sprintf("web server port (default %d)", config.DefaultGeoGPXPort),
						Apply: func(global *config.Config, value string) error {
							port, err := strconv.Atoi(value)
							if err != nil || port < 1 || port > 65535 {
								return fmt.Errorf("%q is not a port between 1 and 65535", value)
							}
							global.Geo.GPX.Port = port
							return nil
						},
					},
				},
			},
		},
	}
}
