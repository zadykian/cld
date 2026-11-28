# 68. Joining again after a drop

Status: Accepted (#241).

## Context

`cld join --host HOST` became ssh ([decision 62.3](0062-commands-on-another-host.md)). A drop, as
the Mac sleeps, ended ssh and left the pane at the Mac's prompt, while the session ran on. Decisions
[62.7](0062-commands-on-another-host.md) and [59.10](0059-ghostty-tabs.md) left drops to ws's
`ws-join`, a script on the Mac (`zadykian/ws` #56). ws drops that script for this decision.

- ssh ends with 255 on a drop, and on a connection or a login that fails. Its log, in the file
  that `-E` names, tells them apart
  ([environment findings](../findings/environment.md#ssh)).
- Each failed login counts towards a fail2ban's ban, which shuts the Mac out of the host
  ([guide](../../guide/ghostty-tabs.md#sshd-and-fail2ban)).
- A drop leaves on the modes the session's programs set, such as the mouse's reports: the host's
  tmux can no longer turn them off. The Mac's shell then reads a click as escape sequences. ws's
  zsh hook turns them off at the prompt (`zadykian/ws` #793).

This amends decisions [62](0062-commands-on-another-host.md), in 62.3 and 62.7, and
[59](0059-ghostty-tabs.md), in 59.5 and 59.10.

## Decision

`join --host` on a terminal runs ssh as cld's child, and joins the session again after a drop.

### 68.1 ssh as cld's child

Where stdin and stdout are terminals, as 62.3 gives ssh `-t`, `join --host` runs ssh as its child.
Other commands, and a `join` without a terminal, still exec ssh. ssh takes two options more:

- `-o ConnectTimeout=10`: a try at a host that does not answer ends in 10 s, not TCP's own minutes.
- `-E LOG`, a temporary file: cld reads ssh's errors there. It prints LOG to stderr after each
  try, and removes it as it ends.

cld passes SIGINT, SIGTERM and SIGHUP on to ssh. It ends with ssh's status, or 128 plus the
number of a signal it caught, through `internal/fail`.

### 68.2 The try

On 255, cld notes the drop and waits 5 s. It flushes what the user typed meanwhile, which ssh would
send to the host, where claude takes it as a prompt. Then it runs `join --existing` on HOST, with
`-C`, `-n` and `-s` as given, and so on until another status. Any status but 255 is the host's
cld's, such as 0 after a detach.

- `--existing` never creates a session ([decision 58](0058-joining-only-a-session-that-exists.md)).
  After a drop the session runs on, and after the host's reboot it resumes.
- Where LOG says ssh never connected, `ssh: connect to host` or `Could not resolve hostname`, the
  host's cld has seen no join, and `--existing` would refuse the session it would make. The try
  runs the first words again, with or without `-s`, until ssh connects.
- `-w`, `--new`, `--resume`, `--fork` and ARGS went to the first join, and `--existing` excludes
  them. `--detach-others` is left out too: a try runs unattended, and would detach a terminal the
  user joined from elsewhere since the drop.
- Without `-s`, no try follows, and cld notes to join again with `-s SUFFIX`. The host's cld chose
  INDEX, and nothing past tmux's output comes back to cld here.
- Ctrl-C in the wait, or the pane's closing, ends the tries. ssh's `-t` keeps Ctrl-C for the host
  while it runs.

### 68.3 A refusal ends the tries

Where LOG has ssh's last line, `USER@HOST: Permission denied (METHODS).` or `Host key
verification failed.`, no try follows. Each would fail the same way, and count towards a
fail2ban's ban. `Permission denied, please try again.` after a mistyped password, and `Load key …:
Permission denied`, are no refusal: the login goes on. A probe against a live sshd gave the first
line for a key it did not know. A timeout or a refused connection is no login, so the tries go on.

### 68.4 The terminal after a drop

After a 255, before LOG, cld turns off the modes the session may have left on:

- the reports of the mouse, focus and the colour scheme;
- modifyOtherKeys and kitty's keyboard flags;
- the cursor and keypad keys' application modes;
- the cursor's style and visibility.

Then it leaves the alternate screen, and turns them off again, as kitty's flags are each screen's
own. `ESC 7` comes first: leaving restores the cursor saved on entering, even where the main screen
is on. That is ws's zsh hook's sequence (`zadykian/ws` #793), so LOG and the next try show on the
main screen.

### 68.5 Ghostty's tabs

`cld join` in front, with an ssh child, makes the pane that ssh's, as `ghostty +ssh` does (59.5).
Between the tries, no ssh runs, and the pane keeps what it showed. Any other cld in front keeps it
too. `ws-join` is no wrapper any more.

A restore's typed line stays `ssh -t HOST … cld join --existing`
([decision 59.7](0059-ghostty-tabs.md)). A detach there leaves the host's shell, where the user had
typed `ssh HOST`. So a drop in a restored pane still leaves the Mac's prompt, which 59.10 defers.

### 68.6 Not taken

- `cld join --host` as the restore's typed line: drops would join again, but a detach would leave
  the Mac's prompt, not the host's shell.
- `ws-join`: a script on each Mac that repeats cld's words, which the watcher must also know.
- A limit on the tries: a Mac asleep for a night would use them up. Ctrl-C or a closed pane ends
  them.
- Tries without `-s`, from cld's title: the title reaches the terminal, not cld.

## Consequences

- From a Mac, `cld join --host ws -n cld -s 3` comes back by itself after a sleep. The
  [guide](../../guide/another-host.md#sshs-connections) says so, and `join`'s help.
- `internal/remote` runs the tries, and `cmd/cld` gives their words. `join --existing` keeps its
  meaning in every later release, as another machine's cld sends it (decision 58).
- The tests give the fake ssh a status per call, and its log in `-E`'s file
  ([testing](../testing/harness.md#the-macs-fakes)). They cover a drop then a detach, the
  refusals, no `-s`, and a signal in the wait.
