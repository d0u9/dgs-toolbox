// Package desktop hands things to the operating system's own applications:
// a URL to the browser, a file to the file manager.
package desktop

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// Open opens a URL or file in its default application.
func Open(target string) error {
	return start(openCommand(runtime.GOOS, target))
}

// Reveal shows path in the file manager: selected inside its folder where the
// file manager supports that (macOS, Windows), otherwise its folder opened.
func Reveal(path string) error {
	return start(revealCommand(runtime.GOOS, path))
}

func openCommand(goos, target string) []string {
	switch goos {
	case "darwin":
		return []string{"open", target}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", target}
	}
	return []string{"xdg-open", target}
}

func revealCommand(goos, path string) []string {
	switch goos {
	case "darwin":
		return []string{"open", "-R", path}
	case "windows":
		return []string{"explorer", "/select," + path}
	}
	return []string{"xdg-open", filepath.Dir(path)}
}

func start(argv []string) error {
	command := exec.Command(argv[0], argv[1:]...)
	if err := command.Start(); err != nil {
		return err
	}
	// Reap the process when it exits; the opened application outlives it.
	go func() { _ = command.Wait() }()
	return nil
}
