package conf

import (
	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

// loaded is the bootstrap every dgs conf command starts from: the inventory,
// the service manifests, and their derivation. export's tree and inspect's
// four views both read the same one.
type loaded struct {
	inv         *inventory.Root
	manifests   map[string]confgen.Manifest
	serviceDirs map[string]string
	derived     *derive.Model
}

// load reads rootPath's inventory and services/, and derives from both.
func load(rootPath string) (loaded, error) {
	var l loaded

	inv, err := inventory.Load(rootPath)
	if err != nil {
		return l, err
	}
	l.inv = inv

	confRoot, err := confgen.Load(rootPath)
	if err != nil {
		return l, err
	}
	manifests := map[string]confgen.Manifest{}
	serviceDirs := map[string]string{}
	for _, svc := range confRoot.Services {
		if svc.Broken == "" {
			manifests[svc.Name] = svc.Manifest
			serviceDirs[svc.Name] = svc.Dir
		}
	}
	l.manifests = manifests
	l.serviceDirs = serviceDirs

	model, err := derive.Derive(inv, manifests)
	if err != nil {
		return l, err
	}
	l.derived = model

	return l, nil
}
