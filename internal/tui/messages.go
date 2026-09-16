package tui

import "lidoo/internal/docker"

type ProfilesLoadedMsg struct {
	Profiles []docker.ProfileSummary
}

type ProfilesFailedMsg struct {
	Err error
}
