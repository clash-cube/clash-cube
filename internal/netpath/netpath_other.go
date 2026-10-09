//go:build !darwin && !windows

package netpath

func Current() Path { return Path{} }
