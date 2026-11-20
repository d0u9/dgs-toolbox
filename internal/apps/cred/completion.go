package cred

import (
	"fmt"
	"strings"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/cred/recipients"
)

func completeHosts(prefix string, global config.Config) ([]string, error) {
	return completeRecipients(prefix, global, false)
}

func completeGroups(prefix string, global config.Config) ([]string, error) {
	return completeRecipients(prefix, global, true)
}

func completeRecipients(prefix string, global config.Config, groups bool) ([]string, error) {
	settings, _, err := config.LoadCredentials(global.CredentialsPath())
	if err != nil {
		return nil, err
	}
	if settings.Recipients == "" {
		return nil, nil
	}
	folder, err := recipients.Load(settings.Recipients)
	if err != nil {
		return nil, err
	}
	if folder.Errors() {
		return nil, fmt.Errorf("recipient folder has problems; check dgs cred key hosts and dgs cred key groups")
	}
	var names []string
	if groups {
		for _, group := range folder.Groups {
			if strings.HasPrefix(group.Name, prefix) {
				names = append(names, group.Name)
			}
		}
	} else {
		for _, host := range folder.Hosts {
			if strings.HasPrefix(host.Name, prefix) {
				names = append(names, host.Name)
			}
		}
	}
	return names, nil
}
