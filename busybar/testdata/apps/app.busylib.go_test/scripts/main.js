// Prove execution before the test requests shutdown.
localStorage.setItem("ready", "true");
listen("input", function () {});
