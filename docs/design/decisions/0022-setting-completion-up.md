# 22. Setting completion up

Status: Accepted. Amended by [27](0027-completion-with-blesh.md).

## Context

Completion took lines set up by hand for each shell, and nothing kept the scripts in step with
cobra's templates. `setup completion SHELL` now writes the script where the shell reads it, and
`update` writes it anew, amending [decision 21](0021-self-update.md).

Probed with bash-completion 2.16, zsh 5.9 and fish 4.0.2 on Debian trixie
([environment findings](../findings/environment.md)):

- bash-completion 2 reads a user directory before the system's;
- fish reads `~/.config/fish/completions` first, hiding its other directories, and makes
  `~/.config/fish` only at its first interactive start;
- zsh reads no directory of the user's, and a second `compinit` drops what was set up after the
  first; Ubuntu runs one for every user, Debian none;
- `source <(cld completion zsh)` needs an earlier `compinit`, and cld on the `PATH` as zsh starts.

## Decision

### 22.1 A command per shell

A command per shell gives each its help, and `help` and completion take them as the other commands
of `setup` ([decision 12.2](0012-help-from-cobra.md), [17.3](0017-shell-completion.md)). The shell
is checked before cobra ([decision 18.8](0018-telemetry.md)). The script is byte for byte what
`completion SHELL` prints.

### 22.2 Where each script goes

bash's goes to the user directory of bash-completion 2. fish's goes to its first directory, where
cobra's help put it. fish's vendor directory would lose to a script there. zsh's goes to a data
directory of cld's, which lines in `.zshrc` load.

### 22.3 The lines in `.zshrc`

The lines put that directory first on `$fpath` and register the script. They run `compinit -i` only
where nothing has, since a second one drops what the first set up. `-i` skips insecure directories,
such as Homebrew's, instead of asking. A `compinit` after them still finds the script on
`$fpath`, by its `#compdef` line. The lines do nothing without the script, and run no cld. They
name the directory by zsh's variables, to hold on every machine, and go in once.

### 22.4 Writing the files

cld writes the files as `setup project` writes its own ([decision 19.4](0019-project-settings.md)),
reading both before it writes either. It removes nothing, and edits no `~/.bashrc`: loading
bash-completion there is Debian's and Ubuntu's default, and on macOS the user's choice.

### 22.5 `update` writes them anew

Once cld is replaced ([decision 21.3](0021-self-update.md)), the new release prints each script,
which its cobra may have changed, and cld writes those that differ. Every release has
`completion SHELL`, and running `setup completion` instead would bring back lines a user removed. A
file that does not start as cobra's script does is left alone, and one without descriptions is
printed anew without them. A failure to write is a warning, as the maintainer asked. The refresh
runs after update's handling of signals ([decision 21.4](0021-self-update.md)). A signal then ends
cld as it ends other commands, and can leave configfile's temporary file beside the script.

### 22.6 Cobra's last line of help

The help ends with cobra's own line, 81 columns long, which the test of the help's width skips: it
holds no text of cld's ([decision 12.1](0012-help-from-cobra.md)).

### 22.7 Tests

The tests write the scripts in the sandbox's home and load them in bash, zsh and fish
([testing](../testing.md)).

## Consequences

Out of scope: PowerShell, whose script `completion powershell` still prints; bash-completion 1's
directory, shared in Homebrew's prefix; checking `~/.bashrc`; removing what cld wrote; `install.sh`
writing the scripts.
