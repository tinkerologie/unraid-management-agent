# PSU telemetry

The shared `psu` collector monitors host-visible Corsair RMi/HXi USB power supplies through
the Linux `corsair-psu` driver. Its hwmon chip name is `corsairpsu`. No separate Corsair plugin,
userspace PSU tool or raw HID polling is required.

## Requirements and compatibility

- Connect the PSU's USB data cable to the host. Standard ATX power cables do not identify a PSU.
- The Unraid kernel must include and bind `CONFIG_SENSORS_CORSAIR_PSU` for the exact USB ID.
  A sufficiently recent kernel version alone does not guarantee its build configuration or IDs.
- The agent must be able to read the device's USB identity and hwmon attributes.
- The data interface must be visible to the host rather than assigned exclusively to a VM.

The initial model table includes Corsair vendor `1b1c`, product IDs `1c03`–`1c0d`, `1c1e`,
`1c1f`, `1c23` and `1c27` for RMi/HXi models declared in upstream
[driver source](https://github.com/torvalds/linux/blob/master/drivers/hwmon/corsair-psu.c).
Host kernels can support a smaller subset. AXi, non-i RM/HX, and other vendors are outside this
backend. See the [research and expansion matrix](../design/psu-telemetry-feasibility.md).

This implementation is validated with synthetic sysfs fixtures. Physical PSU and Unraid kernel
validation remain necessary before treating a particular model/host combination as hardware-tested.

## Automatic detection

The agent matches exact USB vendor/product IDs, verifies the HID driver binding, correlates
the physical USB device with a `corsairpsu` hwmon child, and supports multiple devices. Device IDs hash the model and serial,
falling back to a USB port path for missing or duplicate serials; raw serial numbers are not published. Port-derived IDs change
when a PSU is moved to another port.

A separate inventory watcher checks USB/hwmon hotplug events and rescans every 30 seconds even
when hardware gating has stopped telemetry. An enumerated PSU without hwmon is reported as
`driver_unavailable`. This does not trigger automatic module installation, device rebinding,
voltage/OCP changes or fan-control changes. Do not run a competing raw HID reader while the
kernel driver owns the device.

The snapshot `state` is `pending`, `not_detected`, `driver_unavailable`, `ok`, `partial`, `error`
or `disabled`. Each device has its own state and message. `not_detected` means no recognized
host-visible telemetry interface, not that the server physically lacks a PSU. Access/read errors
remain distinct from absence. A disappearance clears current measurements immediately on inventory
detection; the existing hardware policy stops the collector after two absence observations.

## Configuration and collection modes

In the plugin's Manual collection settings, **PSU Telemetry** defaults to **Automatic**: it runs
at the existing five-minute automatic interval while detected. A numeric interval overrides this
cadence; **Disabled** keeps it off. The daemon used directly defaults to 60 seconds and remains
hardware-gated. `--interval-psu` or `INTERVAL_PSU` sets its Manual interval; the valid enabled
range is 5–86400 seconds. Low-power mode multiplies collection intervals by four.

YAML configuration:

```yaml
intervals:
  psu: 60
```

Dynamic mode uses a 60-second active cadence, five-minute idle safety-net polling, USB/hwmon
change events and refresh on stale REST/MCP/metrics reads. Manual interval values are retained
but unused in Dynamic mode, as for other collectors. Use `UNRAID_DISABLE_COLLECTORS=psu` or
the collector-disable API to explicitly disable collection in either mode. Automatic detection
does not override an explicit runtime disable.

## API and measurement meaning

- REST: `GET /api/v1/psu`.
- WebSocket: `psu_status_update`.
- MCP: `get_psu_status` (read-only).
- MQTT: `<topic_prefix>/<hostname>/psu`, with per-sensor Home Assistant discovery.
- Prometheus: `unraid_psu_state`, `unraid_psu_device_state`, and measured sensor families
  `unraid_psu_voltage_volts`, `unraid_psu_current_amperes`, `unraid_psu_power_watts`,
  `unraid_psu_temperature_celsius`, `unraid_psu_fan_rpm` and derived `unraid_psu_load_percent`.

`PSUStatus` contains `state`, `devices`, optional `errors`, and `timestamp`. A device contains
identity, backend/driver, model/family, state/message, `capacity_watts`, and normalized `sensors`.
Each sensor has a stable `name`, display `label`, `kind`, `unit`, nullable `value`, and optional
`derived` marker. Missing values remain JSON null; valid zero watts and zero fan RPM stay zero.

The `output_power` sensor is **total DC output**, with separate 12V/5V/3.3V rail channels.
The `input_voltage` and optional `input_current` channels do not establish measured real AC watts.
`input_power` and `efficiency` remain null with this backend. Multiplying AC volts by amps would
give apparent power without accounting for power factor. `load` is explicitly derived from DC
output divided by the verified model's rated capacity; capacity itself is not a measurement.
Ambiguous table entries `1c03`, `1c09` and `1c23` have null capacity and load until their ratings
are independently confirmed.

Temperature/fan and voltage/current/power values are scaled from the kernel ABI. Implausible
temperatures, malformed values and unreadable channels are unavailable without discarding other
valid measurements. No debugfs dependency or efficiency-curve estimates are used.

Prometheus omits null readings and removes disappeared device series on the next scrape.
Home Assistant entities require both agent connectivity and a valid sensor reading; unavailable
readings do not become zero. Discovery configurations are cleaned up when measurements disappear.

## Troubleshooting

If the PSU is not detected, check the data cable, USB passthrough and exact model. If it is
`driver_unavailable`, check the running kernel's driver configuration, supported IDs and bound
hwmon interface. The agent reports these conditions without attempting to repair kernel/device
ownership. `partial` or `error` identifies unavailable channels; inspect the snapshot's messages
and errors alongside the host's kernel logs.

Further hardware requirements and channel semantics are documented by the
[Linux driver](https://docs.kernel.org/hwmon/corsair-psu.html).
