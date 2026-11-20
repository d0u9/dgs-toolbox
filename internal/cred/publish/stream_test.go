package publish

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestStreamFailuresNeverPublish(t *testing.T) {
	for _, mode := range []string{"write", "verify", "readback", "race", "existing-part"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "result")
			part := path + ".dgs-part"
			if mode == "existing-part" {
				if err := os.WriteFile(part, []byte("other operation"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := Stream(context.Background(), path, 0600, 0700, func(w io.Writer) error {
				if _, err := io.WriteString(w, "synthetic"); err != nil {
					return err
				}
				switch mode {
				case "write":
					return errors.New("injected write failure")
				case "readback":
					return os.WriteFile(part, []byte("corrupted"), 0600)
				}
				return nil
			}, func(p string) error {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("published before verification")
				}
				if mode == "race" {
					return os.WriteFile(path, []byte("racing writer"), 0600)
				}
				return errors.New("injected verification failure")
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			if mode == "race" {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "racing writer" {
					t.Fatal("racing file replaced")
				}
			} else if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("final output created")
			}
			if mode == "existing-part" {
				data, err := os.ReadFile(part)
				if err != nil || string(data) != "other operation" {
					t.Fatal("foreign part changed")
				}
			} else if _, err := os.Stat(part); !os.IsNotExist(err) {
				t.Fatal("part not cleaned")
			}
		})
	}
}

func TestStreamRejectsPartChangesAndCancellation(t *testing.T) {
	for _, mode := range []string{"replace-before-readback", "replace-after-readback", "modify-after-readback", "cancel-write", "cancel-verify"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := filepath.Join(t.TempDir(), "result")
			part := path + ".dgs-part"
			replace := func() error {
				if err := os.Rename(part, part+".old"); err != nil {
					return err
				}
				return os.WriteFile(part, []byte("foreign replacement"), 0600)
			}
			err := Stream(ctx, path, 0600, 0700, func(w io.Writer) error {
				if _, err := io.WriteString(w, "synthetic original"); err != nil {
					return err
				}
				if mode == "replace-before-readback" {
					return replace()
				}
				if mode == "cancel-write" {
					cancel()
					_, err := io.WriteString(w, "more")
					return err
				}
				return nil
			}, func(string) error {
				switch mode {
				case "replace-after-readback":
					return replace()
				case "modify-after-readback":
					return os.WriteFile(part, []byte("changed after verification"), 0600)
				case "cancel-verify":
					cancel()
				}
				return nil
			})
			if err == nil {
				t.Fatal("changed or cancelled operation succeeded")
			}
			if mode == "cancel-write" || mode == "cancel-verify" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("%v", err)
				}
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("published invalid output")
			}
			if mode == "replace-before-readback" || mode == "replace-after-readback" {
				got, err := os.ReadFile(part)
				if err != nil || string(got) != "foreign replacement" {
					t.Fatal("foreign part removed or changed")
				}
			} else if _, err := os.Stat(part); !os.IsNotExist(err) {
				t.Fatal("own part not cleaned")
			}
		})
	}
}
