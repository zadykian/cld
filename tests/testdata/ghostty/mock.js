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
//     the user changes them while a script reads them;
//   opens: the starts of log lines, an event's or a delay's, after the first of which the user
//     opens a window, each in turn; raises: likewise, the user bringing the back window to the
//     front; quits: the start of a log line after which Ghostty quits.
// Its commands keep Ghostty's rules as v1.4.0's source has them
// (docs/design/findings/terminals.md): a new window joins the windows with its terminal; a new tab
// goes after its window's selected tab, selected, its terminal focused at once; focus moves focus
// at the next Apple Event; send key and input text record what they type in the terminal. The model's failAt is the terminal,
// counted from 1, that Ghostty fails to make: a new window's or tab's never appears. delay only logs.
var mockRun;
(function () {
  const ghosttyMock = {model: null, log: [], idReads: 0, queue: [], made: 0};

  // event logs an Apple Event, which fails where macOS denies them all. A split's or a focus's
  // move of focus lands first, as Ghostty's main queue runs it before the next event.
  ghosttyMock.event = function (what) {
    ghosttyMock.settle();
    ghosttyMock.log.push(what);
    if (ghosttyMock.model.mock.fail === 'denied') {
      ghosttyMock.fail(-1743, 'Not authorized to send Apple events to Ghostty.');
    }
    ghosttyMock.user(what);
  };

  // user acts after the log line what, as mock.opens, mock.raises and mock.quits say.
  ghosttyMock.user = function (what) {
    const m = ghosttyMock.model;
    if (ghosttyMock.next(m.mock.opens, what)) {
      m.windows.unshift(ghosttyMock.newWindow(ghosttyMock.add({})));
    }
    if (ghosttyMock.next(m.mock.raises, what) && m.windows.length > 1) {
      m.windows.unshift(m.windows.pop());
    }
    if (m.mock.quits && what.startsWith(m.mock.quits)) {
      m.running = false;
    }
  };

  // next reports whether what starts with the first of list, which it then takes off.
  ghosttyMock.next = function (list, what) {
    if (!list || list.length === 0 || !what.startsWith(list[0])) {
      return false;
    }
    list.shift();
    return true;
  };

  // settle runs the moves of focus queued so far.
  ghosttyMock.settle = function () {
    const queued = ghosttyMock.queue;
    ghosttyMock.queue = [];
    queued.forEach((f) => f());
  };

  // command fails where Ghostty's config has AppleScript off, as each command does then.
  ghosttyMock.command = function () {
    if (ghosttyMock.model.mock.fail === 'off') {
      ghosttyMock.fail(-1743, 'AppleScript is disabled by the macos-applescript configuration.');
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

  // read is a read of the windows, what: an Apple Event, whose answer value gives.
  ghosttyMock.read = function (what, value) {
    ghosttyMock.event(what);
    return value();
  };

  // terminal is the model's terminal id.
  ghosttyMock.terminal = function (id) {
    return (ghosttyMock.model.terminals || {})[id] || {};
  };

  // locate is the window and tab of the model that hold terminal id, or null where Ghostty has
  // none.
  ghosttyMock.locate = function (id) {
    for (const window of ghosttyMock.windows()) {
      for (const tab of window.tabs) {
        if (ghosttyMock.leaves(tab.tree).includes(id)) {
          return {window: window, tab: tab};
        }
      }
    }
    return null;
  };

  // found is where Ghostty finds terminal id by its id, which fails where it has none.
  ghosttyMock.found = function (id) {
    const at = ghosttyMock.locate(id);
    if (!at) {
      ghosttyMock.fail(-1728, 'Can\'t get object.');
    }
    return at;
  };

  // front selects tab in window, and moves window to the front.
  ghosttyMock.front = function (window, tab) {
    window.tabs.forEach((t) => {
      t.selected = t === tab;
    });
    const m = ghosttyMock.model;
    m.windows = [window].concat(m.windows.filter((w) => w !== window));
  };

  // property is the getter of a property of every window's tabs' terminals, nested by window and
  // tab, as JXA reads it in one Apple Event.
  ghosttyMock.property = function (name, value) {
    return function () {
      return ghosttyMock.read('get windows.tabs.terminals.' + name, () =>
        ghosttyMock.windows().map((w) => w.tabs.map((tab) =>
          ghosttyMock.leaves(tab.tree).map((id) => value(id, ghosttyMock.terminal(id))))));
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
    return ghosttyMock.read('get windows.tabs.focusedTerminal.id', () =>
      ghosttyMock.windows().map((w) => w.tabs.map((tab) => {
        if (!tab.focused) {
          ghosttyMock.fail(-1728, "Can't get focused terminal.");
        }
        return tab.focused;
      })));
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
          return ghosttyMock.read('get windows.tabs.selected', () =>
            ghosttyMock.windows().map((w) => w.tabs.map((tab) => !!tab.selected)));
        },
        focusedTerminal: {id: ghosttyMock.focused},
      },
      terminals: {id: function () {
        return ghosttyMock.read('get windows.terminals.id', () => ghosttyMock.windows().map((w) =>
          [].concat(...w.tabs.map((tab) => ghosttyMock.leaves(tab.tree)))));
      }},
    };
    Object.defineProperty(spec, 'length', {get: function () {
      return ghosttyMock.read('count windows', () => ghosttyMock.windows().length);
    }});
    return ghosttyMock.indexed(spec, (i) => ghosttyMock.windowSpec(i));
  };

  // indexed is spec, whose element i is element(i). As JXA's specifiers, an element names its
  // place, which a getter or a command finds only as it runs.
  ghosttyMock.indexed = function (spec, element) {
    return new Proxy(spec, {get: (target, key) =>
      typeof key === 'string' && /^[0-9]+$/.test(key) ? element(Number(key)) : target[key]});
  };

  // windowSpec is G.windows[i]: its tabs, each by its index, and its terminals.
  ghosttyMock.windowSpec = function (i) {
    const name = 'windows[' + i + ']';
    const window = function () {
      const w = ghosttyMock.windows()[i];
      if (!w) {
        ghosttyMock.fail(-1719, 'Invalid index.');
      }
      return w;
    };
    const tabs = {terminals: {id: function () {
      return ghosttyMock.read('get ' + name + '.tabs.terminals.id', () =>
        window().tabs.map((tab) => ghosttyMock.leaves(tab.tree)));
    }}};
    Object.defineProperty(tabs, 'length', {get: function () {
      return ghosttyMock.read('count ' + name + '.tabs', () => window().tabs.length);
    }});
    return {mockWindow: window, mockIndex: i, tabs: ghosttyMock.indexed(tabs, (j) => ({
      mockName: name + '.tabs[' + j + ']',
      mockTab: () => window().tabs[j] || ghosttyMock.fail(-1719, 'Invalid index.'),
      focusedTerminal: {id: function () {
        ghosttyMock.event('get ' + name + '.tabs[' + j + '].focusedTerminal.id');
        const tab = window().tabs[j] || ghosttyMock.fail(-1719, 'Invalid index.');
        if (!tab.focused) {
          ghosttyMock.fail(-1728, "Can't get focused terminal.");
        }
        return tab.focused;
      }},
    })), terminals: {id: function () {
      return ghosttyMock.read('get ' + name + '.terminals.id', () =>
        [].concat(...window().tabs.map((tab) => ghosttyMock.leaves(tab.tree))));
    }}};
  };

  // terminalsSpec is G.terminals: every terminal of the app, its windows' then those of none; and
  // each by its id.
  ghosttyMock.terminalsSpec = function () {
    const all = function () {
      const ids = [];
      ghosttyMock.windows().forEach((w) => w.tabs.forEach((tab) => {
        ids.push(...ghosttyMock.leaves(tab.tree));
      }));
      return ids.concat(ghosttyMock.model.hidden || []);
    };
    const spec = {
      id: function () {
        ghosttyMock.event('get terminals.id');
        return all();
      },
      byId: ghosttyMock.terminalSpec,
    };
    if (!ghosttyMock.model.mock.noTty) {
      spec.tty = function () {
        ghosttyMock.event('get terminals.tty');
        return all().map((id) => ghosttyMock.terminal(id).tty || '');
      };
    }
    return spec;
  };

  // terminalSpec is G.terminals.byId(id): the terminal id, as a command takes it, and its getters.
  ghosttyMock.terminalSpec = function (id) {
    const get = function (name, value) {
      return function () {
        ghosttyMock.event('get ' + name + ' of ' + id);
        ghosttyMock.found(id);
        return value();
      };
    };
    return {mockId: id, id: get('id', () => id),
      pid: get('pid', () => ghosttyMock.terminal(id).pid || 0)};
  };

  // make makes a terminal for a command, with a surface configuration c, and is its id. The
  // model's failAt-th fails: a split throws, and a new window's or tab's never appears, its id
  // null.
  ghosttyMock.make = function (c, split) {
    if (++ghosttyMock.made !== ghosttyMock.model.failAt) {
      return ghosttyMock.add(c);
    }
    if (split) {
      ghosttyMock.fail(-10000, 'Failed to create split.');
    }
    return null;
  };

  // add adds a terminal with a surface configuration c, whose shell waits at its prompt, as the
  // fake osascript's AddTerminal does, and is its id.
  ghosttyMock.add = function (c) {
    const m = ghosttyMock.model;
    m.serial = (m.serial || 0) + 1;
    const id = 'A1000000-0000-4000-8000-' + String(m.serial).padStart(12, '0');
    const tty = '/dev/ttys9' + String(m.serial).padStart(2, '0');
    const login = 50000 + ++m.serial;
    const shell = 50000 + ++m.serial;
    const start = new Date().toISOString().replace(/\.\d+Z$/, 'Z');
    const name = tty.slice('/dev/'.length);
    m.processes = (m.processes || []).concat([
      {pid: login, ppid: m.pid, pgid: login, tty: name, start: start,
        args: ['/usr/bin/login', '-flp', 'me', '/bin/bash', '--noprofile', '--norc', '-c',
          'exec -l /bin/zsh']},
      {pid: shell, ppid: login, pgid: shell, tty: name, start: start, args: ['-zsh']}]);
    m.terminals = m.terminals || {};
    m.terminals[id] = {name: '', cwd: (c && c.initialWorkingDirectory) || '', pid: shell, tty: tty,
      typed: null, input: (c && c.initialInput) || ''};
    return id;
  };

  // newWindow is a new window of one tab, the terminal id's.
  ghosttyMock.newWindow = function (id) {
    return {tabs: [{selected: true, focused: id, tree: {pane: id}}]};
  };

  // type types text into the terminal to, as the fake osascript records it.
  ghosttyMock.type = function (to, text) {
    const t = ghosttyMock.model.terminals[to.mockId];
    t.typed = (t.typed || []).concat([text]);
  };

  // commands are Ghostty's commands as the scripts send them.
  ghosttyMock.commands = {
    newWindow: function (opts) {
      ghosttyMock.event('new window');
      ghosttyMock.command();
      const id = ghosttyMock.make(opts.withConfiguration, false);
      if (id) {
        ghosttyMock.model.windows.unshift(ghosttyMock.newWindow(id));
      }
      return {};
    },
    newTab: function (opts) {
      ghosttyMock.event('new tab in windows[' + opts.in.mockIndex + ']');
      ghosttyMock.command();
      const w = opts.in.mockWindow();
      const id = ghosttyMock.make(opts.withConfiguration, false);
      if (id) {
        const selected = w.tabs.findIndex((t) => t.selected);
        const tab = {selected: true, focused: id, tree: {pane: id}};
        w.tabs.splice(selected >= 0 ? selected + 1 : w.tabs.length, 0, tab);
        ghosttyMock.front(w, tab);
      }
      return {};
    },
    focus: function (terminal) {
      const id = terminal.mockId;
      ghosttyMock.event('focus ' + id);
      ghosttyMock.command();
      ghosttyMock.found(id);
      ghosttyMock.queue.push(() => {
        const at = ghosttyMock.locate(id);
        if (at) {
          at.tab.focused = id;
          ghosttyMock.front(at.window, at.tab);
        }
      });
    },
    selectTab: function (tab) {
      ghosttyMock.event('select tab ' + tab.mockName);
      ghosttyMock.command();
      const selected = tab.mockTab();
      ghosttyMock.front(ghosttyMock.locate(ghosttyMock.leaves(selected.tree)[0]).window, selected);
    },
    sendKey: function (key, opts) {
      const mods = opts.modifiers ? ' ' + opts.modifiers : '';
      ghosttyMock.event('send key ' + key + mods + ' to ' + opts.to.mockId);
      ghosttyMock.command();
      ghosttyMock.found(opts.to.mockId);
      if (key === 'enter') {
        ghosttyMock.type(opts.to, '\r');
      } else {
        ghosttyMock.type(opts.to, opts.modifiers === 'control' ? '^' + key.toUpperCase() : key);
      }
    },
    inputText: function (text, opts) {
      ghosttyMock.event('input text to ' + opts.to.mockId);
      ghosttyMock.command();
      ghosttyMock.found(opts.to.mockId);
      ghosttyMock.type(opts.to, text);
    },
  };

  // Application is Ghostty's application object, as JXA gives it by bundle id.
  function Application(bundle) {
    if (bundle !== 'com.mitchellh.ghostty') {
      ghosttyMock.fail(-2700, 'Application can\'t be found.');
    }
    const G = Object.assign({
      version: function () {
        ghosttyMock.event('get version');
        if (ghosttyMock.model.mock.versionFails) {
          ghosttyMock.fail(-1728, 'Can\'t get version.');
        }
        return ghosttyMock.model.version;
      },
      newSurfaceConfiguration: function () {
        ghosttyMock.event('new surface configuration');
        ghosttyMock.command();
        return {};
      },
    }, ghosttyMock.commands);
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

  // delay only logs, and lets the user act: the mock keeps no pace.
  function delay(seconds) {
    ghosttyMock.log.push('delay ' + seconds);
    ghosttyMock.user('delay ' + seconds);
  }
