package sshagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

func serve(t *testing.T) (string, agent.Agent) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "agent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	keyring := agent.NewKeyring()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(keyring, conn)
		}
	}()
	return socket, keyring
}

func TestAdd(t *testing.T) {
	socket, keyring := serve(t)
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(private, "")
	err := Add(socket, Key{PEM: pem.EncodeToMemory(block), Comment: "nas-keys.tar.gz.age/server1", Lifetime: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := keyring.List()
	if len(keys) != 1 || keys[0].Comment != "nas-keys.tar.gz.age/server1" {
		t.Errorf("keys %+v", keys)
	}

	protected, _ := ssh.MarshalPrivateKeyWithPassphrase(private, "", []byte("p"))
	if err := Add(socket, Key{PEM: pem.EncodeToMemory(protected)}); !errors.Is(err, ErrProtected) {
		t.Errorf("protected: %v", err)
	}
	if err := Add("", Key{PEM: pem.EncodeToMemory(block)}); err == nil {
		t.Error("no socket accepted")
	}
}
