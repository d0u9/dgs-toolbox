// Package cred is the dgs cred command domain: age-encrypted credentials, the
// identities on this machine that open them, and the hosts they are encrypted
// for. Its design is in docs/apps/cred/.
package cred

import (
	"dgs-toolbox/internal/tui"
)

// New returns the Credentials app definition. It opens a picker rather than a
// command directly, since the vault commands will join Keys.
func New() tui.App {
	return tui.App{
		ID:          "cred",
		Name:        "Credentials",
		Description: "age identities, recipients and encrypted files",
		Commands: []tui.Command{{
			ID:          "keys",
			Name:        "Keys",
			Description: "This machine's identities, and the hosts and groups in the recipient folder.",
			New: func() tui.CommandModel {
				return newKeysModel()
			},
		}},
	}
}
