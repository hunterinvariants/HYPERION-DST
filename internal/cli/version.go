package cli

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
)

// version is stamped at link time by the release workflow, which already knows
// the tag it is building. A downloaded binary is the one artifact that cannot
// answer "which release is this?" any other way: the file name is the only
// other clue, and a file name is whatever the last person to touch it says.
var version string

// versionCommand reports what this binary is, from whichever source knows.
//
// Three build paths reach a user, and each one records the version somewhere
// different, so the command reads them in order of authority rather than
// guessing:
//
//	release binary   -ldflags -X sets version directly
//	go install @vN   the module version lands in BuildInfo.Main.Version
//	go build         no version exists, so the commit is the honest answer
func versionCommand() Command {
	return Command{
		Name:    "version",
		Summary: "report the version, revision and toolchain of this binary",
		Run:     runVersion,
	}
}

func runVersion(args []string) int {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	short := flags.Bool("short", false, "print only the version")
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: promtact version [-short]\n\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}

	release, revision, modified, buildTime := buildIdentity()
	if *short {
		fmt.Println(release)
		return 0
	}

	fmt.Printf("promtact %s\n", release)
	if revision != "" {
		suffix := ""
		if modified {
			// A dirty tree means the revision names a commit the binary does
			// not correspond to, which is worse than no revision at all
			// unless it is said out loud.
			suffix = " (built from a modified tree)"
		}
		fmt.Printf("revision   %s%s\n", revision, suffix)
	}
	if buildTime != "" {
		fmt.Printf("built      %s\n", buildTime)
	}
	fmt.Printf("go         %s\n", runtime.Version())
	fmt.Printf("platform   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return 0
}

// buildIdentity gathers what the toolchain recorded about this build.
func buildIdentity() (release, revision string, modified bool, buildTime string) {
	release = strings.TrimSpace(version)

	info, ok := debug.ReadBuildInfo()
	if !ok {
		if release == "" {
			release = "unknown"
		}
		return release, "", false, ""
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		case "vcs.time":
			buildTime = setting.Value
		}
	}

	if release == "" {
		// "(devel)" is what the toolchain writes for a build that is not a
		// tagged module version. Reporting it verbatim is more useful than
		// inventing a number, and the revision below says exactly which
		// commit it was.
		release = info.Main.Version
	}
	if release == "" {
		release = "unknown"
	}
	return release, revision, modified, buildTime
}
