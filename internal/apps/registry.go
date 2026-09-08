package apps

import (
	"dgs-toolbox/internal/apps/demo"
	"dgs-toolbox/internal/apps/gpx"
	"dgs-toolbox/internal/apps/photo"
	"dgs-toolbox/internal/tui"
)

// All returns the complete toolbox app registry in display order.
func All() []tui.App {
	return []tui.App{demo.New(), photo.New(), gpx.New()}
}
