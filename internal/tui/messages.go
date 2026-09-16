package tui

import (
	"lidoo/internal/addons"
	"lidoo/internal/docker"
	"lidoo/internal/odoo"
)

type ProfilesLoadedMsg struct {
	Profiles []docker.ProfileSummary
}

type ProfilesFailedMsg struct {
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
