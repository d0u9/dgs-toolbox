package conf

import (
	"fmt"
	"io"

	"dgs-toolbox/internal/config"
	rcli "github.com/d0u9/rhumb/cli"
)

// The command-line half of dgs conf is rhumb's: these adapt dgs's
// configuration to the directories rhumb's commands take.

func settings(g config.Config) rcli.Settings {
	return rcli.Settings{Root: g.ConfRoot(), Secrets: g.ConfSecrets(), ExportDir: g.ConfExportDir()}
}

func writeCheckReport(out io.Writer, g config.Config) error  { return rcli.Check(out, settings(g)) }
func writeTargetReport(out io.Writer, g config.Config) error { return rcli.Targets(out, settings(g)) }

func exportAction(in io.Reader, out io.Writer, args []string, flags map[string]string, g config.Config) error {
	if flags["bundle"] == "true" {
		return exportBundles(in, out, args, flags, g)
	}
	if flags["download"] != "" || flags["services"] != "" {
		return fmt.Errorf("--download and --services choose how a bundle is built; they need --bundle")
	}
	return rcli.Export(in, out, args, flags, settings(g))
}

func secretAction(in io.Reader, out io.Writer, args []string, flags map[string]string, g config.Config) error {
	return rcli.Secret(in, out, args, flags, settings(g))
}

func initAction(in io.Reader, out io.Writer, args []string, flags map[string]string, g config.Config) error {
	return rcli.Init(in, out, args, flags, settings(g))
}

func reservationsAction(in io.Reader, out io.Writer, args []string, flags map[string]string, g config.Config) error {
	return rcli.Reservations(in, out, args, flags, settings(g))
}
