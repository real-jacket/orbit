const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'mole-run-test-'));
const log = path.join(directory, 'commands.log');
const script = path.resolve(__dirname, 'run-app.sh');
const writeCommand = (name, content) => fs.writeFileSync(path.join(directory, name), '#!/bin/bash\nset -e\n' + content, { mode: 0o700 });
try {
  writeCommand('go', 'if [ "$1" = env ]; then if [ "$2" = GOOS ]; then echo darwin; else echo arm64; fi; exit; fi\nprintf "go %s\\n" "$*" >> "$TEST_LOG"\n');
  writeCommand('pgrep', 'if [ "${TEST_RAW:-0}" = 1 ] && [[ "$*" == *go-build* ]]; then echo 456; exit 0; fi\nexit 1\n');
  writeCommand('codesign', 'printf "codesign %s\\n" "$*" >> "$TEST_LOG"\n');
  writeCommand('open', 'printf "open %s\\n" "$*" >> "$TEST_LOG"\n');
  const environment = { ...process.env, PATH: directory + ':/usr/bin:/bin', GO: path.join(directory, 'go'), MYGO: path.join(directory, 'missing-mygo'), TEST_LOG: log };
  const blocked = spawnSync('/bin/bash', [script], { env: { ...environment, TEST_RAW: '1' }, encoding: 'utf8' });
  assert.notEqual(blocked.status, 0);
  assert.match(blocked.stderr, /Ctrl-C/);
  assert.equal(fs.existsSync(log), false, 'an existing raw launch is neither killed nor rebuilt');
  const launched = spawnSync('/bin/bash', [script], { env: environment, encoding: 'utf8' });
  assert.equal(launched.status, 0, launched.stderr);
  const commands = fs.readFileSync(log, 'utf8');
  assert.match(commands, /go run github\.com\/egoist\/mygo\/cmd\/mygo@v0\.6\.4 build -o build --skip-dmg/);
  assert.match(commands, /codesign --verify --deep --strict .*build\/darwin-arm64\/orbit\.app/);
  assert.match(commands, /open -n .*build\/darwin-arm64\/orbit\.app/);
  assert.doesNotMatch(commands, /go run \.$|kill|tccutil/m);
  const makeEnvironment = { ...environment };
  delete makeEnvironment.MYGO;
  const makeCommand = (...args) => spawnSync('/usr/bin/make', ['--no-print-directory', '-n', ...args], {
    cwd: path.resolve(__dirname, '..'), env: makeEnvironment, encoding: 'utf8',
  });
  for (const [target, arguments_] of Object.entries({ app: 'build', 'app-nodmg': 'build --skip-dmg', dev: 'dev', generate: 'generate', doctor: 'doctor' })) {
    const result = makeCommand(target);
    assert.equal(result.status, 0, result.stderr);
    assert.equal(result.stdout.trim(), `${environment.GO} run github.com/egoist/mygo/cmd/mygo@v0.6.4 ${arguments_}`);
  }
  writeCommand('mygo', 'exit 0\n');
  const installed = makeCommand('app-nodmg');
  assert.equal(installed.status, 0, installed.stderr);
  assert.equal(installed.stdout.trim(), 'mygo build --skip-dmg');
  const overridden = makeCommand('app-nodmg', `MYGO=${path.join(directory, 'mygo')}`);
  assert.equal(overridden.status, 0, overridden.stderr);
  assert.equal(overridden.stdout.trim(), `${path.join(directory, 'mygo')} build --skip-dmg`);
  console.log('Run entry checks passed: stable bundle, pinned builder fallback, CLI overrides, protected existing raw process.');
} finally {
  fs.rmSync(directory, { recursive: true, force: true });
}
