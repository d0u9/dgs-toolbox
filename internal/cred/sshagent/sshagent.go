// Package sshagent hands an SSH private key to a running ssh-agent over its
// socket, as ssh-add does, without running ssh-add.
package sshagent

import (
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// EnvSocket names the agent's socket.
const EnvSocket = "SSH_AUTH_SOCK"

// ErrProtected is returned for a key that needs a passphrase first.
var ErrProtected = errors.New("the key is protected by a passphrase, which this does not unlock")

// Key is what to add.
type Key struct {
	// PEM is the private key file's content.
	PEM     []byte
	Comment string
	// Lifetime is how long the agent keeps the key; zero is for as long as the
	// agent runs.
	Lifetime time.Duration
	// Confirm makes the agent ask before each use.
	Confirm bool
}

// Add adds key to the agent listening on socket.
func Add(socket string, key Key) error {
	if socket == "" {
		return fmt.Errorf("%s is not set, so there is no agent to add to", EnvSocket)
	}
	raw, err := ssh.ParseRawPrivateKey(key.PEM)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		return ErrProtected
	}
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return fmt.Errorf("the agent at %s: %w", socket, err)
	}
	defer conn.Close()
	added := agent.AddedKey{PrivateKey: raw, Comment: key.Comment, ConfirmBeforeUse: key.Confirm}
	if key.Lifetime > 0 {
		added.LifetimeSecs = uint32(max(1, key.Lifetime/time.Second))
	}
	if err := agent.NewClient(conn).Add(added); err != nil {
		return fmt.Errorf("the agent refused the key: %w", err)
	}
	return nil
}
