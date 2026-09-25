package apps

import (
	"dgs-toolbox/internal/apps/box"
	"dgs-toolbox/internal/apps/capture"
	"dgs-toolbox/internal/apps/conf"
	"dgs-toolbox/internal/apps/cred"
	"dgs-toolbox/internal/apps/demo"
	"dgs-toolbox/internal/apps/doc"
	"dgs-toolbox/internal/apps/geo"
	"dgs-toolbox/internal/apps/photo"
	"dgs-toolbox/internal/tui"
)

// All returns the complete toolbox app registry in display order.
func All() []tui.App {
	return []tui.App{demo.New(), capture.New(), photo.New(), box.New(), doc.New(), geo.New(), cred.New(), conf.New()}
}
