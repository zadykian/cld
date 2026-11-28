# Findings: claude's files

Where Claude Code keeps its files on disk, which cld removes once no claude needs them, probed on
Linux. "Read" means read in claude's JavaScript bundle, not run; "run" means the real claude ran.
The rest of claude's findings are in [claude](claude.md).

## The temp root

- **Where claude keeps its temp files** (#240: 2.1.332, read and run). The temp root is
  `claude-UID` in `CLAUDE_CODE_TMPDIR`, or else in Node's `os.tmpdir()`. That takes `TMPDIR`,
  `TMP` or `TEMP`, by default `/tmp`, and an empty variable counts as unset. A directory for each
  project, its path with each `/` as `-`, holds a directory for each conversation, named by its ID.
  That holds `scratchpad`, `tasks` and the like
  ([decision 67](../decisions/0067-claudes-temp-directories.md)).
- **What a running claude holds there** (#240: 2.1.332, run). Each of five running claudes held
  its conversation's `tasks` directory open, as `ls -l /proc/PID/fd` showed. On one host, six ended
  sessions' directories, 0.8 to 2.5 GB each, had no process with a file open under them
  ([decision 67](../decisions/0067-claudes-temp-directories.md)).
- **Where the transcript is** (2.1.332, run). `projects/PROJECT/ID.jsonl` in claude's directory,
  `CLAUDE_CONFIG_DIR`, by default `~/.claude`, `PROJECT` named as in the temp root. claude writes
  it with each turn ([decision 67](../decisions/0067-claudes-temp-directories.md)).
