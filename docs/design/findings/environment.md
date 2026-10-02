# Findings: environment

What the programs around cld do, apart from tmux, claude and the terminals. That takes in shells and
their completion, cobra, the install script and GitHub, git, and Go and Linux. It also takes in
Docker and the collector, JetBrains IDEs and systemd. The shell entries on "the script" describe cld
up to 0.3.0, a bash script that ended in `exec tmux`.

## Shells

- **The environment the bash script handed tmux** (bash 5.3.9 and 3.2.57, run). bash exported `PWD`,
  and `SHLVL=0` where it got none, and dropped `_`, `PS1`, `OLDPWD` and others of its own. It passed
  on `SHELLOPTS` with the script's `errexit`, `nounset` and `pipefail`, and Debian's 5.2.15 did as
  5.3.9. The Go cld hands on what it got, but for `TMUX` and the terminal's variables
  ([decision 11](../decisions/0011-go-and-cobra.md),
  [decision 33](../decisions/0033-terminal-variables.md)).
- **The script's name check by locale** (bash 5.3.9 and glibc 2.43, run; macOS not checked).
  `[[ =~ ]]` follows the locale's collation: under `en_US.UTF-8`, `[A-Za-z0-9]` matched `é`, `ß` and
  `①`, so `cld new -n café` made `cld-café`. `printf '%-*s'` pads by bytes, so such a name broke
  `list`'s columns ([decision 11](../decisions/0011-go-and-cobra.md)).
- **Where bash stepped in for the script** (bash 5.3.9 on Ubuntu 26.04 unless noted, run). A failed
  write ended it under `set -e` with status 1, or by SIGPIPE, before its `exec` of tmux. A missing
  `#!` interpreter gave 127 in Debian's 5.2.15 and 5.2.37 and in Ubuntu's 5.2.21. It gave 126 in
  5.3.9 and 3.2.57. bash's search, `command -v` too, took a file without the execute permission
  where no other had one ([decision 11](../decisions/0011-go-and-cobra.md)).
- **Job control in a `sh -c` script under `set -m`**. Checked with macOS's `sh` and Apple's bash
  3.2.57, read at tag `bash-144` and run on CI's macOS 26 runner. Also with GNU bash 3.2.57 and
  5.3.9, and with dash 0.5.12 as well. Apple's bash notices a stopped job only when interactive, so
  a script waits on for it and never runs the rest. bash 3.2.57 also reports a background job's end
  on stderr while job control is on. So the list's stop cases run under `sh -i` (`startJob` in
  [session_jobs_test.go](../../../tests/session_jobs_test.go)).
