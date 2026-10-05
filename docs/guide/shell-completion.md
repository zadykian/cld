# Shell completion

- `-s` describes each session by its state and claude's status, or as `ended in DIR`. Without
  `-n`, it offers only the sessions that `cld join` takes from here.
- Nothing offers file names, and `cld kill` and `--resume` offer nothing.
- Each TAB runs `cld`, so the names are current. `CLD_COMPLETION_DESCRIPTIONS=0` drops the
  descriptions.
- `cld update` writes the scripts anew, and one written with `--no-descriptions` stays so. Where it
  cannot, it warns, naming the command to run. The install command leaves the scripts as they are:
  run `cld setup completion` after it.
- To undo the setup, delete the script, and for zsh its lines in `.zshrc`.

## bash

- Without bash-completion 2, which Debian's and Ubuntu's `~/.bashrc` load, every TAB prints
  `_get_comp_words_by_ref: command not found`. With Homebrew's bash, install `bash-completion@2`.
- Under [ble.sh](https://github.com/akinomyoga/ble.sh) 0.4, `-n=NAME`, `--name=NAME` and
  `--mcp=SERVER` complete nothing, as ble.sh drops what follows the `=`: write `-n NAME`. ble.sh
  0.3 offers file names where cld offers none.
- macOS's bash 3.2 takes Homebrew's `bash-completion` 1.3, which reads no such directory. Install it
  as its caveats say, and write the script by hand, which `cld update` leaves alone:

  ```sh
  cld completion bash > "$(brew --prefix)/etc/bash_completion.d/cld"
  ```

  There no space follows a completed name, and file names come where cld offers none.

## zsh

`cld setup completion zsh` adds these lines to `.zshrc`:

```zsh
# cld's completion, from cld setup completion zsh
if [[ -r ${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh/_cld ]]; then
  fpath=("${XDG_DATA_HOME:-$HOME/.local/share}/cld/zsh" $fpath)
  (( $+functions[compdef] )) || { autoload -U compinit && compinit -i; }
  autoload -Uz _cld && compdef _cld cld
fi
```

`compinit -i` leaves out directories that other users can write to, rather than ask. Where
home-manager or a dotfiles manager writes `.zshrc`, add the lines there.

## fish

cld replaces a script you wrote by hand in `~/.config/fish/completions`.
