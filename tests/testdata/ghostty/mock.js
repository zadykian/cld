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

  // event logs an Apple Event.
  ghosttyMock.event = function (what) {
    ghosttyMock.log.push(what);
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

  // windows are the model's windows that Ghostty's getters find.
  ghosttyMock.windows = function () {
    return ghosttyMock.model.windows || [];
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
      id: ghosttyMock.property('id', (id) => id),
      name: ghosttyMock.property('name', (id, t) => t.name || ''),
      workingDirectory: ghosttyMock.property('workingDirectory', (id, t) => t.cwd || ''),
      pid: ghosttyMock.property('pid', (id, t) => t.pid || 0),
      tty: ghosttyMock.property('tty', (id, t) => t.tty || ''),
    };
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
    return {
      id: function () {
        ghosttyMock.event('get terminals.id');
        return all();
      },
      tty: function () {
        ghosttyMock.event('get terminals.tty');
        return all().map((id) => ghosttyMock.terminal(id).tty || '');
      },
    };
  };

  // Application is Ghostty's application object, as JXA gives it by bundle id.
  function Application(bundle) {
    if (bundle !== 'com.mitchellh.ghostty') {
      ghosttyMock.fail(-2700, 'Application can\'t be found.');
    }
    const G = {
      version: function () {
        ghosttyMock.event('get version');
        return ghosttyMock.model.version;
      },
      newSurfaceConfiguration: function () {
        ghosttyMock.event('new surface configuration');
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
