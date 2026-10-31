// cld: snapshot
// Reads Ghostty's process, then its windows front to back, and prints them as one JSON document
// (internal/ghostty/snapshot.go). Where Ghostty does not run it sends no Apple Event.
// UNVERIFIED until #177's spike (decision 59.12): JXA's spellings, -1743's two steps and
// errorNumber (4), NSRunningApplication sending no Apple Event and launching nothing (5).
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
// probe whose -1743 is macos-applescript off, then the version.
function snapshot(G, found) {
  let step = 'count';
  let version = '';
  try {
    G.windows.length;
    step = 'probe';
    G.newSurfaceConfiguration();
    version = versionOf(G);
    return JSON.stringify({running: true, apps: found, version: version});
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
