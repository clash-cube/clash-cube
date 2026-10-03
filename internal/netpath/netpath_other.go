//go:build !darwin

package netpath

func Current() Path { return Path{} }
