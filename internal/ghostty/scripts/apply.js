// cld: apply
// Types into the panes Ghostty kept, then prints what it did as one JSON document
// (internal/ghostty/apply.go).
// UNVERIFIED until #177's check 11 (decision 59.12): pid read again just before typing; Ctrl+U,
// input text and Enter in zsh with half-typed input and no paste prompt; Ctrl+U under a layout
// that moves U, Dvorak's say, as send key names a key by its place.
const BUNDLE = 'com.mitchellh.ghostty';
let G = null;
let known = [];

// message is a typing's error: JXA's text, then its number in brackets.
function message(e) {
  const text = String(e.message === undefined ? e : e.message);
  return typeof e.errorNumber === 'number' ? text + ' (' + e.errorNumber + ')' : text;
}

// type empties the prompt's line, pastes line, and presses Enter: input text is a paste, which a
// prompt in bracketed-paste mode does not run. It types nothing, and is false, where another
// group than the snapshot's shell is in front: the user started a program since (decision 59.7).
function type(typing) {
  if (G.terminals.byId(typing.pane).pid() !== typing.pid) return false;
  G.sendKey('u', {modifiers: 'control', to: G.terminals.byId(typing.pane)});
  G.inputText(typing.line, {to: G.terminals.byId(typing.pane)});
  G.sendKey('enter', {to: G.terminals.byId(typing.pane)});
  return true;
}

function run(argv) {
  const plan = JSON.parse(argv[0]);
  G = Application(BUNDLE);
  known = G.terminals.id();
  const out = {typed: [], builds: []};
  for (const typing of plan.typed) {
    try {
      out.typed.push(type(typing) ? {pane: typing.pane} : {pane: typing.pane, busy: true});
    } catch (e) {
      out.typed.push({pane: typing.pane, error: message(e)});
    }
  }
  return JSON.stringify(out);
}
