package cred

const encryptHelp = `Encrypt one file or directory to existing recipient hosts/groups.

Usage: dgs cred e <source> [-H <host>]... [-G <group>]... [-o <file>]
The long command name encrypt is an alias for e.

Choose at least one host or group. Hosts contribute all their public keys;
groups contribute all their hosts. Repeat -H and -G to combine recipients;
duplicate keys are used once. Shared-key warnings do not block encryption. Names ignore case; groups include their g- prefix.
Use dgs cred key hosts and dgs cred key groups to list available names.

By default, a file becomes <source>.age beside the source. A directory becomes
<source>.tar.gz.age, keeping its directory name and structure. Directory archives
use archive_skip from cred/config.json. -o / --output chooses a FILE name, not a
directory; its parent must exist, and directory output must be outside the source.
Sources are kept. Existing output files, directories, links and .dgs-part files
are refused. Use -- before a source filename beginning with -.

Files and archives are streamed without an application size limit. Output is
read back before publication; matching unlocked local identities additionally
verify it by decryption. Without one, success explicitly reports that decryption
verification was unavailable. Passphrase mode and protected identities require
the vault TUI. Neither stdin nor stdout streaming is supported.

Settings come from the active cred/config.json; --config / -c chooses its parent
configuration directory. Encryption does not use a vault or write a vault record. Private identities stay
in memory; no extra key files or encryption helper processes are created.
Bash/Zsh completion offers configured names for -H/--host and -G/--group.

Examples:
  # Encrypt for one host, using the default output notes.txt.age.
  dgs cred e notes.txt -H host-a

  # Encrypt for several hosts and groups, choosing the output name.
  dgs cred e notes.txt -H host-a -H host-b -G g-team -G g-backup -o backup.age

  # Archive and encrypt a directory to photos.tar.gz.age.
  dgs cred e photos/ -G g-team

  # Choose a different configuration directory.
  dgs --config /path/to/config-directory cred e notes.txt -G g-team

  # Treat a dash-prefixed filename as the source, not an option.
  dgs cred e -H host-a -- -notes.txt`

const decryptHelp = `Decrypt one age file to a plaintext file using local identities.

Usage: dgs cred d <source> [-o <file>]
The long command name decrypt is an alias for d.

By default, remove the final .age suffix to choose the output name. Without that
suffix, -o / --output is required. -o chooses a FILE name, not a directory.
Sources are kept; an existing output is never overwritten, including the original
plaintext left beside a newly encrypted file. Use -o to restore under another name.
Use -- before a source filename beginning with -.

Binary and armored age files are accepted. Identities come from the active
cred/config.json; --config / -c chooses its parent configuration directory.
Passphrase files and protected identities require the vault TUI. Hosts and groups
are encryption options; decryption uses local private identities in memory and
never writes extra key files.

Plaintext is streamed without an application size limit to a 0600 .dgs-part file.
The final filename is published only after complete authenticated decryption and
SHA-256 disk readback succeed. Failures and normal Ctrl+C/SIGTERM clean up this
operation's own part file; forced termination may leave an unpublished part.
pre-existing parts are refused. Missing parent directories are created as 0700.
Archives remain archives; no files are automatically extracted from them.
Neither stdin nor stdout streaming is supported.

Examples:
  # Restore under a new name while keeping notes.txt.age.
  dgs cred d notes.txt.age -o restored.txt

  # Restore to notes.txt when that name does not already exist.
  dgs cred d notes.txt.age

  # Decrypt a directory archive; extract the resulting tar.gz separately.
  dgs cred d photos.tar.gz.age -o restored-photos.tar.gz

  # A renamed ciphertext needs an explicit output name.
  dgs cred d backup.bin -o restored.txt

  # Choose a different configuration directory.
  dgs --config /path/to/config-directory cred d notes.txt.age -o restored.txt`
