# PSU telemetry

The `psu` collector reads Corsair RMi and HXi USB power supplies that the host can see.
It uses the Linux `corsair-psu` driver and the `corsairpsu` hwmon interface.
This feature needs no separate Corsair plugin, userspace PSU tool, or raw HID polling.

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

The agent matches the USB vendor and product IDs. It checks the HID driver and finds the matching `corsairpsu` hwmon device.
The agent supports multiple devices.

The agent calculates a device ID from the model and serial number. If the serial number is missing or duplicated, it uses the USB port path.
The agent does not publish raw serial numbers. A device ID that uses the port path changes when you move the PSU to another port.

A separate inventory watcher checks USB/hwmon hotplug events and rescans every 30 seconds even
when hardware gating has stopped telemetry. An enumerated PSU without hwmon is reported as
`driver_unavailable`. This does not trigger automatic module installation, device rebinding,
voltage/OCP changes or fan-control changes. Do not run a competing raw HID reader while the
kernel driver owns the device.

The snapshot `state` is `pending`, `not_detected`, `driver_unavailable`, `ok`, `partial`, `error`
or `disabled`. Each device has its own state and message. `not_detected` means no recognized
host-visible telemetry interface, not that the server physically lacks a PSU. Access/read errors
remain distinct from absence. When the inventory watcher detects that a PSU is absent, it clears the current measurements.
The hardware policy stops the collector after two consecutive absent results.

## Configuration and collection modes

In the plugin's Manual collection settings, **PSU Telemetry** defaults to **Automatic**.
The collector runs every five minutes when the agent detects a supported PSU.
A numeric interval replaces the automatic interval. **Disabled** stops the collector.

The daemon without the plugin uses a default interval of 60 seconds. It still requires supported hardware.
The `--interval-psu` flag or `INTERVAL_PSU` variable sets the Manual interval.
The available range is 5 to 86400 seconds. Low-power mode multiplies collection intervals by four.

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

| State                | Checks                                                           |
| -------------------- | ---------------------------------------------------------------- |
| `not_detected`       | Check the data cable, USB passthrough, and PSU model.            |
| `driver_unavailable` | Check the kernel driver, supported USB IDs, and hwmon interface. |
| `partial` or `error` | Compare the snapshot messages with the kernel logs.              |

The agent reports these conditions. It does not change which driver controls the device.

Further hardware requirements and channel semantics are documented by the
[Linux driver](https://docs.kernel.org/hwmon/corsair-psu.html).
