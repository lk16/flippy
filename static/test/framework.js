// A tiny zero-dependency test framework. Test files register cases with test(name, fn);
// each fn throws (via node:assert) to fail. run.js calls runAll() once at the end.
const cases = [];

function test(name, fn) {
  cases.push({ name, fn });
}

async function runAll() {
  let passed = 0;
  const failures = [];
  for (const { name, fn } of cases) {
    try {
      await fn();
      passed++;
      console.log(`  ok   ${name}`);
    } catch (err) {
      failures.push({ name, err });
      console.log(`  FAIL ${name}`);
      console.log(String(err && err.stack ? err.stack : err).replace(/^/gm, '       '));
    }
  }
  console.log(`\n${passed}/${cases.length} passed, ${failures.length} failed`);
  return failures.length === 0;
}

module.exports = { test, runAll };
