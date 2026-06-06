package platform

// MissingKeys returns the keys absent from m (or with empty values), preserving
// the requested order. Used by collector shape validators on parsed INI maps.
func MissingKeys(m map[string]string, keys ...string) []string {
	var missing []string
	for _, k := range keys {
		if v, ok := m[k]; !ok || v == "" {
			missing = append(missing, k)
		}
	}
	return missing
}
