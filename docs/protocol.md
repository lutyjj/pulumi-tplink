# Router protocol

The client implements TP-Link's undocumented local HTTPS web API, not its cloud
API. Archer clients using an AES session envelope cannot substitute for this
RSA-only transport.

## Verified device

| Field | Reported value |
| --- | --- |
| Model and hardware | Archer AXE75 v1.0 |
| Firmware | `1.5.6 Build 20260623 rel.56854(4555)` |
| Configured region | DE |

Firmware and hardware versions come from an authenticated `read` of
`admin/firmware?form=upgrade`. The HTML version tag identifies the web UI build,
not the installed firmware. Region comes from `locale?form=country`.

The unmodified client successfully authenticates and reads DHCP settings,
reservations, UPnP, DMZ, remote administration and both NAT tables on this build.
This check verifies read compatibility, not all write operations. The NAT tables
were empty, so populated rule parsing and deletion were not exercised live.

The Pulumi schema describes the resources implemented in `provider/`. It is not
a specification of the router's entire API. Generated SDKs expose those resources;
adding a router feature still requires client and resource code with lifecycle tests.

## Authentication

1. POST `/cgi-bin/luci/;stok=/login?form=keys` with `operation=read`.
   The response supplies the RSA public modulus and exponent as hexadecimal strings.
2. Encrypt the UTF-8 admin password using RSA PKCS#1 v1.5 and encode the ciphertext
   as hexadecimal. The firmware requires this padding; OAEP is not wire-compatible.
3. POST `/cgi-bin/luci/;stok=/login?form=login` with
   `operation=login&password=<ciphertext>`.
4. Use the returned `stok` and session cookie on form-encoded requests under
   `/cgi-bin/luci/;stok=<stok>/admin/...`.
5. POST `admin/system?form=logout` with `operation=write` in a cleanup handler.

There is no AES session envelope or request signing in this firmware. TLS verification
is enabled unless the consumer explicitly selects `insecure`. Redirects are refused,
and responses are bounded to 8 MiB. Each HTTP request has a 15-second timeout.

The router permits one admin session. The client serializes login-through-logout
per host inside its process. Separate deployments and browser sessions must not
overlap. Cancellation still attempts logout with a bounded cleanup context.

## Resource semantics

DHCP reservations use `admin/dhcps?form=reservation` with `load`, `insert` and `remove`.
The insert body carries a JSON `new` row with MAC, IP, hostname and enable fields.
Removal addresses the row by `index`; the firmware ignores `key`. Resolve the index
from a fresh table read each time. The addressing of `update` has not been verified,
so readdressing uses remove and insert rather than that operation. Reads validate
MAC, IPv4 address, hostname and enable fields before changing any record. Declaring
a reservation requires it to be enabled; refresh detects external disablement.
Readdressing preserves the freshly observed hostname when no name is managed,
and rollback restores the original row's enable state as well as its address.

Displayed device names live in `admin/traffic?form=dev_name`, keyed by MAC. They
outrank a reservation row's hostname for devices the router has seen. An empty alias
answers success and changes nothing, so the resource rejects blank names. Destroy
removes the reservation but leaves the name, which the firmware cannot unset.

Settings writes replace the entire record. DHCP and toggle resources read, merge
managed fields, and write the complete record back. Removing a settings resource
stops management without blanking or restoring values.

Toggles hold one `on`/`off` field. UPnP, DMZ and remote administration merge it into
the full record. EasyMesh (`admin/easymesh?form=easymesh_enable`) and media sharing
(`admin/folder_sharing?form=media`, field `media_sharing`) send that field alone,
matching the web UI's writes. The EasyMesh record also reports `time`, which the
UI never writes back. `admin/nat?form=alg` holds eight `on`/`off` fields: `ftp`,
`tftp`, `h323`, `rtsp`, `sip`, `pptp`, `l2tp` and `ipsec`. The UI writes all eight
together, and the resource declares every one. Reads fail closed on a missing field
or any other value. These write shapes come from the web UI, not from a live write test.

The UI selects controls from the capability record at `device_config?form=config`.
On the verified device it reports `mainNetwork.ofdmaMumimo`, so the UI shows the
combined `admin/wireless?form=ofdma_mimo` selector, not the `form=ofdma` switch.
It reports `mainNetwork.ffs` as false, so the UI hides Amazon Wi-Fi Simple Setup
(`admin/ffs?form=config`) although that record still reads. The provider manages
neither.

NAT port forwarding (`admin/nat?form=vs`) and port triggering (`form=pt`) are positional
tables. The `InboundRules` resource asserts both are empty and removes rows from last
to first so deletion cannot shift an index still queued for removal. Unknown table
shapes fail closed. Both creation and update previews read and report existing
rules without removing them. This resource does not author rules or restore them
on destroy.
