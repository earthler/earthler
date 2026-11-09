# Access proof

A throwaway experiment. It is not part of the Earthler system and must not be used to run a community.

## What it checks

Earthler is meant to be reached only through the browser, on a local network, with no internet access. Browsers only allow camera, microphone and offline use on HTTPS pages, and a public certificate needs the internet to be renewed. So each community will create its own certificate authority, and each device will install the community certificate once.

That design depends on three things that only real devices can confirm:

1. A phone can install the community certificate and open the site with no warning.
2. The phone keeps reaching the server on a Wi-Fi network that has no internet.
3. The phone refuses a certificate for an outside name, even when the community issuer signed it.

If any of these fails, the access design has to change before anything is built on top of it.

## What it does

- Creates a root certificate and an issuer, both limited to one `.internal` name and to private network addresses. The root key signs the issuer and is dropped. It is never written to disk.
- Serves a welcome page over HTTP, with the steps to install the certificate.
- Serves a test page over HTTPS that reports what the browser allows on that connection, and the same page over HTTP as a control.
- Answers any name outside the community with a forged certificate, signed by the issuer, and records whether the device accepted it.

The tests in `chain_test.go` confirm that the Go verifier accepts and refuses what it should. They say nothing about phones. That is what the experiment is for.

## Run

From the root of the repository, on a computer joined to the Wi-Fi network:

```bash
go run ./experiments/access-proof
```

It listens on ports 80 and 443 and prints the address to open on each phone. If those ports are busy or not allowed, pick others:

```bash
go run ./experiments/access-proof -http 8080 -https 8443
```

Other options: `-name` sets the community name (the default is `demo`, for `demo.internal`), `-ip` chooses the local address, and `-data` chooses where the chain and the results are kept.

## Test on each phone

Repeat this on every phone you can, and write down the model and the system version in the notes field.

1. Join the Wi-Fi network and type the address the terminal printed, starting with `http://`.
2. Compare the fingerprint on the page with the one in the terminal.
3. Download and install the certificate, following the steps on the page. Note whether the phone showed the fingerprint during the installation.
4. Wait for the page to say the device trusts the certificate, and open the secure site. It must open with no warning.
5. On the secure site, tap Test camera and microphone, choose "The network has internet" and save the result.
6. Open the forged certificate links. The right outcome is a browser error.
7. Cut the internet at the router, leaving the Wi-Fi on. Turn mobile data off, reload the secure site and save the result as "Internet cut, mobile data off".
8. Turn mobile data on, reload and save the result as "Internet cut, mobile data on". If the page does not load, that is a result too: write it down.
9. Open the control page and save a result there, to record what the browser refuses without HTTPS.

## Read the results

Everything is appended to `access-proof-data/results.jsonl`, one JSON object per line, and shown in the terminal.

- A saved result lists each check with `true`, `false` or a short text.
- `forged-rejected` means a device refused a forged certificate. It only counts when `device_trusts_real_certificate` is `true`, because a device that has not installed the certificate refuses everything.
- `forged-accepted` means a device accepted a forged certificate. One such line from a phone means that phone does not enforce the limits.

## Clean up

When the test is over, remove the certificate from every device and delete the data folder, which holds the issuer key.

- iPhone or iPad: Settings, General, VPN and Device Management, then remove the profile.
- Android: Settings, Security, Encryption and credentials, Trusted credentials, User, then remove the certificate.
- Computer: delete the certificate from the system certificate store.
- This machine: delete `access-proof-data`.

## Limits

- The link that uses sslip.io sends a DNS question, containing this machine's local address, to a public service. It needs internet, and some routers block answers that point to a local address. In that case the terminal shows no line for that name, and the test says nothing.
- The experiment does not test push notifications, calls or anything else in the system.
