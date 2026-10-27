# PSU telemetry feasibility and automatic discovery

Research date: 27 October 2026. Scope: issues [#61][i61], [#194][i194], [#195][i195],
[#196][i196] and [#197][i197]. This records the research baseline and phased implementation decision. The initial Corsair
collector is now implemented; see [PSU telemetry](../guides/psu-telemetry.md) for current behavior.
Hardware validation remains separate.

## Decision

Adding PSU monitoring with automatic discovery is feasible. Start with Corsair RMi/HXi through
Linux hwmon, then NZXT E500/E650/E850 through a separate native HID backend. Use one PSU collector
and shared data contract for every vendor. Cooler Master is a credible additional hwmon backend
when its external driver is already installed.

Do not promise universal PSU detection. The host can discover an enumerated telemetry interface;
it cannot reliably identify an ordinary ATX PSU through its power cables. Missing USB evidence
can mean an unconnected data cable, VM passthrough, missing driver, or non-reporting hardware.
Successful model-specific reads are needed to establish working telemetry, especially when a
USB dongle can remain attached without a responding PSU. Vendor manuals explicitly require the
data connection for software monitoring. [Corsair manual][hxi-manual], [NZXT FAQ][nzxt-faq],
[EVGA manual][evga-manual].

The evidence does not justify generic retail EVGA, Seasonic, be quiet! or SilverStone support.
It does establish specific newer products and server interfaces worth investigating. Keep
hardware capability, Linux protocol availability, host driver availability, and agent validation
as separate compatibility facts.

## Model-specific support matrix

| Family or model                                                  | Evidence                                                                                                                                           | Proposed agent support                                                                                                                                                                                                                                      |
| ---------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Corsair RMi and HXi with supported USB IDs                       | Mainline `corsair-psu` driver; hwmon chip name `corsairpsu`; documented sensors and USB hotplug.                                                   | First implementation. Read existing hwmon attributes in native Go. Match the actual host kernel's support, not just current upstream. [Kernel documentation][corsair-doc], [source][corsair-src].                                                           |
| Corsair AX760i/AX860i/AX1200i/AX1500i/AX1600i                    | Different Link/dongle protocols; absent from the mainline RMi/HXi driver's ID table. Existing userspace implementations provide research material. | Separate later backend. Require exact model, bridge identity, validated transactions and hardware testing. A dongle match is insufficient. [Maintainer TODO][corsair-todo], [OpenCorsairLink][ocl].                                                         |
| NZXT E500/E650/E850                                              | Original liquidctl implementation and vendor USB/PMBus documentation. IDs `7793:5911`, `7793:5912`, `7793:2500`.                                   | Feasible native Go HID backend, separately tested. No vendor software or cloud required by the demonstrated protocol. [Driver][nzxt-src], [vendor announcement][nzxt-announcement].                                                                         |
| NZXT C models covered by the vendor FAQ                          | Vendor explicitly distinguishes E USB monitoring from C models without CAM monitoring.                                                             | Exclude these C models. Reassess new revisions by exact model, not the C name alone. [FAQ][nzxt-faq].                                                                                                                                                       |
| EVGA NEX1500 Classified, 120-PG-1500-XR                          | Vendor manual proves USB monitoring and a supplied motherboard-header cable.                                                                       | Plausible future backend; no validated Linux protocol, exact USB IDs or production Linux implementation established here. Other SuperNOVA families are not automatically compatible. [Manual][evga-manual], [launch][evga-launch].                          |
| Seasonic conventional PRIME/FOCUS/VERTEX; CONNECT/SYNCRO         | Retail names, efficiency certification and power-distribution backplanes do not establish host telemetry.                                          | No verified generic backend. Do not infer compatibility from the Seasonic OEM platform inside NZXT E. [SYNCRO][syncro], [NZXT announcement][nzxt-announcement].                                                                                             |
| Seasonic PRIME ENTERPRISE TX/PX-1300/1600 OptiGuard              | Official 2026 presentation explicitly advertises Bluetooth monitoring; OptiGuard 2.0 monitors the PSU-to-GPU power path.                           | Worth a separate Bluetooth protocol investigation. Public GATT/API and usable Linux support were not established. Announcement does not prove availability of every listed configuration. [Computex][seasonic-computex], [technical recap][seasonic-recap]. |
| Seasonic server/OEM PMBus models                                 | Official SIC-NRX522-22A63GT material documents PMBus/Blackbox. Other industrial claims require model datasheets.                                   | Read only already-bound, positively identified PSU hwmon devices or an established BMC interface. No automatic I2C address scan. [Computex][seasonic-computex], [PMBus driver][pmbus-doc].                                                                  |
| Cooler Master V850 Gold i multi; X SILENT Edge Platinum 850/1100 | Original `cm-psu` project reports support through a separately installed kernel module, exposing hwmon readings.                                   | Practical second-stage expansion by consuming existing `cmpsu` hwmon. Stock Unraid module availability is unverified. XG650/750/850 use an unsupported protocol. [Project][cm-project], [source][cm-src].                                                   |
| be quiet! Dark Power Pro 14 IO 1300/1600W                        | Official 18 August 2026 release calls this its first PSU with smart software control; IO Center displays efficiency, fan speed and power usage.    | Real telemetry-capable candidate, but Linux protocol, USB IDs and exact transport require validation. Older Pro 12/13 digital regulation does not establish the same interface. [Announcement][bq-release].                                                 |
| SilverStone server CRPS systems                                  | CPB01 power-distribution board documents PMBus connectivity for compatible redundant PSU systems.                                                  | Conditional server/backend candidate. This is not evidence of telemetry in ordinary Strider models. [CPB01][silverstone-cpb01].                                                                                                                             |
| Thermaltake Toughpower DPS G/iRGB variants                       | Vendor software monitoring exists on particular models; historical Linux implementation work exists.                                               | Additional protocol research, not a supported native backend yet. RGB support alone is insufficient. [Vendor product][tt-product], [original implementation discussion][tt-work].                                                                           |
| ASUS ROG Thor 1200W Platinum III                                 | Official page advertises a local power display; inspected accessory specifications do not establish USB telemetry.                                 | Do not infer a host interface from OLED, Aura or cooling-device drivers. No usable PSU Linux backend established here. [Product][asus-product], [specification][asus-spec].                                                                                 |

### Corsair IDs and measured values

The current upstream source declares USB vendor `1b1c` and these products. These are upstream
declarations, not a tested Unraid compatibility list. Older host kernels may lack later IDs.
[Kernel ID table][corsair-src].

| PID            | Upstream model description                 |
| -------------- | ------------------------------------------ |
| `1c03`–`1c06`  | HX550i, HX650i, HX750i, HX850i             |
| `1c07`, `1c08` | HX1000i Legacy, HX1200i Legacy             |
| `1c09`–`1c0d`  | RM550i, RM650i, RM750i, RM850i, RM1000i    |
| `1c1e`         | HX1000i Series 2023                        |
| `1c1f`         | HX1500i Legacy / Series 2023 / Series 2025 |
| `1c23`, `1c27` | HX1200i Series 2023, HX1200i Series 2025   |

Year labels differ between projects. Store VID/PID and device-reported model rather than using
marketing year labels as the sole match. [liquidctl table][corsair-liquidctl].

For this backend, `power1_input` means total **DC output power**, not wall consumption.
`power2`–`power4` correspond to output rails. `in0_input` is AC input voltage; output voltages
are `in1`–`in3`. Rail currents are `curr2`–`curr4`; AC input current at `curr1` is optional in
the current source. Temperatures are VRM/case; `fan1_input` is RPM. Enumerate existing attributes
and read labels instead of assuming all files exist. Debugfs identity/uptime data are optional.
[Kernel documentation][corsair-doc], [implementation][corsair-src].

Do not call AC volts multiplied by amps real input watts: it is apparent power unless power
factor is accounted for. liquidctl's Corsair guide explicitly describes estimated input power
and efficiency, with low-load accuracy limits. Leave measured input watts and efficiency null
when the source cannot provide them. Derive load only from verified capacity and DC output;
label it as calculated. [liquidctl guide][corsair-guide].

### NZXT telemetry and native implementation effort

The source provides temperature, fan RPM, firmware, and voltage/current/power for five rail
groups: peripheral 12V, EPS/ATX12V 12V, motherboard/PCIe 12V, combined 5V, and combined 3.3V.
Its implemented status path does not provide measured AC input watts, efficiency or runtime.
A sum of rail power must be described as derived DC output. [Original driver][nzxt-src].

The protocol uses HID reports carrying PMBus requests. The existing implementation uses
64-byte reports, spacing and bounded retries; it does not provide complete PMBus PEC validation.
Treat it as protocol evidence and a fixture reference, not proof that a new Go backend is correct.
Native Linux support is technically plausible using `golang.org/x/sys/unix`, already a dependency,
with nonblocking device I/O, polling deadlines, response validation and cancellation. This is an
engineering proposal; it has not been built or tested here. [Source][nzxt-src],
[existing dependency list](../../go.mod).

### Seasonic PMBus evidence limits

The existing #195 plan overstates what follows from a PSU supporting PMBus. The motherboard,
backplane or BMC must expose a usable path; Linux must already have the correct device bound;
identification and sensor support still need validation. The generic PMBus driver explicitly
avoids automatic device probing because there is no universally safe identification register
or well-defined address range. [Kernel PMBus documentation][pmbus-doc].

SIC-NRX522-22A63GT PMBus support is confirmed by the official page. A supplier-hosted,
manufacturer-format [S2N-851CCP5-B specification][seasonic-851] documents manufacturer/model
identity and PMBus readings, but its provenance is weaker than an original manufacturer-hosted
download. S2N-302CCP5 remains a supplier-listing lead rather than a validated implementation
target. None of these documents proves a compatible Unraid host binding. Do not require debugfs
to establish baseline availability or assume its identity files are a stable universal ABI.

## Automatic discovery and connection semantics

Use a cheap inventory scan followed by backend-specific telemetry. The UI/API must say what was
actually observed rather than asserting that a PSU is physically absent.

| Device state         | Evidence and behavior                                                                                                             |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| `pending`            | First inventory/read has not completed.                                                                                           |
| `not_detected`       | No recognized host-visible telemetry interface. Explain that an ordinary or data-disconnected PSU can still power the server.     |
| `detected`           | Exact supported interface identity matched; successful telemetry not yet established.                                             |
| `driver_unavailable` | Known device enumerated but the expected hwmon driver/interface is missing or unbound.                                            |
| `unsupported`        | Exact PSU identity recognized, but no supported backend for it. Do not classify every USB device from a PSU vendor as a PSU.      |
| `ok` / `partial`     | Valid telemetry returned, with missing/failed individual channels represented explicitly.                                         |
| `busy` / `error`     | Permission, ownership, protocol or read failure; keep these distinct from absence.                                                |
| `disconnected`       | A previously observed interface disappeared. Publish a fresh unavailable snapshot and stop presenting its last values as current. |

These are proposed per-device states, not a commitment to a final wire schema. Mixed-device
success must not be hidden by one global boolean. Include timestamp, source, supported channels,
freshness and error reason. Preserve zero fan RPM or zero watts when valid; missing is null.

Discovery proposal:

1. Scan `/sys/bus/usb/devices` for exact supported VID/PID/interface combinations and
   `/sys/class/hwmon/hwmon*` for known PSU profiles. Do not match a vendor ID alone.
2. Correlate hwmon/HID nodes with resolved physical device ancestry. Reuse the existing USB
   identity walker, but hash or otherwise avoid exposing raw serials in new public IDs.
   A port-path fallback survives USB address changes but changes if the cable is moved.
3. Check driver readiness separately from device presence. Native hwmon should own the device
   whenever a usable kernel driver is bound. Do not race it with direct HID readers.
4. For direct HID, open only a recognized model/interface and send verified monitoring requests
   with bounded deadlines/retries. Serialize local requests. An advisory lock alone cannot
   coordinate with unrelated applications that ignore it; report busy rather than taking ownership.
5. Rediscover periodically and on relevant USB/hwmon events. A disabled telemetry collector still
   needs an independent inventory trigger so insertion can enable it. Publish disappearance before
   hardware gating stops collection, so caches and consumers cannot retain apparently live values.

Reading hwmon is an application-level read-only design. The kernel can still issue USB requests
underneath. Likewise a HID monitoring request requires writing request bytes; this must never be
confused with permission to alter voltage, OCP, fan mode or PSU protection.

Do not use generic `liquidctl initialize` as a discovery step: Corsair's direct initialization can
change OCP and fan mode. Do not force direct access, detach a bound driver, load new kernel modules
or instantiate I2C devices as part of passive discovery. [Corsair guide][corsair-guide],
[implementation][corsair-liquidctl], [PMBus safety guidance][pmbus-doc].

## Fit with the agent architecture at the research baseline

Inspected local source at commit `881a847`, initially on `feat/289-manifest-release-notes`.
No PSU collector existed at that baseline. The following records the integration plan subsequently
implemented for Corsair in this change. The owning code differed
from some architecture descriptions in AGENTS.md and older issue plans.

| Integration point     | Existing implementation and proposed addition                                                                                                                                                                                                                                                                                                  |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Hardware inventory    | [hardware_probe.go](../../daemon/services/collectors/hardware_probe.go): add a `hardware.psu` capability/profile and map it to `psu`. Preserve unknown versus absent on access failures.                                                                                                                                                       |
| Enable/disable policy | [collector_auto_policy.go](../../daemon/services/collector_auto_policy.go) and [collector_manager.go](../../daemon/services/collector_manager.go): reuse gating, explicit user disable, low-power handling and two-absence policy; explicitly clear PSU availability on removal.                                                               |
| Rescanning            | [capability_refresher.go](../../daemon/services/capability_refresher.go): current periodic hardware check is 15 minutes. Add a PSU-aware inventory wakeup for acceptable attachment latency, including when telemetry is stopped.                                                                                                              |
| Dynamic scheduling    | [scheduler/policy.go](../../daemon/services/scheduler/policy.go): register the PSU topic, demand policy, refresh-on-read and safety-net cadence. A proposed start is 60 seconds active and 5 minutes idle, subject to hardware validation.                                                                                                     |
| Hotplug               | [collector_event_sources.go](../../daemon/services/collector_event_sources.go) and [uevent_source.go](../../daemon/services/collectors/uevent_source.go): USB events already exist for hardware inventory collection. Explicitly wire PSU inventory/capability refresh and hwmon bind timing; this does not happen just by adding a collector. |
| Identity              | [system_hwmon.go](../../daemon/services/collectors/system_hwmon.go): reuse ancestry and serial/port correlation; distinguish chip `corsairpsu` from module `corsair-psu`.                                                                                                                                                                      |
| Data flow             | [eventbus.go](../../daemon/domain/eventbus.go), [topics.go](../../daemon/constants/topics.go), [event_bindings.go](../../daemon/services/api/event_bindings.go): use one typed `*dto.PSUStatus` topic and cache binding.                                                                                                                       |
| Cache/startup         | [cache_store.go](../../daemon/services/api/cache_store.go), [orchestrator.go](../../daemon/services/orchestrator.go): current caches use atomic snapshots and startup uses `Ready()`. Do not copy old mutex-cache/fixed-delay instructions.                                                                                                    |
| Consumers             | Add REST `/api/v1/psu`, WebSocket `psu_status_update`, MCP `get_psu_status`, MQTT/HA discovery and Prometheus deliberately. A cache binding does not create the REST/MCP/MQTT/metrics implementations automatically.                                                                                                                           |
| Existing power data   | [sysfs_power.go](../../daemon/lib/sysfs_power.go) reads RAPL package/DRAM energy. It is not PSU AC input or whole-server power and must remain semantically separate.                                                                                                                                                                          |

The normalized contract should include identity/model/backend, per-device availability and errors,
nullable AC input and DC output fields, rails, temperatures, fan speed, ratings provenance and
optional runtime. Each derived/estimated value needs its calculation/source marked. Read hwmon
units correctly: mV to V, mA to A, microwatts to W, millidegrees to degrees Celsius; RPM unchanged.
Do not treat generic motherboard voltage monitors as PSU devices. [hwmon ABI][hwmon-abi].

The repository uses MIT; liquidctl and kernel-derived implementations carry GPL licenses.
Reading protocol evidence is useful, but copying/adapting protected implementation into an MIT-only
agent requires an explicit license strategy. Review before reuse. A separately invoked compatible
tool is an optional backend with packaging/runtime costs, not equivalent to a dependency-free
native implementation. [Agent license](../../LICENSE), [NZXT source][nzxt-src], [cm-psu][cm-project].

## Existing issue plans need consolidation

All five issues were open when inspected. Their CodeRabbit plans are evidence of proposed work,
not proof of vendor capability, correctness or user approval.

- [#61 plan][p61] appropriately prefers hwmon, but AXi fallback support and discovery still need
  exact-model validation. It proposes a dedicated PSU status with pending/unavailable behavior.
- [#194 plan][p194] suggests identifying EVGA devices from a vendor ID/manufacturer. This could
  mistake other EVGA USB products for a PSU. Do not implement that without exact PSU evidence.
  It also mixes `corsair-psu` with a hwmon chip-name allowlist.
- [#195 plan][p195] treats a PMBus-capable industrial PSU and a functioning Linux binding too
  similarly. It needs host-path prerequisites and the newer OptiGuard Bluetooth investigation.
- [#196 plan][p196] has the strongest exact-model scope for NZXT; add protocol/coexistence,
  licensing and real hardware evidence before claiming support.
- [#197 plan][p197] embeds values in `SystemInfo`, unlike the other dedicated collector plans,
  and assumes only Corsair is relevant. Replace that fragmentation with a shared collector;
  assess Cooler Master, be quiet! IO and SilverStone CRPS individually.

Use #61 as the initial collector/backend work, #196 as the NZXT backend, and #197 as the shared
support matrix/expansion tracker. Keep #194 and #195 as exact-model protocol investigations.
Before implementation, reconcile the contract and acceptance criteria across the plans rather
than implementing five overlapping collectors or conflicting payloads.

## Delivery sequence and validation gates

1. Build the shared read-only collector and Corsair hwmon backend; test on an actual supported
   PSU with the target Unraid kernel. Check `CONFIG_SENSORS_CORSAIR_PSU`, module availability,
   binding, device IDs and exposed channels. A kernel version alone is insufficient.
2. Add NZXT E with protocol fixtures and physical E-series testing. Confirm expected interface,
   response framing, request spacing, cancellation, unplug behavior and competing-tool behavior.
3. Add Cooler Master hwmon consumption if a compatible module exists on the target. Keep module
   installation/maintenance separate; out-of-tree modules are tied to the Unraid kernel build.
4. Investigate AXi, EVGA NEX1500, be quiet! IO, Seasonic OptiGuard and server PMBus/BMC paths
   using exact hardware and protocol documentation. Do not advertise all units in a vendor family.

Fixture tests must cover multiple devices, `hwmon12` and higher, optional files, unit scaling,
valid zero versus missing, malformed/partial replies, model mismatches, permissions, timeout,
reconnect, VM passthrough/host invisibility, and unrelated vendor devices. Integration tests must
cover cache invalidation before collector stop, mixed availability, Dynamic and Manual modes,
API/MCP freshness, null-safe HA discovery and omission of missing Prometheus values.

No physical PSU, Unraid kernel configuration or live server was tested during this investigation.
No device protocol commands were sent. Research support is not hardware certification.

## Research method and limitations

- Exa: two research agents reviewed 82 returned search-result slots across the initial and
  follow-up searches, then checked candidate results against vendor documents, original driver
  repositories and kernel sources. This count is not 82 unique independently verified documents.
- Tavily: advanced searches and primary-page extraction independently checked PMBus safety,
  current Seasonic/be quiet! material and Cooler Master implementation. The separate automated
  research endpoint returned `Research task failed`; the report uses successful search/extraction
  results instead of claiming that endpoint produced a report.
- Context7: resolved `/liquidctl/liquidctl` and the upstream Linux tree. Results supported the
  preference for hwmon and standard registration interfaces, but detailed NZXT/PSU queries also
  returned unrelated cooling examples. Those examples were excluded; exact protocol/model claims
  above come from original source, not inferred Context7 snippets.
- Negative findings mean no validated interface was established in the reviewed sources. They
  do not prove that an undocumented implementation cannot exist. Marketing pages establish product
  capabilities, not necessarily register semantics or compatibility with Unraid.

[i61]: https://github.com/tinkerologie/unraid-management-agent/issues/61
[i194]: https://github.com/tinkerologie/unraid-management-agent/issues/194
[i195]: https://github.com/tinkerologie/unraid-management-agent/issues/195
[i196]: https://github.com/tinkerologie/unraid-management-agent/issues/196
[i197]: https://github.com/tinkerologie/unraid-management-agent/issues/197
[p61]: https://github.com/tinkerologie/unraid-management-agent/issues/61#issuecomment-3882817603
[p194]: https://github.com/tinkerologie/unraid-management-agent/issues/194#issuecomment-6075832091
[p195]: https://github.com/tinkerologie/unraid-management-agent/issues/195#issuecomment-6075829051
[p196]: https://github.com/tinkerologie/unraid-management-agent/issues/196#issuecomment-6075898638
[p197]: https://github.com/tinkerologie/unraid-management-agent/issues/197#issuecomment-6075872169
[corsair-doc]: https://docs.kernel.org/hwmon/corsair-psu.html
[corsair-src]: https://github.com/torvalds/linux/blob/master/drivers/hwmon/corsair-psu.c
[corsair-todo]: https://github.com/wgottwalt/corsair-psu/blob/main/todo.txt
[corsair-liquidctl]: https://github.com/liquidctl/liquidctl/blob/main/liquidctl/driver/corsair_hid_psu.py
[corsair-guide]: https://github.com/liquidctl/liquidctl/blob/main/docs/corsair-hxi-rmi-psu-guide.md
[hxi-manual]: https://www.corsair.com/us/en/explorer/diy-builder/power-supply-units/corsair-hxi-series/
[ocl]: https://github.com/audiohacked/OpenCorsairLink/blob/master/device.c
[nzxt-src]: https://github.com/liquidctl/liquidctl/blob/main/liquidctl/driver/nzxt_epsu.py
[nzxt-announcement]: https://nzxt.com/news/nzxt-announces-the-e-series-digital-psus
[nzxt-faq]: https://support.nzxt.com/hc/en-us/articles/1260801752069-NZXT-E-Series-and-C-Series-Power-Supply-FAQ
[evga-manual]: https://www.evga.com/support/manuals/files/120-PG-1500.pdf
[evga-launch]: https://www.evga.com/about/pressrelease/?id=136
[syncro]: https://seasonic.com/syncro-psu/
[seasonic-computex]: https://seasonic.com/computex-2026/
[seasonic-recap]: https://seasonic.com/insights/seasonic-updates-at-computex-2026/
[seasonic-851]: https://techace.jp/PDF/Seasonic/S2N-851CCP5-B/S2N-851CCP5-B%20spec.pdf
[pmbus-doc]: https://docs.kernel.org/hwmon/pmbus.html
[hwmon-abi]: https://docs.kernel.org/hwmon/sysfs-interface.html
[cm-project]: https://github.com/Jannis234/cm-psu
[cm-src]: https://github.com/Jannis234/cm-psu/blob/master/cm-psu.c
[bq-release]: https://www.bequiet.com/en/press/34164
[silverstone-cpb01]: https://www.silverstonetek.com/en/product/info/power-supplies/cpb01/
[tt-product]: https://www.thermaltake.com/toughpower-dps-g-rgb-1250w-titanium.html
[tt-work]: https://github.com/chestm007/linux_thermaltake_riing/issues/32
[asus-product]: https://rog.asus.com/us/power-supply-units/rog-thor/rog-thor-1200p3-gaming/
[asus-spec]: https://rog.asus.com/us/power-supply-units/rog-thor/rog-thor-1200p3-gaming/spec/
