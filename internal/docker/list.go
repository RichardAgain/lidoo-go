package docker

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

type profileRow struct {
	name   string
	status string
}

func List(args []string) error {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("list does not accept positional arguments")
	}

	output, err := dockerOutput(
		"ps", "--all",
		"--filter", "label="+containerNameLabel,
		"--format", `{{.Label "io.lidoo.name"}}	{{.State}}`,
	)
	if err != nil {
		return fmt.Errorf("find profiles: %w", err)
	}

	profiles, err := parseProfileList(output)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		fmt.Println("no profiles found")
		return nil
	}
	return renderProfileList(os.Stdout, profiles)
}

func parseProfileList(output []byte) ([]profileRow, error) {
	var profiles []profileRow
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" {
			return nil, fmt.Errorf("invalid profile row from Docker: %q", line)
		}
		profiles = append(profiles, profileRow{
			name:   strings.TrimSpace(fields[0]),
			status: strings.TrimSpace(fields[1]),
		})
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].name < profiles[j].name
	})
	return profiles, nil
}

func renderProfileList(w io.Writer, profiles []profileRow) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "PROFILE\tSTATUS\tURL")
	for _, profile := range profiles {
		fmt.Fprintf(table, "%s\t%s\thttp://%s\n", profile.name, profile.status, profileHostname(profile.name))
	}
	return table.Flush()
}
