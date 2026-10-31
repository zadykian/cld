// cld: mock
// A mock Ghostty for cld's scripts, which TestTabsScriptsMocked runs under osascript
// (tests/tabs_scripts_test.go): this file, a script of internal/ghostty/scripts, then mock-end.js.
// They share one function, in which this file's Application, ObjC, $ and delay hide JXA's own:
// UNVERIFIED until the macos job first runs it (decision 59.12).
// The mock reads a model, argv[1], written as tests/internal/mac writes a Model, and logs each
// Apple Event a script sends. Its key mock holds the mock's own settings.
var mockRun;
(function () {
  const ghosttyMock = {model: null, log: []};

  // ObjC and $ are the bridge as the scripts use it: NSRunningApplication's Ghostty processes,
  // which a script reads without an Apple Event.
  const ObjC = {
    import: function () {},
    unwrap: function (value) {
      return value;
    },
  };
  const $ = {NSRunningApplication: {runningApplicationsWithBundleIdentifier: function (bundle) {
    const m = ghosttyMock.model;
    const found = [];
    if (m.running && bundle === 'com.mitchellh.ghostty') {
      found.push({
        processIdentifier: m.pid,
        launchDate: {isNil: () => !m.launched, timeIntervalSince1970: m.launched},
        executableURL: {isNil: () => !m.executable, path: m.executable},
      });
    }
    return {count: found.length, objectAtIndex: (i) => found[i]};
  }}};

  // delay only logs: the mock keeps no pace.
  function delay(seconds) {
    ghosttyMock.log.push('delay ' + seconds);
  }
