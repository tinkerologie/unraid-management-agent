# Producer contract fixtures

The corpus in [v1](v1/manifest.json) captures synthetic responses from the real
REST router and WebSocket event bridge. It covers network service names, ZFS
file lists, partial collector updates, UPS and NUT, notifications, share usage,
and source health. Each fixture preserves wire keys, number values and
null/empty/omitted distinctions. An explicitly listed `/timestamp` is validated
against the request window before replacement with the manifest sentinel.

Consumers can pin a commit SHA or release tag and copy `contracts/v1/manifest.json`
and the listed JSON files from that ref. Verify each file's SHA-256 against the
manifest before replaying it. The ref identifies the producer commit, while the
manifest versions the schema and generator. Consumer tests can vendor the files
and run offline. No Unraid host or Go toolchain is required for replay.

Run `go test ./daemon/services/api -run '^TestProducerContract'` to verify the
corpus. The regular Go test suite also runs these checks. For an intentional
wire change, run `make contract-fixtures`, review the generated diff and update
the changelog. If Detect Secrets flags a new manifest checksum, verify it
against the fixture and audit that manifest entry in `.secrets.baseline`. Delete a retired fixture explicitly when removing its case.
The generator rejects extra files and non-regular entries in the fixture directory.
