package tui

import (
	"os/exec"

	"lidoo/internal/addons"
	"lidoo/internal/app"
	"lidoo/internal/docker"
	"lidoo/internal/odoo"
)

type ProfilesLoadedMsg struct {
	Profiles []docker.ProfileSummary
}

type ProfilesFailedMsg struct {
	Err error
}

type ProfileWatcherStartedMsg struct {
	Events <-chan app.ProfileInvalidation
	Errors <-chan error
}

type ProfileInvalidationMsg struct {
	Invalidation app.ProfileInvalidation
}

type ProfileWatcherDisconnectedMsg struct {
	Err error
}

type ProfileDetailLoadedMsg struct {
	RequestID   uint64
	ProfileName string
	Detail      docker.ProfileDetail
}

type ProfileDetailFailedMsg struct {
	RequestID   uint64
	ProfileName string
	Err         error
}

type DatabasesLoadedMsg struct {
	RequestID   uint64
	ProfileName string
	Databases   []odoo.Database
}

type DatabasesFailedMsg struct {
	RequestID   uint64
	ProfileName string
	Err         error
}

type DatabaseInfoLoadedMsg struct {
	RequestID        uint64
	ProfileName      string
	DatabasePhysical string
	Info             odoo.DatabaseInfo
}

type DatabaseInfoFailedMsg struct {
	RequestID        uint64
	ProfileName      string
	DatabasePhysical string
	Err              error
}

type AddonsLoadedMsg struct {
	RequestID   uint64
	ProfileName string
	Addons      []addons.AddonStatus
}

type AddonsFailedMsg struct {
	RequestID   uint64
	ProfileName string
	Err         error
}

type TaskStartedMsg struct {
	ID uint64
}

type TaskProgressMsg struct {
	ID   uint64
	Text string
}

type TaskProgressDoneMsg struct {
	ID uint64
}

type TaskCompletedMsg struct {
	ID          uint64
	ProfileName string
	Output      string
}

type TaskFailedMsg struct {
	ID          uint64
	ProfileName string
	Err         error
	Output      string
}

type TaskCancelledMsg struct {
	ID          uint64
	ProfileName string
	Output      string
}

type ProfileLogsLoadedMsg struct {
	RequestID   uint64
	ProfileName string
	Output      string
}

type ProfileLogsFailedMsg struct {
	RequestID   uint64
	ProfileName string
	Err         error
}

type interactiveKind uint8

const (
	interactiveProfileLogs interactiveKind = iota
	interactiveDatabaseShell
)

type InteractiveCommandReadyMsg struct {
	Kind         interactiveKind
	ProfileName  string
	DatabaseName string
	Command      *exec.Cmd
}

type InteractiveCommandFailedMsg struct {
	Kind         interactiveKind
	ProfileName  string
	DatabaseName string
	Err          error
}

type InteractiveCommandFinishedMsg struct {
	Kind         interactiveKind
	ProfileName  string
	DatabaseName string
	Err          error
}
