// cld: apply
// Types into the panes Ghostty kept, then prints what it did as one JSON document
// (internal/ghostty/apply.go). Where Ghostty does not run, or the Ghostty it began with has gone,
// it sends no more Apple Events.
// UNVERIFIED until #177's check 11 (decision 59.12): pid read again just before typing; Ctrl+U,
// input text and Enter in zsh with half-typed input and no paste prompt; Ctrl+U under a layout
// that moves U, Dvorak's say, as send key names a key by its place.
ObjC.import('AppKit');

const BUNDLE = 'com.mitchellh.ghostty';
// GONE is thrown where the Ghostty the apply began with has gone: an Apple Event would launch it.
const GONE = new Error('Ghostty is not running');
let G = null;
let started = 0;
let known = [];

// ghosttyPid is the pid of Ghostty's process, through NSRunningApplication, which sends no Apple
// Event: 0 where none runs.
function ghosttyPid() {
  const list = $.NSRunningApplication.runningApplicationsWithBundleIdentifier(BUNDLE);
  return list.count > 0 ? list.objectAtIndex(0).processIdentifier : 0;
}

// gone reports whether the Ghostty the apply began with has quit, or another runs in its place.
function gone() {
  return ghosttyPid() !== started;
}

// alive throws GONE where gone holds, before an Apple Event that would launch Ghostty again.
function alive() {
  if (gone()) throw GONE;
}

// message is a typing's error: JXA's text, then its number in brackets.
function message(e) {
  const text = String(e.message === undefined ? e : e.message);
  return typeof e.errorNumber === 'number' ? text + ' (' + e.errorNumber + ')' : text;
}

// failure is the error document for step, with JXA's number and text apart, as snapshot.js
// prints it.
function failure(step, e) {
  return JSON.stringify({error: {
    step: step,
    number: typeof e.errorNumber === 'number' ? e.errorNumber : 0,
    message: String(e.message === undefined ? e : e.message),
    version: '',
  }});
}

// type empties the prompt's line, pastes line, and presses Enter: input text is a paste, which a
// prompt in bracketed-paste mode does not run. It types nothing, and is false, where another
// group than the snapshot's shell is in front: the user started a program since (decision 59.7).
function type(typing) {
  alive();
  if (G.terminals.byId(typing.pane).pid() !== typing.pid) return false;
  G.sendKey('u', {modifiers: 'control', to: G.terminals.byId(typing.pane)});
  G.inputText(typing.line, {to: G.terminals.byId(typing.pane)});
  G.sendKey('enter', {to: G.terminals.byId(typing.pane)});
  return true;
}

// attempt runs f, one typing, and is its entry of the result, or caught's entry for its error. It
// is null where Ghostty has gone, before f or during it, which ends the apply.
function attempt(f, caught) {
  if (gone()) return null;
  try {
    return f();
  } catch (e) {
    return e === GONE || gone() ? null : caught(e);
  }
}

function run(argv) {
  const plan = JSON.parse(argv[0]);
  started = ghosttyPid();
  if (started === 0) {
    return failure('running', GONE);
  }
  G = Application(BUNDLE);
  try {
    known = G.terminals.id();
  } catch (e) {
    return failure('count', e);
  }
  const out = {typed: [], builds: []};
  for (const typing of plan.typed) {
    const busy = {pane: typing.pane, busy: true};
    const entry = attempt(() => type(typing) ? {pane: typing.pane} : busy,
      e => ({pane: typing.pane, error: message(e)}));
    if (entry === null) return failure('running', GONE);
    out.typed.push(entry);
  }
  return JSON.stringify(out);
}
