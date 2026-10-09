package gui

import "os/exec"

func appsDirectory() string { return "/Applications" }
func openPath(path string, reveal, edit bool) error {
	args := []string{}
	if reveal {
		args = append(args, "-R")
	}
	if edit {
		args = append(args, "-t")
	}
	return exec.Command("open", append(args, path)...).Run()
}
