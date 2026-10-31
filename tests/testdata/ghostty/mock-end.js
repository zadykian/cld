
  // mockRun runs the script's run with argv against the model argv[1], and is what it printed,
  // the model after it, its moves of focus landed, and the Apple Events it sent:
  // {result, model, log}.
  mockRun = function (argv) {
    ghosttyMock.model = JSON.parse(argv[1]);
    ghosttyMock.model.mock = ghosttyMock.model.mock || {};
    ghosttyMock.model.windows = ghosttyMock.model.windows || [];
    const out = run(argv);
    ghosttyMock.settle();
    delete ghosttyMock.model.mock;
    return JSON.stringify({result: JSON.parse(out), model: ghosttyMock.model,
      log: ghosttyMock.log});
  };
})();

// run is the handler osascript calls, outside the mock's function.
function run(argv) {
  return mockRun(argv);
}
