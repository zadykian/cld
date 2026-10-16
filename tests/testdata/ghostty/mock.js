// cld: mock
// A mock Ghostty for cld's scripts, which TestTabsScriptsMocked runs under osascript
// (tests/tabs_scripts_test.go): this file, a script of internal/ghostty/scripts, then mock-end.js.
// They share one function, in which this file's Application, ObjC, $ and delay hide JXA's own:
// UNVERIFIED until the macos job first runs it (decision 59.12).
// The mock reads a model, argv[1], written as tests/internal/mac writes a Model, and logs each
// Apple Event a script sends. Its key mock may hold:
//   fail: 'denied', where macOS denies every Apple Event; or 'off', where Ghostty's config has
//     AppleScript off, so that its getters find no window and its commands fail;
//   versionFails: true, where reading Ghostty's version fails;
//   noTty: true, for Ghostty 1.3.1, whose terminals have no tty;
//   then: windows that replace the model's, one after each first read of the windows' ids, as
//     the user changes them while a script reads them.
var mockRun;
(function () {
  const ghosttyMock = {model: null, log: [], idReads: 0};

  // event logs an Apple Event, which fails where macOS denies them all.
  ghosttyMock.event = function (what) {
    ghosttyMock.log.push(what);
    if (ghosttyMock.model.mock.fail === 'denied') {
      ghosttyMock.fail(-1743, 'Not authorized to send Apple events to Ghostty.');
    }
  };

  // fail throws as JXA does, with Apple's error number.
  ghosttyMock.fail = function (number, message) {
    const e = new Error(message);
    e.errorNumber = number;
    throw e;
  };

  // leaves are the terminal ids of tree n, left or top first.
  ghosttyMock.leaves = function (n) {
    if (!n) {
      return [];
    }
    if (n.pane) {
      return [n.pane];
    }
    return ghosttyMock.leaves(n.first).concat(ghosttyMock.leaves(n.second));
  };

  // windows are the model's windows that Ghostty's getters find: none with AppleScript off.
  ghosttyMock.windows = function () {
    const m = ghosttyMock.model;
    return m.mock.fail === 'off' ? [] : (m.windows || []);
  };

  // terminal is the model's terminal id.
  ghosttyMock.terminal = function (id) {
    return (ghosttyMock.model.terminals || {})[id] || {};
  };

  // property is the getter of a property of every window's tabs' terminals, nested by window and
  // tab, as JXA reads it in one Apple Event.
  ghosttyMock.property = function (name, value) {
    return function () {
      ghosttyMock.event('get windows.tabs.terminals.' + name);
      return ghosttyMock.windows().map((w) => w.tabs.map((tab) =>
        ghosttyMock.leaves(tab.tree).map((id) => value(id, ghosttyMock.terminal(id)))));
    };
  };

  // ids reads every window's tabs' terminal ids. After a first read of an attempt, the windows
  // of mock.then replace the model's.
  ghosttyMock.ids = function () {
    const ids = ghosttyMock.property('id', (id) => id)();
    const then = ghosttyMock.model.mock.then || [];
    ghosttyMock.idReads++;
    if (ghosttyMock.idReads % 2 === 1 && then.length > 0) {
      ghosttyMock.model.windows = then.shift();
    }
    return ids;
  };

  // focused reads each tab's focused terminal in one Apple Event, which fails where a tab has
  // none, as a missing value fails JXA's bulk read.
  ghosttyMock.focused = function () {
    ghosttyMock.event('get windows.tabs.focusedTerminal.id');
    return ghosttyMock.windows().map((w) => w.tabs.map((tab) => {
      if (!tab.focused) {
        ghosttyMock.fail(-1728, "Can't get focused terminal.");
      }
      return tab.focused;
    }));
  };

  // windowsSpec is G.windows: the bulk getters, and each window by its index.
  ghosttyMock.windowsSpec = function () {
    const terminals = {
      id: ghosttyMock.ids,
      name: ghosttyMock.property('name', (id, t) => t.name || ''),
      workingDirectory: ghosttyMock.property('workingDirectory', (id, t) => t.cwd || ''),
      pid: ghosttyMock.property('pid', (id, t) => t.pid || 0),
    };
    if (!ghosttyMock.model.mock.noTty) {
      terminals.tty = ghosttyMock.property('tty', (id, t) => t.tty || '');
    }
    const spec = {
      tabs: {
        terminals: terminals,
        selected: function () {
          ghosttyMock.event('get windows.tabs.selected');
          return ghosttyMock.windows().map((w) => w.tabs.map((tab) => !!tab.selected));
        },
        focusedTerminal: {id: ghosttyMock.focused},
      },
    };
    Object.defineProperty(spec, 'length', {get: function () {
      ghosttyMock.event('count windows');
      return ghosttyMock.windows().length;
    }});
    ghosttyMock.windows().forEach((w, i) => {
      spec[i] = {tabs: w.tabs.map((tab, j) => ({focusedTerminal: {id: function () {
        ghosttyMock.event('get windows[' + i + '].tabs[' + j + '].focusedTerminal.id');
        if (!tab.focused) {
          ghosttyMock.fail(-1728, "Can't get focused terminal.");
        }
        return tab.focused;
      }}}))};
    });
    return spec;
  };

  // terminalsSpec is G.terminals: every terminal of the app, its windows' then those of none.
  ghosttyMock.terminalsSpec = function () {
    const all = function () {
      const ids = [];
      ghosttyMock.windows().forEach((w) => w.tabs.forEach((tab) => {
        ids.push(...ghosttyMock.leaves(tab.tree));
      }));
      return ids.concat(ghosttyMock.model.hidden || []);
    };
    const spec = {id: function () {
      ghosttyMock.event('get terminals.id');
      return all();
    }};
    if (!ghosttyMock.model.mock.noTty) {
      spec.tty = function () {
        ghosttyMock.event('get terminals.tty');
        return all().map((id) => ghosttyMock.terminal(id).tty || '');
      };
    }
    return spec;
  };

  // Application is Ghostty's application object, as JXA gives it by bundle id.
  function Application(bundle) {
    if (bundle !== 'com.mitchellh.ghostty') {
      ghosttyMock.fail(-2700, 'Application can\'t be found.');
    }
    const G = {
      version: function () {
        ghosttyMock.event('get version');
        if (ghosttyMock.model.mock.versionFails) {
          ghosttyMock.fail(-1728, 'Can\'t get version.');
        }
        return ghosttyMock.model.version;
      },
      newSurfaceConfiguration: function () {
        ghosttyMock.event('new surface configuration');
        if (ghosttyMock.model.mock.fail === 'off') {
          ghosttyMock.fail(-1743,
            'AppleScript is disabled by the macos-applescript configuration.');
        }
        return {};
      },
    };
    Object.defineProperty(G, 'windows', {get: ghosttyMock.windowsSpec});
    Object.defineProperty(G, 'terminals', {get: ghosttyMock.terminalsSpec});
    return G;
  }

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
      for (let i = 0; i < Math.max(m.apps || 0, 1); i++) {
        found.push({
          processIdentifier: m.pid,
          launchDate: {isNil: () => !m.launched, timeIntervalSince1970: m.launched},
          executableURL: {isNil: () => !m.executable, path: m.executable},
        });
      }
    }
    return {count: found.length, objectAtIndex: (i) => found[i]};
  }}};

  // delay only logs: the mock keeps no pace.
  function delay(seconds) {
    ghosttyMock.log.push('delay ' + seconds);
  }