- **A word quoted for the user's shell** (#112). Checked in the test image of tmux 3.7c, with fish
  4.0.2, zsh 5.9, bash 5.2.37 and dash 0.5.12 alike. Within single quotes fish reads `\\` and `\'`
  as escapes, so `x\'; echo INJECTED #`, quoted as `sh` quotes it, ran `echo INJECTED` in fish. A
  word with each `'` and `\` outside the quotes, after a `\`, reached all four unchanged
  ([decision 51](../decisions/0051-moving-between-sessions.md)).

## Completion

- **cobra 1.10.2's `__complete`** (read in the source; run with cld). It hands a flag's function the
  text after `-n `, `--name `, `--name=` and `-n=`, but takes `-nre` for an option name and offers
  none. An argument without a function, or a line cobra cannot read, gets the directive `:0`, on
  which shells offer file names. A failing root `PersistentPreRunE` leaves stdout empty, which bash
  takes for `:0` too ([decision 17](../decisions/0017-shell-completion.md)).
- **`cld join -n <TAB>` in bash**, typed in a pane of tmux 3.5a. Checked with bash 3.2.57 and
  bash-completion 1.3 as Homebrew builds it, and with bash 5.2.37 and bash-completion 2.16 on Debian
  trixie. Both list names and states, but bash 3.2 has no `compopt`, so it adds no space and offers
  file names where cld offers none. `source <(cld completion bash)` loads nothing in bash 3.2
  ([decision 17](../decisions/0017-shell-completion.md)).
- **The same in zsh and fish** (zsh 5.9 with `compinit`, fish 4.0.2; Debian trixie, tmux 3.5a). Both
  list names and states on the first TAB, add a space and offer nothing where cld offers nothing.
  fish still shows a grey autosuggestion of a file name, which TAB does not insert
  ([decision 17](../decisions/0017-shell-completion.md)).
- **Where zsh 5.9 looks for `_cld`** (Debian trixie, an ordinary user; tmux 3.7c). `${fpath[1]}` is
  root's `/usr/local/share/zsh/site-functions`, so cobra's advice to write there fails. A directory
  of the user's, put on `$fpath` before `compinit`, works
  ([decision 22](../decisions/0022-setting-completion-up.md)).
- **Where bash-completion 2.16, zsh 5.9 and fish 4.0.2 read a user's scripts**. Checked on Debian
  trixie as new users, and in Ubuntu 24.04's `/etc/zsh/zshrc`. bash-completion reads
  `$BASH_COMPLETION_USER_DIR`, else `${XDG_DATA_HOME:-~/.local/share}/bash-completion`, and fish
  `~/.config/fish/completions` first, made only at fish's first interactive start. Ubuntu runs
  `compinit` for every user and Debian does not; `source <(cld completion zsh)` needs `compinit` and
  cld on the `PATH` before it ([decision 22](../decisions/0022-setting-completion-up.md)).
- **The lines `cld setup completion zsh` adds to `.zshrc`** (the same systems). They register `_cld`
  from cld's file before or after the user's `compinit`, follow `$XDG_DATA_HOME` and `$ZDOTDIR`, and
  do nothing where the script is missing
  ([decision 22](../decisions/0022-setting-completion-up.md)).
- **cld's completion in bash with ble.sh**. Checked with ble.sh `0.4.0~git20250806.8060b7a`, Ubuntu
  26.04's package, and the nightly `0.4.0-nightly+d81fd54`. They ran in a tmux 3.7c pane under bash
  5.3.9, with bash-completion 2.16 and cobra 1.10.2 as well. ble.sh keeps `-o default` and adds
  completions of its own, so it offers file names where cld offers none.
  `compopt +o default +o ble/default` at the end of `__start_cld` stops that, where ble.sh 0.3.4 has
  no way to ([decision 27](../decisions/0027-completion-with-blesh.md)).
- **`--mcp=go` with ble.sh** (the same versions). It completes file names, as ble.sh's adapter keeps
  only the answers that start with the whole word. With cld's lines it completes nothing, and
  `--mcp go` completes as in bash ([decision 27](../decisions/0027-completion-with-blesh.md)).
- **Where ble.sh keeps its cache** (`0.4.0~git20250806.8060b7a`, bash 5.3.9, Ubuntu 26.04; as CI
  runs it). It uses `${XDG_CACHE_HOME:-~/.cache}/blesh` only where that exists, else the package's
  directory, which only root may write. Another user's bash then goes on without ble.sh
  ([decision 27](../decisions/0027-completion-with-blesh.md)).

## cobra, install.sh and GitHub

- **The help cobra 1.10.2 generates** (pflag 1.0.9; a test program, run). Commands are sorted unless
  `EnableCommandSorting` is false, and `help` then goes last. A flag's value shows as its type
  unless its usage names it in backquotes, and nothing is wrapped. The help drops a failed write and
  exits 0 ([decision 12](../decisions/0012-help-from-cobra.md)).
- **`install.sh` against v0.4.0** (curl 8.18.0; dash 0.5.12, bash 5.3.9 and busybox's sh on Ubuntu
  26.04, run). `releases/latest/download/cld.sha256` redirected (302) to the release, then to
  `release-assets.githubusercontent.com`. Each shell installed `cld 0.4.0`, matching the
  `HASH  cld-OS-ARCH` line of `cld.sha256`, and a missing version ended at curl's 404
  ([decision 20](../decisions/0020-install-script.md)).
- **`releases/latest` of cld's repository** (v0.5.0 the latest; curl 8.18.0, GET and HEAD). It
  answered 302 to `releases/tag/v0.5.0`, and a missing release or repository 404. A cld built as
  0.4.0, run through a symbolic link, updated the link's target
  ([decision 21](../decisions/0021-self-update.md)).

## git

- **`/.claude/*`, then `!/.claude/settings.json`, in `.gitignore`** (git 2.53.0 on Ubuntu 26.04 and
  2.47.3 in the test image). git ignores `settings.local.json` and keeps `settings.json`, unless an
  earlier `.claude/` or a later `*.json` ignores it again. `git check-ignore -v` names the last
  matching pattern, an exception too, and exits 0 either way
  ([decision 19](../decisions/0019-project-settings.md)).
- **`git check-ignore --verbose -z --stdin` on paths under `.claude`** (the same versions). The two
  lines above ignore `.claude/commands/`, `skills/`, `CLAUDE.md` and the rest, so `git add` refuses
  a new skill. The check skips tracked paths and directories of files added with `-f`, which
  `--no-index` does not, and fails on a path beyond a symbolic link
  ([decision 28](../decisions/0028-shared-project-settings.md)).
- **`git init -q DIR` under `git rebase --exec`** (git 2.53.0). In a linked worktree `GIT_DIR` is
  set, so `git init` turned the worktree's git directory into a bare repository, breaking the main
  work tree. So `TestMain` unsets every `GIT_*` variable
  ([main_test.go](../../../tests/main_test.go)).
- **`git rev-parse --git-common-dir`** (git 2.47.3 in the test image). It names the main work tree's
  `.git` from the work tree, a subdirectory or a linked worktree, and the bare repository from its
  worktrees. In a submodule it names `.git/modules/PATH` of the superproject
  ([decision 24.3](../decisions/0024-names-from-the-repository.md)).
- **The same through a symbolic link** (git 2.53.0 on Ubuntu 26.04 and 2.47.3 in the test image).
  From the main work tree and a subdirectory it gives a relative path, which `PWD` resolves through
  the link; from a linked worktree, the real path. `os.SameFile` finds the two paths one directory
  ([decision 37](../decisions/0037-sessions-of-another-repository.md)).

## Go and Linux

- **SIGTSTP once `os/signal` has had it** (Go 1.27.1, Linux 7.0). After `signal.Stop` or
  `signal.Reset` Go drops it, so `kill -TSTP` stops nothing. `kill(getpid(), SIGSTOP)` returns
  before the process stops ([decision 14](../decisions/0014-the-session-list.md)).
- **`encoding/json` on invalid JSON** (Go 1.26.0 and 1.27.1, run). `json.Decoder` reports an object
  cut short as `EOF` in 1.26 and `unexpected end of JSON input` in 1.27. In 1.26 it places a syntax
  error's offset before the white space ahead of it: `line 3` for a `}` starting line 4. In 1.27 it
  places it after the character. `json.Unmarshal` gives the same `*json.SyntaxError`, offset
  included, in both, which is how cld reports it. CI's macOS job builds with 1.26 and the Linux
  image with 1.27 ([decision 19](../decisions/0019-project-settings.md)).
- **The record's lock across `exec`** (#114, tmux 3.7c in the test image). Go opens the lock file
  close-on-exec, so the `flock` goes as cld becomes tmux, before tmux has made the session. Two
  `join` of one session then made it twice, which the start mark prevents
  ([decision 40](../decisions/0040-session-record.md),
  [decision 50](../decisions/0050-one-command-join.md)).
- **`kill(pid, 0)` on a starting snap tmux** (#114: the tmux 3.7c snap at revision 95, snapd 2.77.1,
  Linux 7.0.0-31-generic, run). Of 601 calls, 9 failed with `EACCES`, from AppArmor, while
  snap-confine started tmux. So only `ESRCH` means no process
  ([decision 50](../decisions/0050-one-command-join.md)).
- **Where a Go dial reaches a listener on `127.0.0.1` alone** (Go 1.27.1, Ubuntu 26.04 with
  systemd-resolved). `127.0.0.1`, `0.0.0.0`, `::`, `::ffff:127.0.0.1`, `localhost` and
  `foo.localhost` reach it; `::1` and `127.0.0.2` do not
  ([decision 18](../decisions/0018-telemetry.md)).
- **The ports the kernel picks** (Linux 7.0.0-31-generic and Go 1.27.1, natively and in the test
  image). It picks from `ip_local_port_range` alone, odd ports for listeners, and a port let go came
  back 28 times in 200000 natively. `net.Listen` fails with `EADDRINUSE` on a port that a connection
  holds, `TIME_WAIT` included. It fails too where a socket is bound to the port without listening
  and without `SO_REUSEADDR`, which refuses connections and frees the port once closed
  ([decision 18](../decisions/0018-telemetry.md)).
- **The telemetry tests given ports a listener had let go** (#44, on 8 CPUs, in containers and
  natively). cld found such a port in use in 1 of 10 suites and in 1 of 32. Ports above the kernel's
  range, handed out in turn, cut that to 2 failures in 20 runs of a narrowed range
  ([testing.md](../testing.md)).

## Telemetry

Docker 29.6.0 and `otel/opentelemetry-collector` 0.161.0 on Linux, and the JetBrains OpenTelemetry
plugin 2.1.5 in GoLand 2026.2.3, all run; the entries serve
[decision 18](../decisions/0018-telemetry.md).

- **The plugin's receiver**. A separate `satellite.jar` process on a random port of every interface,
  which its settings can fix. It takes OTLP over gRPC only. Not probed: what the plugin's terminal
  customizer puts in a GoLand terminal's environment. It names `OTEL_EXPORTER_OTLP_ENDPOINT`,
  `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_SERVICE_NAME` and `OTEL_METRIC_EXPORT_INTERVAL`. Nor whether
  the `env` of `settings.json` wins over it for a claude started there.
- **The collector's configuration**. `--config=env:CFG` works with `docker run -e CFG`, and a second
  `--config` merges into the first, maps merged and lists replaced. `validate` exits 1 for unknown
  keys or a source that is no YAML, and 0, silent, for a valid or empty one.
- **The exporter type `otlp`**. In 0.161.0 the type aliases `otlp_grpc` and logs a deprecation at
  startup, which `validate` does not show.
- **The collector's own metrics with `--network host`**. Their server takes `localhost:8888`, so a
  second collector exits 1, unless `service.telemetry.metrics.level` is `none`.
- **Startup**. The collector logs `Everything is ready.` within 3 s, once its receivers run. At log
  level `warn` it logs nothing, though it took connections 0.6 s after `docker run`.
- **A second source that moves the receiver**. The collector listens on the new port alone, and cld
  said after 10 s that nothing took connections on its port.
- **A receiver's port taken, under `--restart unless-stopped`**. The collector exits 1, and Docker
  restarts it at once, so it can show `running` with a `RestartCount` of 1.
  `docker update --restart no` leaves it `exited`, though a restart already scheduled may still
  come.
- **The `--local` endpoint**. While that endpoint is down the collector logs about 30 lines in 45 s
  after one span. Pointed at the collector's own receiver, a metric came back without end, at 146%
  CPU.
- **Docker's answers**. `docker rm -f` of a missing container exits 0, and `docker pause` keeps the
  receiver's port, accepting connections it never answers.
- **A variable passed with `docker run -e NAME`** (Linux 7.0, 4 KiB pages). `NAME=VALUE` may be
  131071 bytes, `MAX_ARG_STRLEN` less its NUL; one byte more and `docker` cannot start.
- **DNS on the default bridge**. A container resolves what the host's systemd-resolved does.
- **`cld setup telemetry` end to end**, two debug collectors standing in for the plugin and a team's
  collector. The local one got a span, a metric and a log, the remote one the metric. cld found a
  collector stopped within 2.4 s, and one silent at `warn` ready by its port within 1.5 s.

## JetBrains IDEs and systemd

- **The port of a JetBrains IDE's MCP server** (GoLand 2026.2.3's `mcpserver` plugin, read; GoLand
  2026.2.3 and Rider 2026.2.1 running). The port is 64342 plus an offset per product: GoLand 80 and
  Rider 140, so 64422 and 64482. The settings and the property `idea.mcp.server.force.port` override
  it ([decision 19](../decisions/0019-project-settings.md)).
- **The tools of GoLand's MCP server and of `jbcontext mcp`** (GoLand 2026.2.3 and JetBrains Context
  0.9.15, asked for `tools/list`). GoLand's 44 tools include `execute_terminal_command` and
  `apply_patch`, and 16 have `readOnlyHint`, none `destructiveHint`. `jbcontext mcp` has one,
  `code_search` ([decision 28](../decisions/0028-shared-project-settings.md)).
- **A tmux server that a transient oneshot unit starts** (systemd 259 on Ubuntu 26.04, lingering on;
  the snap's tmux 3.7c, run). With `KillMode=process` the server outlived `systemctl --user stop`,
  run through `/snap/bin` in the snap's own scope or directly in the unit's cgroup. Under the
  default `KillMode` the one run directly went with the unit
  ([decision 48](../decisions/0048-restore-after-reboot.md)).

tmux-resurrect and tmux-continuum, which decision 48 passes over, are in
[tmux-sessions.md](tmux-sessions.md).
