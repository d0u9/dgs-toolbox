// This file is `dgs conf reservations <network>`: what a network's router
// is to be given. See docs/apps/conf/inventory.md#what-the-router-is-given.
package conf

import (
	"fmt"
	"github.com/d0u9/rhumb/engine"
	"io"
	"text/tabwriter"

	"dgs-toolbox/internal/config"
	"github.com/d0u9/rhumb/derive"
)

func reservationsAction(in io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
	root := global.ConfRoot()
	if root == "" {
		return fmt.Errorf("conf.root is not configured")
	}
	l, err := engine.Load(root)
	if err != nil {
		return err
	}
	if _, ok := l.Inv.NetworkInfo[args[0]]; !ok {
		return fmt.Errorf("network %q is not declared in networks.yaml", args[0])
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, r := range derive.Reservations(l.Inv, args[0]) {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Address, r.MAC, r.ID)
	}
	return w.Flush()
}
