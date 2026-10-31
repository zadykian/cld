// cld: apply
// Types into the panes Ghostty kept and builds the windows of cld's plan, then prints what it made
// as one JSON document (internal/ghostty/apply.go). Between commands it holds terminal ids alone.
// Where Ghostty does not run, or the Ghostty it began with has gone, it sends no more Apple
// Events.
// UNVERIFIED until #177's check 11 (decision 59.12): a JS object as a command's surface
// configuration, initialWorkingDirectory among its keys; a new window's terminal among the
// terminals at once; pid read again just before typing; Ctrl+U, input text and Enter in zsh with
// half-typed input and no paste prompt; Ctrl+U under a layout that moves U, Dvorak's say, as send
// key names a key by its place; initial input run once under a slow .zshrc.
ObjC.import('AppKit');

const BUNDLE = 'com.mitchellh.ghostty';
// GONE is thrown where the Ghostty the apply began with has gone: an Apple Event would launch it.
const GONE = new Error('Ghostty is not running');
let G = null;
let started = 0;
let pace = null;
let paced = 0;
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

// step waits before each line that starts an ssh: pace.gap apart, and pace.pause more after
// every pace.every, so that sshd's MaxStartups and a fail2ban see no burst (decision 59.7).
function step() {
  if (paced > 0) {
    delay(paced % pace.every === 0 ? pace.gap + pace.pause : pace.gap);
  }
  paced++;
}

// message is a typing's or a build's error: JXA's text, then its number in brackets.
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

// config is leaf's surface configuration: its line as initial input, never a command, and its
// directory (decision 59.7).
function config(leaf) {
  const c = {};
  if (leaf.line) c.initialInput = leaf.line + '\n';
  if (leaf.cwd) c.initialWorkingDirectory = leaf.cwd;
  return c;
}

function firstLeaf(node) {
  return node.leaf ? node.leaf : firstLeaf(node.first);
}

// created is the one terminal id the last new window added to terminals, waited for up to 3 s:
// that command returns a window, not the terminal.
function created() {
  for (let i = 0; i < 30; i++) {
    alive();
    const now = G.terminals.id();
    const added = now.filter(id => !known.includes(id));
    if (added.length > 1) throw new Error('more than one terminal appeared');
    if (added.length === 1) return added[0];
    delay(0.1);
  }
  throw new Error('no terminal appeared');
}

// open runs create, a new window, with leaf's configuration and returns the terminal it made. It
// reads the terminals just before, once the pace has passed, so that one the user opened
// meanwhile counts as no new one.
function open(leaf, create) {
  if (leaf.line) step();
  alive();
  known = G.terminals.id();
  create(config(leaf));
  return created();
}

// buildWindow opens a window of b's first tab.
function buildWindow(b, keys) {
  const leaf = firstLeaf(b.tabs[0].tree);
  keys[leaf.key] = open(leaf, c => G.newWindow({withConfiguration: c}));
  return {keys: keys};
}

// type empties the prompt's line, pastes line, and presses Enter: input text is a paste, which a
// prompt in bracketed-paste mode does not run. Once the pace has passed, it types nothing, and is
// false, where another group than the snapshot's shell is in front: the user started a program
// since (decision 59.7).
function type(typing) {
  step();
  alive();
  if (G.terminals.byId(typing.pane).pid() !== typing.pid) return false;
  G.sendKey('u', {modifiers: 'control', to: G.terminals.byId(typing.pane)});
  G.inputText(typing.line, {to: G.terminals.byId(typing.pane)});
  G.sendKey('enter', {to: G.terminals.byId(typing.pane)});
  return true;
}

// attempt runs f, one typing or build, and is its entry of the result, or caught's entry for its
// error. It is null where Ghostty has gone, before f or during it, which ends the apply.
function attempt(f, caught) {
  if (gone()) return null;
  try {
    return f();
  } catch (e) {
    return e === GONE || gone() ? null : caught(e);
  }
}

const BUILDERS = {window: buildWindow};

function run(argv) {
  const plan = JSON.parse(argv[0]);
  pace = plan.pace;
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
  for (const b of plan.builds) {
    const keys = {};
    const entry = attempt(() => BUILDERS[b.kind](b, keys), e => ({keys: keys, error: message(e)}));
    if (entry === null) return failure('running', GONE);
    out.builds.push(entry);
  }
  return JSON.stringify(out);
}
