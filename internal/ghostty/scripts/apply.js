// cld: apply
// Types into the panes Ghostty kept and builds the windows, tabs and splits of cld's plan, then
// prints what it made as one JSON document (internal/ghostty/apply.go). Between commands it holds
// terminal ids alone: a window's id can change, and a tab's goes through its window's. Where
// Ghostty does not run, or the Ghostty it began with has gone, it sends no more Apple Events.
// UNVERIFIED until #177's check 11 (decision 59.12): a JS object as a command's surface
// configuration, initialWorkingDirectory among its keys; a new window's or tab's terminal among
// the terminals at once, and its window among the windows within 3 s (check 7 too); split's
// result answering id(); new tabs in the order made, the index read back; select tab and focus;
// pid read again just before typing; Ctrl+U, input text and Enter in zsh with half-typed input and
// no paste prompt; Ctrl+U under a layout that moves U, Dvorak's say, as send key names a key by its
// place; initial input run once under a slow .zshrc.
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

// step waits before each line that starts an ssh: pace.gap apart, and pace.pause more after
// every pace.every, so that sshd's MaxStartups and a fail2ban see no burst (decision 59.7).
function step() {
  if (paced > 0) {
    delay(paced % pace.every === 0 ? pace.gap + pace.pause : pace.gap);
  }
  paced++;
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

function leaves(node) {
  return node.leaf ? [node.leaf] : leaves(node.first).concat(leaves(node.second));
}

// created is the one terminal id the last new window or new tab added to terminals, waited for
// up to 3 s: those commands return a window or a tab, not the terminal.
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

// windowWith is the index in windows of the window holding terminal id, and that window's
// terminal ids, waited for up to 3 s: a new window joins windows once AppKit orders it
// (AppDelegate+AppleScript.swift's new window).
function windowWith(id) {
  for (let i = 0; i < 30; i++) {
    alive();
    const all = G.windows.terminals.id();
    const w = all.findIndex(ids => ids.includes(id));
    if (w >= 0) return {index: w, ids: all[w]};
    delay(0.1);
  }
  throw new Error('no window holds terminal ' + id);
}

function windowOf(id) {
  return windowWith(id).index;
}

function tabOf(w, id) {
  return G.windows[w].tabs.terminals.id().findIndex(ids => ids.includes(id));
}

// open runs create, a new window or a new tab, with leaf's configuration and returns the
// terminal it made. It reads the terminals just before, once the pace has passed, so that one the
// user opened meanwhile counts as no new one.
function open(leaf, create) {
  if (leaf.line) step();
  alive();
  known = G.terminals.id();
  create(config(leaf));
  return created();
}

// splitAt splits terminal id towards direction with leaf's configuration, and returns the id of
// the terminal split returns. For right and down, id keeps the first half.
function splitAt(id, direction, leaf) {
  if (leaf.line) step();
  alive();
  const c = config(leaf);
  return G.split(G.terminals.byId(id), {direction: direction, withConfiguration: c}).id();
}

// grow builds node at terminal id: a split makes the second child's first terminal, and each
// child grows from its own. Each key goes in as its terminal is made, so a split that fails still
// reports every terminal made before it.
function grow(node, id, keys) {
  keys[firstLeaf(node).key] = id;
  if (node.leaf) {
    return;
  }
  const made = splitAt(id, node.split, firstLeaf(node.second));
  keys[firstLeaf(node.second).key] = made;
  grow(node.first, id, keys);
  grow(node.second, made, keys);
}

// settle evens a tab's splits (decision 59.8) and focuses its focused terminal.
function settle(tab, keys) {
  const ids = leaves(tab.tree).map(leaf => keys[leaf.key]);
  if (ids.length > 1) G.performAction('equalize_splits', {on: G.terminals.byId(ids[0])});
  G.focus(G.terminals.byId(keys[tab.focused] || ids[0]));
}

// choose selects tab in its window and focuses its focused terminal.
function choose(tab, keys) {
  const id = keys[firstLeaf(tab.tree).key];
  const w = windowOf(id);
  G.selectTab(G.windows[w].tabs[tabOf(w, id)]);
  G.focus(G.terminals.byId(keys[tab.focused] || id));
}

// buildWindow opens a window of b's tabs, each after the one before, as a new tab goes after its
// window's selected one, and selects b's selected tab.
function buildWindow(b, keys) {
  let first = '';
  b.tabs.forEach((tab, n) => {
    const id = open(firstLeaf(tab.tree), c => n === 0
      ? G.newWindow({withConfiguration: c})
      : G.newTab({in: G.windows[windowOf(first)], withConfiguration: c}));
    if (n === 0) first = id;
    grow(tab.tree, id, keys);
    settle(tab, keys);
  });
  choose(b.tabs.find(tab => tab.selected) || b.tabs[0], keys);
  return {keys: keys};
}

// buildTab opens a lost tab in the window holding its anchor, found once the pace has passed, or
// as a window, and reports the index it took. Its key goes in at once, so a failure reports it. A
// tab that lands beside other terminals than its anchor's fails, once built.
function buildTab(b, keys) {
  const tab = b.tabs[0];
  const id = open(firstLeaf(tab.tree), c => {
    const home = b.anchor ? G.windows.terminals.id().findIndex(ids => ids.includes(b.anchor)) : -1;
    return home >= 0
      ? G.newTab({in: G.windows[home], withConfiguration: c})
      : G.newWindow({withConfiguration: c});
  });
  keys[firstLeaf(tab.tree).key] = id;
  const found = windowWith(id);
  const beside = found.ids;
  const result = {keys: keys};
  if (b.anchor && beside.includes(b.anchor)) {
    result.index = tabOf(found.index, id) + 1;
  } else if (beside.length === 1) {
    result.window = true;
  } else {
    result.error = 'it opened in another window than the pane that was beside it';
  }
  grow(tab.tree, id, keys);
  settle(tab, keys);
  return result;
}

// buildPane splits b's anchor for a lost split's tree, the new terminal taking the side b names.
function buildPane(b, keys) {
  const tab = b.tabs[0];
  const id = splitAt(b.anchor, b.split, firstLeaf(tab.tree));
  grow(tab.tree, id, keys);
  G.focus(G.terminals.byId(keys[tab.focused] || id));
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

const BUILDERS = {window: buildWindow, tab: buildTab, pane: buildPane};

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
