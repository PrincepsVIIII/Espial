# Website and certificate diagnosis

## Website checks

1. Open `/webpages/{id}` and read the authoritative DNS, TCP, TLS, HTTP, and body
   stages in order. The first failed stage and safe reason code define the next
   check; do not infer failure from a missing later stage.
2. Verify the monitor's exact host, resolved IPv4/IPv6 addresses, port, redirect
   ceiling, status set, timeout, and optional exact content expectation against the
   approved change. Protected values remain mounted files and are never readable in
   the UI.
3. DNS policy rejection means either the host or at least one answer is outside the
   allowlist. Approve each answer deliberately. Redirect targets require independent
   host/address/port approval and protected headers cannot cross origins.
4. Use the audited manual check only after correcting configuration. Adapter crash
   evidence belongs to integration runtime; normal freshness must make the webpage
   Stale and then Unknown while Core stays ready.

## Certificates

1. Open `/webpages/certificates/{id}` and verify endpoint, observed time, freshness,
   subject/SAN summary, issuer, SHA-256 fingerprint, validity interval, hostname
   validity, chain validity, remaining days, and linked incident.
2. `certificate.no_certificate`, untrusted chain, hostname mismatch, expired, and
   not-yet-valid are distinct failures. Do not weaken TLS verification to make a
   monitor healthy. Correct the endpoint certificate, hostname, approved trust
   roots, or monitor URL as appropriate.
3. Default expiry thresholds are warning at 30 days, critical at 14 days, and a
   meaningful critical condition update at 7 days. A threshold crossing preserves
   the active incident fingerprint rather than opening daily incidents.
4. Fingerprint or issuer change is replacement evidence, not a failure by itself.
   Compare the observation with the approved certificate change. Espial never
   retains private keys, full chains, trust material, or raw handshake errors.
5. Missing evidence is `Unknown` or `Not reported`; never treat it as zero days or a
   valid certificate. Check clock synchronization when validity boundaries disagree
   with the issuer's record.

Use maintenance for planned certificate replacement only when expected failure
should not open or worsen an incident. The raw check remains visible throughout.
