# Validate the plugin lifecycle on Unraid

Use a disposable x86-64 Unraid 7.4 system with its own boot device and configuration. Record the
Unraid and agent versions and the candidate bundle SHA256. Keep test manifests and tokens outside
the repository.

## Prepare the artifacts

1. Build a candidate with a version newer than the baseline. Run `make package` and the package
   artifact and ownership checks.
2. Copy the candidate manifest to a private test directory. Point its update and bundle URLs at
   the test server. Set the candidate version and use `scripts/update-plg-checksums.sh` to
   insert the checksums of the exact bundle. Check that the manifest parses as XML.
3. Copy the published baseline manifest. Point its update URL at the test server. Keep the
   baseline bundle and checksums from the published release.
4. Copy the candidate manifest and set an incorrect SHA256 for the integrity failure check.

## Run the lifecycle

1. On a clean test system, open **Plugins → Install Plugin** and install the wrong-checksum
   manifest. Confirm that the native manager reports an integrity failure and leaves the
   daemon uninstalled.
2. Install the valid candidate. Confirm that native progress appears, the installed
   `VERSION` and bundle match the candidate, and `GET /api/v1/health` returns HTTP 200.
3. Remove the candidate and install the baseline.
4. Set a non-default collector interval. Record the service PID and save a private copy
   of `config.cfg` for the comparison.
5. Serve the valid candidate at the baseline update URL.
6. Select **Check for Updates** and update the plugin from **Installed Plugins**. Confirm the
   new PID, version, bundle checksum, HTTP 200 and retained configuration.
7. Remove the candidate and install the baseline again.
8. Stop the service and repeat the upgrade. Confirm the candidate version and verify that the
   service remains stopped.
9. Test an unreachable update URL and record the native manager's result.
10. Remove the test plugin. Confirm that the process, HTTP service, install directory, plugin
    configuration directory, and main log are gone. Record whether the manifest and any rotated
    logs remain.

The packaging test executes the embedded version check on either side of the 7.2.0 boundary.
An installation on 7.4 does not exercise the native manager's rejection of an older OS.
Ansible package deployment exercises a different installation path.

## Compare update status

Compare **Installed Plugins** with `GET /api/v1/plugins` after the native manager has checked
for updates. Compare the installable subset with `GET /api/v1/plugins/check-updates` after a
scheduled agent check. Use a disposable system to create validation failures and incompatible
updates. Treat `plugin checkall` as a refresh that writes cached metadata.

Record status names, versions, and counts. Keep credentials, configuration contents, and private
URLs out of shared logs.
