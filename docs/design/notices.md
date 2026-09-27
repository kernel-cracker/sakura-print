# The notifications centre

Everything that needs the person, in one list, most urgent first (`notices.go: collectNotices`). The web app shows it
behind the bell with a count (`web/js/12-notices.js`); the bell asks every 15 seconds, when a print changes state, and
when the app comes back into view.

| Order | Notice | Level | For | A tap |
|---|---|---|---|---|
| 1 | Turn the paper over: "Homework" | action | everyone | opens that print's flip guide |
| 2 | Finish setting up | action | the computer | opens setup |
| 3 | The printer's problems: out of paper, paper jam, a cover open, out of ink, tray out, can't be reached, paused | problem | everyone | Printer care |
| 4 | A print is waiting (from a phone that hasn't logged in) | action | everyone | home, to allow it or not |
| 5 | Low ink (15% or less), per colour | warning | everyone | Printer care |
| 6 | A newer driver for a printer (checked once a day) | info | the computer | Printers & drivers |
| 7 | Phone access turns off soon (15 minutes or less) | info | the computer | **Keep on for an hour** |

The printer's problems come from CUPS's `printer-state-reasons` (RFC 8011 keywords, the `-error`/`-warning`/`-report`
ending dropped) and put in plain words with what to do; unknown reasons are left out rather than shown raw. A stopped
printer with no reason given is "paused". Low ink is said once, from the ink levels when the printer reports them.

**Putting a notice away**: anything but an action can be put away (`dismissals`), and stays away until it changes: low
ink comes back when it drops another 5%, a driver notice when a newer version appears, the phone access notice when
the time changes. Prints waiting for a flip or for permission can't be put away: they need finishing or deciding.

**The computer's own desktop** gets a notification for a double-sided print from another app that needs turning over,
with a **Print the other side** button (libnotify actions, `ipp_jobs.go: flipNotify`), and for prints from unknown phones.

Tests: `notices_test.go` (the order, what phones see, plain words for each reason, putting away; checked against
deliberately broken code), `tests/notices.test.mjs` (the bell, the list, a flip made by a real IPP client, "Keep on for
an hour").
