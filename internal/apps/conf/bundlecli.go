// This file is the command line's two ways to a deploy bundle:
// `dgs conf bundle`, which is `rhumb deploy build` over an export already on
// disk, and `dgs conf export --bundle`, the inspect page's Bundle format,
// where a selector chooses the instances and each one whose export has a
// manifest becomes a bundle.
package conf

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/config"
	rcli "github.com/d0u9/rhumb/cli"
	"github.com/d0u9/rhumb/deploy"
	"github.com/d0u9/rhumb/engine"
	"github.com/d0u9/rhumb/target"
)

// bundleAction runs `dgs conf bundle <export-dir> --to <bundle>`: one
// instance's export, as rhumb export or dgs conf export wrote it, built into
// the bundle ctl installs. It reads the export, never the root, so it needs
// neither conf.root nor conf.secrets.
func bundleAction(_ io.Reader, out io.Writer, args []string, flags map[string]string, g config.Config) error {
	to := flags["to"]
	if to == "" {
		return fmt.Errorf("no destination: give --to <bundle>")
	}
	opt, err := bundleOptions(flags, g)
	if err != nil {
		return err
	}
	opt.Platform = flags["platform"]
	opt.Binary = expandHome(flags["bin"])
	to = expandHome(to)
	if err := deploy.Build(expandHome(args[0]), to, opt); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s; install with %s/ctl install\n", to, to)
	return nil
}

// bundleOptions are the choices every bundle built from the command line
// shares: --download, checked, and --services, else conf.services.
func bundleOptions(flags map[string]string, g config.Config) (deploy.Options, error) {
	opt := deploy.Options{Download: flags["download"], Services: g.ConfServices()}
	if opt.Download != "" && opt.Download != deploy.DownloadBuild && opt.Download != deploy.DownloadInstall {
		return opt, fmt.Errorf("unknown --download %q; give %s or %s", opt.Download, deploy.DownloadBuild, deploy.DownloadInstall)
	}
	if dir := flags["services"]; dir != "" {
		opt.Services = expandHome(dir)
	}
	return opt, nil
}

// exportBundles runs `dgs conf export <selector>... --bundle`.
func exportBundles(in io.Reader, out io.Writer, args []string, flags map[string]string, g config.Config) error {
	s := settings(g)
	where := flags["to"]
	switch {
	case flags["zip"] != "":
		return fmt.Errorf("--bundle writes a directory per instance; it does not take --zip")
	case where == "-":
		return fmt.Errorf("--bundle writes a directory per instance; it cannot write to stdout")
	case flags["format"] != "":
		return fmt.Errorf("--format writes one document to stdout; --bundle does not")
	}
	opt, err := bundleOptions(flags, g)
	if err != nil {
		return err
	}
	if where == "" {
		where = s.ExportDir
	}
	if where == "" {
		return fmt.Errorf("no destination: give --to <dir>")
	}
	where = expandHome(where)
	if s.Root == "" {
		return fmt.Errorf("no generator root given")
	}

	l, err := engine.Load(s.Root)
	if err != nil {
		return err
	}
	r := engine.Renderer{Data: l, RootPath: s.Root, SecretsDir: s.Secrets}
	selector := strings.Join(args, " ")
	matched, err := target.Match(selector, target.List(l.Inv, l.Derived))
	if err != nil {
		return err
	}
	if r.Routes, err = target.Routes(selector, matched); err != nil {
		return err
	}
	var instances, broken []string
	for _, t := range matched {
		if t.Broken != "" {
			broken = append(broken, fmt.Sprintf("%s: %s", t.Instance, t.Broken))
			continue
		}
		instances = append(instances, t.Instance)
	}
	if len(broken) > 0 {
		return fmt.Errorf("selector matched %s that cannot be rendered:\n  %s",
			plural(len(broken), "target"), strings.Join(broken, "\n  "))
	}
	sort.Strings(instances)

	// Render and describe everything before building anything, so a bundle
	// that cannot be built stops the export before the first is written.
	files, err := r.RenderAll(instances)
	if err != nil {
		return err
	}
	dirs := bundleDirs(files)
	if len(dirs) == 0 {
		return fmt.Errorf("no matched instance has a manifest to build a bundle from")
	}
	described, err := describeBundles(files, opt)
	if err != nil {
		return err
	}

	var existing []string
	fmt.Fprintf(out, "%s in %s:\n", plural(len(dirs), "bundle"), where)
	for _, dir := range dirs {
		suffix := ""
		if _, err := os.Lstat(filepath.Join(where, dir)); err == nil {
			existing = append(existing, dir)
			suffix = "  (rebuilds)"
		}
		fmt.Fprintf(out, "  %s/%s\n    %s\n", dir, suffix, described[dir])
	}
	fmt.Fprintln(out, "\nEach holds ctl and the rendered files, plaintext, with every credential in them.")

	// As for any export, --yes answers the question but does not decide
	// that replacing a bundle already on disk is what was meant.
	if len(existing) > 0 && flags["overwrite"] != "true" {
		if flags["yes"] == "true" {
			return fmt.Errorf("%s already %s; pass --overwrite to rebuild %s",
				plural(len(existing), "bundle"), rcli.Exists(len(existing)), rcli.Them(len(existing)))
		}
		ok, err := rcli.Confirm(in, out, fmt.Sprintf("%s already %s. Rebuild %s and build the rest? [y/N] ",
			plural(len(existing), "bundle"), rcli.Exists(len(existing)), rcli.Them(len(existing))))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "nothing written")
			return nil
		}
	} else if flags["yes"] != "true" {
		ok, err := rcli.Confirm(in, out, "Build them? [y/N] ")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "nothing written")
			return nil
		}
	}

	if err := writeBundles(r, instances, dirs, where, opt); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s; copy one to its machine and run ./ctl install there\n", plural(len(dirs), "bundle"))
	return nil
}

// userHome is where bundleGCAction looks; a test points it elsewhere.
var userHome = os.UserHomeDir

// bundleGCAction runs `dgs conf bundle-gc [--yes]`, which is
// `rhumb deploy gc`: it lists the launchd agents and ~/.local/bin shims an
// installed bundle left behind when its directory was deleted rather than
// uninstalled, and with --yes removes them. It runs on the machine the
// bundles were installed on.
func bundleGCAction(_ io.Reader, out io.Writer, _ []string, flags map[string]string, _ config.Config) error {
	home, err := userHome()
	if err != nil {
		return err
	}
	left, err := deploy.Leftovers(home)
	if err != nil {
		return err
	}
	if len(left) == 0 {
		fmt.Fprintln(out, "nothing left behind")
		return nil
	}
	yes := flags["yes"] == "true"
	for _, l := range left {
		fmt.Fprintf(out, "%-7s %s (bundle %s is gone)\n", l.Kind, l.Path, l.Bundle)
		if yes {
			if err := l.Remove(); err != nil {
				return fmt.Errorf("%s: %w", l.Path, err)
			}
		}
	}
	if !yes {
		fmt.Fprintln(out, "run again with --yes to remove these")
	}
	return nil
}
