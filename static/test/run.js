#!/usr/bin/env node
// Node-runnable test suite for the browser JS under static/. No framework, no build
// step: each ./*.test.js file registers cases via ./framework, and this runner executes
// them all, printing per-test pass/fail and a summary. Exit code is 1 if anything failed.
const { runAll } = require('./framework');

// Register test cases (each require() populates the shared framework registry).
require('./pass.test');
require('./diverge.test');
require('./flip.test');
require('./level.test');

runAll().then((ok) => process.exit(ok ? 0 : 1));
