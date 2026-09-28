package tui

import (
	"os/exec"

	"lidoo/internal/addons"
	"lidoo/internal/app"
	"lidoo/internal/docker"
	"lidoo/internal/files"
	"lidoo/internal/odoo"
	"lidoo/internal/profile"
)

type ProfileURLOpenFailedMsg struct {
	Err error
}

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

type databaseInfoResult struct {
	DatabasePhysical string
	Info             odoo.DatabaseInfo
	Err              error
}

type DatabaseInfosLoadedMsg struct {
	RequestID   uint64
	ProfileName string
	Results     []databaseInfoResult
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
	ID          uint64
	ProfileName string
	Text        string
}

type TaskProgressDoneMsg struct {
	ID uint64
}

type TaskCompletedMsg struct {
	ID          uint64
	ProfileName string
}

type TaskFailedMsg struct {
	ID          uint64
	ProfileName string
	Err         error
}

type TaskCancelledMsg struct {
	ID          uint64
	ProfileName string
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

type ProfileLogStreamMsg struct {
	RequestID   uint64
	ProfileName string
	Text        string
}

type ProfileLogStreamFailedMsg struct {
	RequestID   uint64
	ProfileName string
	Err         error
}

type ProfileLogStreamDoneMsg struct {
	RequestID   uint64
	ProfileName string
}

type interactiveKind uint8

const interactiveDatabaseShell interactiveKind = iota

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

type VersionsLoadedMsg struct {
	RequestID uint64
	Options   []docker.VersionOption
}

type VersionsFailedMsg struct {
	RequestID uint64
	Err       error
}

type ProfileConfigLoadedMsg struct {
	RequestID   uint64
	ProfileName string
	Config      profile.Config
	Found       bool
}

type ProfileConfigFailedMsg struct {
	RequestID   uint64
	ProfileName string
	Err         error
}

type AllAddonsLoadedMsg struct {
	RequestID uint64
	Addons    []addons.AddonStatus
}

type AllAddonsFailedMsg struct {
	RequestID uint64
	Err       error
}

type AddonBranchesLoadedMsg struct {
	RequestID uint64
	Source    string
	Branches  []string
}

type AddonBranchesFailedMsg struct {
	RequestID uint64
	Source    string
	Err       error
}

type RestoreSourcesLoadedMsg struct {
	RequestID uint64
	Sources   []string
}

type RestoreSourcesFailedMsg struct {
	RequestID uint64
	Err       error
}

type DirectoryLoadedMsg struct {
	RequestID uint64
	Path      string
	Entries   []files.DirEntry
}

type DirectoryFailedMsg struct {
	RequestID uint64
	Path      string
	Err       error
}
