# Patches on top of Matir/sshdog (for Android / Termux)

1. **go.rice removed.** Upstream reads its config through go.rice,
   whose `init()` opens the running executable as an appended zip
   (go.zipexe → debug/elf) and panics with a nil pointer dereference
   on Android before `main()` runs. Configuration is now a plain
   directory, searched in this order: `$SSHDOG_CONFIG_DIR`,
   `config/` next to the executable, `config/` in the working
   directory. (Also fixed: the module path case mismatch —
   `github.com/Matir/sshdog` vs lowercase imports — so the local
   daemon/pty packages are used instead of the published module.)
2. **Password-file auth (from the sshdog_ios fork).** If a file named
   `password` exists in the config directory, its contents are the
   login password, and password authentication is enabled. It can be
   combined with `authorized_keys` (public-key auth). Two deliberate
   differences from the iOS fork: a trailing CR/LF in the password
   file is not part of the password, and when neither a password nor
   authorized_keys is configured the server refuses to start instead
   of serving without authentication.

3. **SSHDOG_SHELL (from the sshdog_ios fork).** When the
   `SSHDOG_SHELL` environment variable names an existing file, that
   program is used as the shell for sessions and exec commands
   (e.g. `SSHDOG_SHELL=$PREFIX/bin/bash` on Termux, which has no
   `/bin/sh`); otherwise the platform default applies.

4. **PTY sessions fixed (upstream bug).** `pty_linux.go` passed the
   parent's tty fd number as `SysProcAttr.Ctty`, but Go expects the
   fd number in the child (the tty is wired to stdin, so 0). Every
   PTY session — i.e. every interactive login — died instantly while
   the server logged a misleading clean "Finished execution."
   Reproduced with `ssh -tt` on Linux, fixed by setting `Ctty = 0`,
   and `ExecuteForChannel` now logs start errors as errors.

Config files (all optional except one auth method):
`port` (default 2222), `password`, `authorized_keys`,
`ssh_host_rsa_key` / `ssh_host_ecdsa_key` / `ssh_host_dsa_key`
(random host key when absent), `daemon`, `quiet`.
