//go:build !darwin

package wifi

func Current() Status    { return Status{State: "unavailable"} }
func RequestPermission() {}

func SavedNetworks() ([]string, error) { return []string{}, nil }
