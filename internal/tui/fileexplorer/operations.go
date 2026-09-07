package fileexplorer

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

type actionMode int

const (
	actionNone actionMode = iota
	actionCreate
	actionRename
	actionDelete
	actionPath
)

type operationKind int

const (
	createDirectory operationKind = iota
	renameEntry
	deleteEntry
)

type operationMsg struct {
	id          int64
	kind        operationKind
	source      string
	destination string
	err         error
}

func runOperation(id int64, kind operationKind, source, destination string) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch kind {
		case createDirectory:
			err = os.Mkdir(destination, 0o755)
		case renameEntry:
			err = os.Rename(source, destination)
		case deleteEntry:
			err = os.RemoveAll(source)
		}
		return operationMsg{id: id, kind: kind, source: source, destination: destination, err: err}
	}
}

func childPath(parent, name string) (string, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("enter a single valid name")
	}
	return filepath.Join(parent, name), nil
}
