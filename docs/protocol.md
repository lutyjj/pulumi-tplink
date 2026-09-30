# Router protocol

The client implements the local HTTPS web API verified on Archer AXE75 firmware
1.10.5. This is an undocumented protocol, not TP-Link's cloud API. Older Archer
clients using an AES session envelope cannot substitute for this transport.

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

NAT port forwarding (`admin/nat?form=vs`) and port triggering (`form=pt`) are positional
tables. The `InboundRules` resource asserts both are empty and removes rows from last
to first so deletion cannot shift an index still queued for removal. Unknown table
shapes fail closed. Both creation and update previews read and report existing
rules without removing them. This resource does not author rules or restore them
on destroy.
