# Testing

```bash
./test.sh           # everything: gofmt, go vet, Go tests (with the race detector), the pixel engine, browser tests
./test.sh editor    # only browser tests with "editor" in their name
KEEP=1 node tests/run.mjs drivers   # one browser test, keeping its screenshots
```

## Tests first

New work starts with its test: the Go test or browser test that says what should happen, failing, then the code
that makes it pass. Bugs found by hand get a test that fails without the fix (the driver tests have several:
"found testing on a real computer").

## Rules

- **Fix the code, not the test.** A failing test is changed only if it's provably wrong, and then the reason goes
  in the commit, and the check stays at least as strict. If a test can't observe the right thing, the code gets a
  proper hook (`jobActiveHook`, `cancelHook`, the pretend computer) instead of a looser check.
- **A new test must fail without its fix.** Revert the fix (or break the code on purpose) and watch it fail. The
  notification tests were checked this way, and one gap (low ink nagging at every percent) was found and closed.
- **"Flaky" is a bug until proven otherwise.** Reproduce it (run it many times, and under load: the full suite
  alongside `go test -race`), then find the cause. Two intermittent failures turned out to be one real bug: the
  editor lost a line drawn just before Save, Undo or closing on a busy computer (`tests/quicksave.test.mjs`).
- **Wait for results, not for what's on screen.** A test waits for the thing it checks (with a time limit it fails
  at), never a fixed pause or the look of a message box.

## Go tests (`*_test.go`)

Unit tests for everything with logic: PINs, sessions, login limits, phone access, the IPP message format, raster
decoding, paper savers, file formats, editor autosave, Print again, driver plans, package handling, the queue
script (run against stand-in CUPS commands), paper size from the time zone.

## Browser tests (`tests/*.test.mjs`)

`tests/run.mjs` builds the program, starts it on a spare port with throwaway settings and data folders, and drives
a headless Chromium like a phone (touch, drags, file choosers, slowed network), checking screens and, often, the
saved pictures pixel by pixel. Nothing is ever really printed (`SAKURA_DRY_PRINT`), nothing is announced on the
WiFi (`SAKURA_NO_BONJOUR`), and pretend phones say who they are with a header (`SAKURA_TEST_FAKE_MAC`). Each test
file exports `default async function (t)`; `t.launch()` opens a browser (closed afterwards even if the test
crashes); `t.resetSettings(patch, env)` restarts the server with other settings or environment.

## The pretend computer

`SAKURA_FAKE_SYSTEM=<file.json>` replaces the computer for driver setup (`drivers_sys.go: fakeSystem`): which
programs exist, what commands print, what downloads contain, and what changes after an install. What would have
run as the administrator is written to a log instead. Tests use it to try Debian, Fedora, Arch, NixOS and CUPS 3
without installing anything.

## Real hardware

Some things only real hardware shows (five driver bugs did). `sakuraprint doctor drivers` and Settings → Advanced
show what the real computer sees; the driver setup was tested for real by removing and reinstalling a Brother
printer and scanner driver (with a backup first).
