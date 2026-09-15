# Credentials Roadmap

The parts of `dgs cred` after [keys](keys.md), in the order they build on each
other. Each is designed in its own document when it is reached; what is written
here is only the shape agreed so far.

- **B — Browsing.** List every `.age` file under an opened folder as a tree, and
  mark each as decryptable, not decryptable, needing a passphrase, or damaged, by
  trying local identities against the header only.
- **C — Decrypted view.** Decrypt a file into memory, list its contents (a single
  file, or an archive expanded into a tree), view them inside the TUI, and clear
  the plaintext on leaving.
- **D — Actions.** Actions chosen by file type, catalogued in a document of their
  own: copy to a folder, install an SSH key into `~/.ssh`, add an SSH key to the
  agent. More are added as files need them.
- **E — Encrypting and recipients.** Encrypt a file or an archived folder for
  chosen recipients; add or remove recipients of an existing file by
  re-encrypting it; remove or replace a host across every file. Publication goes
  through a `.part` file checked by decrypting it back before it replaces the
  original. Because an age file does not name its recipients, E needs a record
  of who each file was encrypted for. Removing a recipient protects only later
  versions: an older copy stays readable to it.
