package api

import "github.com/prometheus/client_golang/prometheus"

// PSU metrics are collected directly from one immutable snapshot per scrape.
// This avoids global GaugeVec reset races and drops disappeared/nil readings.
var psuMetricDescriptors = map[string]*prometheus.Desc{
	"voltage":     prometheus.NewDesc("unraid_psu_voltage_volts", "PSU measured voltage in volts", []string{"device", "sensor"}, nil),
	"current":     prometheus.NewDesc("unraid_psu_current_amperes", "PSU measured current in amperes", []string{"device", "sensor"}, nil),
	"power":       prometheus.NewDesc("unraid_psu_power_watts", "PSU power in watts; output channels are DC, not wall power", []string{"device", "sensor"}, nil),
	"temperature": prometheus.NewDesc("unraid_psu_temperature_celsius", "PSU temperature in Celsius", []string{"device", "sensor"}, nil),
	"fan":         prometheus.NewDesc("unraid_psu_fan_rpm", "PSU fan revolutions per minute", []string{"device", "sensor"}, nil),
	"load":        prometheus.NewDesc("unraid_psu_load_percent", "PSU DC output load derived from rated capacity", []string{"device", "sensor"}, nil),
	"efficiency":  prometheus.NewDesc("unraid_psu_efficiency_percent", "PSU efficiency when provided by a backend", []string{"device", "sensor"}, nil),
}

var psuDeviceStateDesc = prometheus.NewDesc("unraid_psu_device_state", "PSU device state (always 1)", []string{"device", "state"}, nil)
var psuStateDesc = prometheus.NewDesc("unraid_psu_state", "PSU snapshot state (always 1)", []string{"state"}, nil)

type psuMetricsCollector struct{ cache *CacheStore }

func (c psuMetricsCollector) Describe(out chan<- *prometheus.Desc) {
	for _, desc := range psuMetricDescriptors {
		out <- desc
	}
	out <- psuDeviceStateDesc
	out <- psuStateDesc
}

func (c psuMetricsCollector) Collect(out chan<- prometheus.Metric) {
	status := c.cache.GetPSUCache()
	if status == nil {
		return
	}
	out <- prometheus.MustNewConstMetric(psuStateDesc, prometheus.GaugeValue, 1, status.State)
	for _, dev := range status.Devices {
		out <- prometheus.MustNewConstMetric(psuDeviceStateDesc, prometheus.GaugeValue, 1, dev.ID, dev.State)
		for _, sensor := range dev.Sensors {
			desc := psuMetricDescriptors[sensor.Kind]
			if sensor.Value == nil || desc == nil {
				continue
			}
			out <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, *sensor.Value, dev.ID, sensor.Name)
		}
	}
}
