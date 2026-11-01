// cld: snapshot
// Reads Ghostty's process, then its windows front to back, and prints them as one JSON document
// (internal/ghostty/snapshot.go). Where Ghostty does not run it sends no Apple Event.
// UNVERIFIED until #177's spike (decision 59.12): JXA's spellings, and its bulk reads nested by
// window and tab, pid and tty among them (checks 3, 6); -1743's two steps and errorNumber (4);
// 1.3.1's tty term throwing with no window (3); a bulk focused terminal read failing on a tab that
// has none (6); NSRunningApplication sending no Apple Event and launching nothing (5).
ObjC.import('AppKit');

// read: begin
const BUNDLE = 'com.mitchellh.ghostty';

// apps lists Ghostty's processes through NSRunningApplication, which sends no Apple Event.
function apps() {
  const list = $.NSRunningApplication.runningApplicationsWithBundleIdentifier(BUNDLE);
  const found = [];
  for (let i = 0; i < list.count; i++) {
    const app = list.objectAtIndex(i);
    const date = app.launchDate;
    const url = app.executableURL;
    found.push({
      pid: app.processIdentifier,
      launched: date.isNil() ? 0 : date.timeIntervalSince1970,
      executable: url.isNil() ? '' : ObjC.unwrap(url.path),
    });
  }
  return found;
}

// failure is the error document for step, with JXA's number and Ghostty's text.
function failure(step, e, version) {
  return JSON.stringify({error: {
    step: step,
    number: typeof e.errorNumber === 'number' ? e.errorNumber : 0,
    message: String(e.message === undefined ? e : e.message),
    version: version,
  }});
}

// focusedIds is each tab's focused terminal: in bulk, else tab by tab, '' where one is missing.
function focusedIds(G, ids) {
  try {
    return G.windows.tabs.focusedTerminal.id();
  } catch (e) {
    return ids.map((tabs, i) => tabs.map((_, j) => {
      try {
        return G.windows[i].tabs[j].focusedTerminal.id();
      } catch (missing) {
        return '';
      }
    }));
  }
}

// windows reads every window with bulk getters, an Apple Event each, or is null where the
// windows changed between its first read and its last.
function windows(G) {
  const ids = G.windows.tabs.terminals.id();
  const names = G.windows.tabs.terminals.name();
  const cwds = G.windows.tabs.terminals.workingDirectory();
  const pids = G.windows.tabs.terminals.pid();
  const ttys = G.windows.tabs.terminals.tty();
  const selected = G.windows.tabs.selected();
  const focused = focusedIds(G, ids);
  if (JSON.stringify(G.windows.tabs.terminals.id()) !== JSON.stringify(ids)) {
    return null;
  }
  return ids.map((tabs, i) => ({tabs: tabs.map((terms, j) => ({
    selected: selected[i][j],
    focused: focused[i][j] || '',
    terminals: terms.map((id, k) => ({
      id: id, name: names[i][j][k], cwd: cwds[i][j][k], pid: pids[i][j][k], tty: ttys[i][j][k],
    })),
  }))}));
}

// versionOf is Ghostty's version, '' where the read fails: only the wording uses it, and the
// getter is Cocoa's, not Ghostty's (Ghostty.sdef:15).
function versionOf(G) {
  try {
    return G.version();
  } catch (e) {
    return '';
  }
}

// snapshot reads Ghostty, found running: the getter whose -1743 is macOS denying Automation, the
// probe whose -1743 is macos-applescript off, the version, then 1.4's tty term, then the windows.
function snapshot(G, found) {
  let step = 'count';
  let version = '';
  try {
    G.windows.length;
    step = 'probe';
    G.newSurfaceConfiguration();
    version = versionOf(G);
    step = 'tty';
    G.terminals.tty();
    step = 'read';
    for (let attempt = 0; attempt < 2; attempt++) {
      const read = windows(G);
      if (read !== null) {
        return JSON.stringify({running: true, apps: found, version: version,
          terminals: G.terminals.id(), windows: read});
      }
    }
    step = 'changed';
    throw new Error('the windows changed while cld read them');
  } catch (e) {
    return failure(step, e, version);
  }
}
// read: end

function run(argv) {
  const found = apps();
  if (found.length === 0) {
    return JSON.stringify({running: false, apps: []});
  }
  return snapshot(Application(BUNDLE), found);
}
