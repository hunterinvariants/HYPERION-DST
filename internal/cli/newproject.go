package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func newCommand() Command {
	return Command{
		Name:    "new",
		Summary: "generate a starter project wiring your protocol to the engine",
		Run:     runNew,
	}
}

// modulePlaceholder is replaced with the generated module path.
const modulePlaceholder = "MODULEPATH"

type templateFile struct {
	name string
	body string
}

func runNew(args []string) int {
	flags := flag.NewFlagSet("promtact new", flag.ExitOnError)
	module := flags.String("module", "", "module path for go.mod (defaults to the directory name)")
	_ = flags.Parse(args)

	if flags.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: promtact new [-module path] <directory>")
		return 2
	}
	dir := flags.Arg(0)

	// Refuse to write into an existing non-empty directory. Overwriting a
	// file someone already edited is not a tolerable failure mode for a
	// convenience command.
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		fmt.Fprintf(os.Stderr, "new: %s already exists and is not empty\n", dir)
		return 2
	} else if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "new: %v\n", err)
		return 2
	}

	path := *module
	if path == "" {
		path = filepath.Base(filepath.Clean(dir))
	}
	if path == "." || path == string(filepath.Separator) || path == "" {
		fmt.Fprintln(os.Stderr, "new: cannot infer a module path; pass -module")
		return 2
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "new: %v\n", err)
		return 2
	}
	for _, file := range projectTemplate {
		body := strings.ReplaceAll(file.body, modulePlaceholder, path)
		target := filepath.Join(dir, file.name)
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "new: %v\n", err)
			return 1
		}
		fmt.Println("wrote", target)
	}

	fmt.Printf("\nnext:\n  cd %s\n  go mod tidy\n  go test ./... -v\n", dir)
	return 0
}
