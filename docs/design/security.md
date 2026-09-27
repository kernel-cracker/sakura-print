# Security

Sakura Print is for a home, but it's reachable by everyone on the WiFi, and apartment and hostel WiFi is shared
with strangers. It uses plain HTTP (the person chose not to use HTTPS), so the design assumes others on the WiFi
can see traffic, and limits what they can do.

## The computer and phones

`isLocal(r)` (`security.go`): the request comes from this computer **and** is addressed to `localhost` or a
loopback address. Checking the address stops a web page in the computer's browser from reaching in through a
look-alike name (DNS rebinding). The computer never needs a PIN, and only the computer can change PINs, phone
access, print from any app, install drivers, or see diagnostics.

## PINs and sessions

- Each person can have their own PIN (4 to 8 digits), stored salted and hashed (PBKDF2-SHA256, 200,000 rounds);
  a PIN can't be shown again, only removed.
- Logging in gives a random session cookie (`HttpOnly`, `SameSite=Strict`); only its SHA-256 is kept. Sessions
  last a year; removing a PIN logs out the phones that used it.
- Wrong PINs (`loginLimiter`): 5 per phone per 10 minutes (IPv6 phones counted by their /64), and 30 per hour from
  everyone together, which pauses new logins. Logged-in phones are never affected.

## Phone access

The switch (`phonesAllowed`): always, off, or on until a time (1 or 3 hours), then off by itself. While off,
anything not from the computer gets a friendly "phone access is off" page and nothing else, not even the app's
files. Print from any app and Bonjour stop too.

## Every response

`protect()`: `http.CrossOriginProtection` (no other website can make the browser post to Sakura Print),
`X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`. (No Content-Security-Policy yet: the app uses inline styles.)

## Print from any app

AirPrint can't ask for a PIN, so a phone earns it by logging in with a PIN once: the computer then knows it by its
WiFi hardware address (`devices.go`), from the neighbour table. Prints from other phones wait, with a picture, for
someone to allow them. A phone can only cancel or add pages to its own jobs. A MAC address isn't secret, so this
keeps out neighbours, not a determined attacker; the worst they could do is print. See [ipp.md](ipp.md).

## The administrator

Sakura Print runs as the person. Installing drivers, and nothing else, uses the administrator, through polkit's
own password window (`pkexec`), once per install, from the computer only. Downloads are HTTPS to the makers' own
addresses, remembered by fingerprint. See [drivers.md](drivers.md).

## Files

Uploads are checked by content and kept apart from the person's files; the computer's files can only be read from
inside the home folder (links are followed and checked). Settings are written with mode 600.
