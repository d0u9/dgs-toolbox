package bundled

import (
	"strings"
	"testing"
)

var testPaths = Paths{
	Public:    "Lib/Public",
	QuickAdd:  "Lib/QuickAdd",
	Templates: "Templater/DGS",
	Timelines: "Toolkit/timelines.json",
	Entries:   "Toolkit/Entry Templates",
}

// Every placeholder a file carries is replaced with the configured path: one
// left behind is a folder the plugin or a script looks for and never finds.
func TestEveryPlaceholderIsReplaced(t *testing.T) {
	for _, plugin := range All(testPaths) {
		files, err := plugin.Files()
		if err != nil {
			if plugin.ID == "dgs-toolbox" {
				t.Skipf("%s: %v (run make plugins first)", plugin.Name(), err)
			}
			t.Fatalf("%s: %v", plugin.Name(), err)
		}
		for name, data := range files {
			if text(name) && strings.Contains(string(data), "{{dgs:") {
				t.Errorf("%s/%s keeps a {{dgs:…}} placeholder", plugin.Name(), name)
			}
		}
	}
}

// The plugin's default settings point at the configured folders, the entry
// templates among them, so a phone's configuration folder that has no
// settings of its own still finds them.
func TestThePluginDefaultsNameTheConfiguredFolders(t *testing.T) {
	files, err := obsidianToolbox(testPaths).Files()
	if err != nil {
		t.Skipf("%v (run make plugins first)", err)
	}
	main := string(files["main.js"])
	for _, want := range []string{
		"timelinesFile: 'Toolkit/timelines.json'",
		"libFolder: 'Lib/Public'",
		"entriesFolder: 'Toolkit/Entry Templates'",
	} {
		if !strings.Contains(main, want) {
			t.Errorf("main.js lacks %s", want)
		}
	}
}
