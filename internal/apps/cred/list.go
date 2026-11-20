package cred

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/tui"
)

func listAction(kind string) tui.Action {
	return tui.Action{
		ID:          kind,
		Description: "List recipient " + kind + " without opening the TUI.",
		Help:        "Read hosts/groups from recipients in cred/config.json. No vault or private keys are opened.\nExamples:\n  dgs cred key " + kind + "\n  dgs --config /path/to/config-directory cred key " + kind,
		RunWithConfig: func(_ io.Reader, out io.Writer, _ []string, _ map[string]string, global config.Config) error {
			return listRecipients(out, global.CredentialsPath(), kind)
		},
	}
}

func listRecipients(out io.Writer, path, kind string) error {
	settings, _, err := config.LoadCredentials(path)
	if err != nil {
		return err
	}
	if settings.Recipients == "" {
		return fmt.Errorf("no recipient folder configured: set recipients in cred/config.json")
	}
	folder, err := recipients.Load(settings.Recipients)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if kind == "hosts" {
		if _, err := fmt.Fprintln(w, "HOST\tKEYS"); err != nil {
			return err
		}
		for _, host := range folder.Hosts {
			if _, err := fmt.Fprintf(w, "%s\t%d\n", host.Name, len(host.Keys)); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(w, "GROUP\tHOSTS"); err != nil {
			return err
		}
		for _, group := range folder.Groups {
			if _, err := fmt.Fprintf(w, "%s\t%s\n", group.Name, strings.Join(group.Hosts, ", ")); err != nil {
				return err
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if err := writeRecipientWarnings(out, folder); err != nil {
		return err
	}
	return recipientErrors(folder)
}

func writeRecipientWarnings(out io.Writer, folder recipients.Folder) error {
	for _, p := range folder.Problems {
		if p.Severity == recipients.Warning {
			if _, err := fmt.Fprintf(out, "warning: %s: %s\n", p.File, p.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func recipientErrors(folder recipients.Folder) error {
	var problems []string
	for _, p := range folder.Problems {
		if p.Severity == recipients.Error {
			problems = append(problems, fmt.Sprintf("%s: %s", p.File, p.Message))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("recipient folder has errors:\n%s\ncheck dgs cred key hosts and dgs cred key groups", strings.Join(problems, "\n"))
	}
	return nil
}
