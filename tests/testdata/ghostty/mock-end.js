
  // mockRun runs the script's run with argv against the model argv[1], and is what it printed,
  // the model after it, and the Apple Events it sent: {result, model, log}.
  mockRun = function (argv) {
    ghosttyMock.model = JSON.parse(argv[1]);
    ghosttyMock.model.mock = ghosttyMock.model.mock || {};
    const out = run(argv);
    delete ghosttyMock.model.mock;
    return JSON.stringify({result: JSON.parse(out), model: ghosttyMock.model,
      log: ghosttyMock.log});
  };
})();

// run is the handler osascript calls, outside the mock's function.
function run(argv) {
  return mockRun(argv);
}
