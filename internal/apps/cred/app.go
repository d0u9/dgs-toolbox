// Package cred is the dgs cred command domain: age-encrypted credentials, the
// identities on this machine that open them, and the hosts they are encrypted
// for. Its design is in docs/apps/cred/.
package cred

import (
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

// New returns the Credentials app definition. It opens a picker rather than a
// command directly, since the vault commands will join Keys.
func New() tui.App {
	return tui.App{
		ID:          "cred",
		Help:        appHelp,
		Name:        "Credentials",
		Description: "age identities, recipients and encrypted files",
		Commands: []tui.Command{{
			ID:          "keys",
			Help:        keysHelp,
			Name:        "Keys",
			Description: "This machine's identities, and the hosts and groups in the recipient folder.",
			New: func() tui.CommandModel {
				return newKeysModel()
			},
			NewWithConfig: func(global config.Config) tui.CommandModel {
				m := newKeysModel()
				m.path = global.CredentialsPath()
				return m
			},
		}, {
			ID:          "vault",
			Help:        vaultHelp,
			Name:        "Vault",
			Description: "The age files in a folder, and which this machine can open.",
			New: func() tui.CommandModel {
				return newVaultModel()
			},
			NewWithConfig: func(global config.Config) tui.CommandModel {
				m := newVaultModel()
				m.path = global.CredentialsPath()
				return m
			},
		}},
	}
}
