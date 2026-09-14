package odoo

import (
	"fmt"
	"net/http"
	"time"

	"lidoo/internal/docker"
	"lidoo/internal/files"
	profiles "lidoo/internal/profile"
)

func Wait(name string, timeout time.Duration, state files.State) error {
	if name == "" {
		return fmt.Errorf("wait requires name")
	}
	if err := docker.ValidateProfileName(name); err != nil {
		return err
	}
	if timeout <= 0 {
		return fmt.Errorf("wait timeout must be positive")
	}

	deadline := time.Now().Add(timeout)
	lastReason := "profile checks have not run"
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		containerState, exists, err := docker.ProfileState(name)
		if err != nil {
			lastReason = err.Error()
		} else if !exists {
			return fmt.Errorf("profile %q has no container; run it before waiting", name)
		} else if containerState == "exited" || containerState == "dead" {
			return fmt.Errorf("profile %q exited before becoming ready; inspect it with lidoo logs --name %s", name, name)
		} else if containerState != "running" {
			lastReason = "container state is " + containerState
		} else if err := databaseReachable(name); err != nil {
			lastReason = "PostgreSQL is not reachable: " + err.Error()
		} else if err := odooHTTPReady(client, name); err != nil {
			lastReason = "Odoo HTTP is not ready: " + err.Error()
		} else {
			fmt.Printf("profile %q is ready at http://%s\n", name, profiles.Hostname(name))
			return nil
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("profile %q did not become ready within %s: %s; inspect it with lidoo logs --name %s", name, timeout, lastReason, name)
		}
		interval := 500 * time.Millisecond
		if remaining < interval {
			interval = remaining
		}
		time.Sleep(interval)
	}
}

func databaseReachable(name string) error {
	container, err := docker.RequireRunningProfile(name)
	if err != nil {
		return err
	}
	if _, err := runCapture(container,
		"psql", "--no-psqlrc", "--tuples-only", "--no-align", "--command", "SELECT 1", "postgres",
	); err != nil {
		return err
	}
	return nil
}

func odooHTTPReady(client *http.Client, name string) error {
	request, err := http.NewRequest(http.MethodGet, "http://"+profiles.Hostname(name)+"/web/login", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("HTTP status %s", response.Status)
	}
	return nil
}
