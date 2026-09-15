package recipients

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"strings"

	"filippo.io/age"
	"golang.org/x/crypto/ssh"
)

// KeyType is the kind of public key a recipient is.
type KeyType string

const (
	TypeX25519  KeyType = "age-x25519"
	TypeED25519 KeyType = "ssh-ed25519"
	TypeRSA     KeyType = "ssh-rsa"
)

// MinRSABits is the shortest RSA key accepted as a recipient.
const MinRSABits = 2048

// PublicKey is a parsed recipient key.
type PublicKey struct {
	Type KeyType
	// Key is the canonical form: the age1… string, or an SSH key's type and
	// base64 body without its comment. Two spellings of one key have the same
	// Key, so it is what keys are compared by.
	Key string
	// Fingerprint is the SHA256 fingerprint ssh-keygen -l shows, for SSH keys.
	// An age key is short enough to be shown whole and has none.
	Fingerprint string
}

// ParsePublicKey parses one recipient as written in a host file: an age X25519
// recipient, or an ssh-ed25519 or ssh-rsa public key with an optional trailing
// comment. Anything age cannot encrypt to is refused with the reason.
func ParsePublicKey(text string) (PublicKey, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return PublicKey{}, errors.New("public key is empty")
	}
	if strings.HasPrefix(text, "age1") {
		return parseAge(text)
	}
	return parseSSH(text)
}

func parseAge(text string) (PublicKey, error) {
	// Bech32's human-readable part runs to the last "1": "age" for an X25519
	// key, "age1pq" for a post-quantum one, "age1<name>" for a plugin's.
	switch hrp := text[:strings.LastIndex(text, "1")]; {
	case hrp == "age":
	case hrp == "age1pq":
		return PublicKey{}, errors.New("post-quantum age recipients (age1pq1…) are not supported")
	default:
		return PublicKey{}, fmt.Errorf("age plugin recipients (%s1…) are not supported", hrp)
	}
	recipient, err := age.ParseX25519Recipient(text)
	if err != nil {
		return PublicKey{}, err
	}
	return PublicKey{Type: TypeX25519, Key: recipient.String()}, nil
}

func parseSSH(text string) (PublicKey, error) {
	key, _, options, rest, err := ssh.ParseAuthorizedKey([]byte(text))
	if err != nil {
		return PublicKey{}, fmt.Errorf("not an age or SSH public key: %w", err)
	}
	if len(options) > 0 {
		return PublicKey{}, errors.New("SSH public key has authorized_keys options; write the key alone")
	}
	if len(strings.TrimSpace(string(rest))) > 0 {
		return PublicKey{}, errors.New("more than one SSH public key")
	}
	parsed := PublicKey{
		Type:        KeyType(key.Type()),
		Key:         strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))),
		Fingerprint: ssh.FingerprintSHA256(key),
	}
	switch key.Type() {
	case ssh.KeyAlgoED25519:
	case ssh.KeyAlgoRSA:
		crypto, ok := key.(ssh.CryptoPublicKey)
		if !ok {
			return PublicKey{}, errors.New("unreadable RSA key")
		}
		rsaKey, ok := crypto.CryptoPublicKey().(*rsa.PublicKey)
		if !ok {
			return PublicKey{}, errors.New("unreadable RSA key")
		}
		if bits := rsaKey.N.BitLen(); bits < MinRSABits {
			return PublicKey{}, fmt.Errorf("RSA key is %d bits, shorter than %d", bits, MinRSABits)
		}
	default:
		return PublicKey{}, fmt.Errorf("age cannot encrypt to %s keys; use ssh-ed25519, ssh-rsa or an age key", key.Type())
	}
	return parsed, nil
}
