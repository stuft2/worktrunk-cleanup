package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type runner interface {
	run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return output.Bytes(), err
}

type listResult struct {
	Repo struct {
		DefaultBranch string `json:"default_branch"`
		Forge         struct {
			Remote string `json:"remote"`
		} `json:"forge"`
	} `json:"repo"`
	Items []listItem `json:"items"`
}

type listItem struct {
	Branch        string `json:"branch"`
	Worktree      *worktree
	DefaultBranch struct {
		Integration *integration `json:"integration"`
	} `json:"default_branch"`
}

type worktree struct {
	Path    string          `json:"path"`
	Main    bool            `json:"main"`
	Current bool            `json:"current"`
	Locked  json.RawMessage `json:"locked"`
	Changes changes         `json:"changes"`
}

type changes struct {
	Staged     bool `json:"staged"`
	Modified   bool `json:"modified"`
	Untracked  bool `json:"untracked"`
	Renamed    bool `json:"renamed"`
	Deleted    bool `json:"deleted"`
	Conflicted bool `json:"conflicted"`
}

type integration struct {
	Reason string `json:"reason"`
}

type previewResult struct {
	Branch        string `json:"branch"`
	Path          string `json:"path"`
	Reason        string `json:"reason"`
	Target        string `json:"target"`
	BranchDeleted bool   `json:"branch_deleted"`
}

func main() {
	if err := run(os.Args[1:], execRunner{}, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, commands runner, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("wt cleanup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	apply := flags.Bool("yes", false, "remove matching worktrees and branches")
	flags.BoolVar(apply, "y", false, "remove matching worktrees and branches")
	noDeleteBranch := flags.Bool("no-delete-branch", false, "keep local branches after removing worktrees")
	reap := flags.Bool("reap", false, "terminate non-interactive processes in removed worktrees")
	noHooks := flags.Bool("no-hooks", false, "skip Worktrunk removal hooks")
	format := flags.String("format", "text", "output format: text or json")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: wt cleanup [--yes]")
		fmt.Fprintln(flags.Output(), "\nFetch the default branch and remove clean worktrees integrated into it.")
		fmt.Fprintln(flags.Output(), "Without --yes, only show what would be removed.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	if *format != "text" && *format != "json" {
		return fmt.Errorf("invalid format %q: expected text or json", *format)
	}

	listed, err := listWorktrees(commands)
	if err != nil {
		return err
	}
	if listed.Repo.Forge.Remote == "" || listed.Repo.DefaultBranch == "" {
		return errors.New("repository has no remote default branch")
	}
	upstream := listed.Repo.Forge.Remote + "/" + listed.Repo.DefaultBranch
	if _, err := commands.run("git", "fetch", "--quiet", listed.Repo.Forge.Remote, "refs/heads/"+listed.Repo.DefaultBranch); err != nil {
		return fmt.Errorf("fetch default branch %s: %w", upstream, err)
	}

	listed, err = listWorktrees(commands)
	if err != nil {
		return err
	}
	matched := 0
	removed := 0
	for _, item := range listed.Items {
		if item.Worktree == nil || item.Worktree.Main || item.Worktree.Current || item.Branch == "" ||
			isPresent(item.Worktree.Locked) || item.Worktree.Changes.dirty() || item.DefaultBranch.Integration == nil {
			continue
		}

		matched++
		if !*apply && *format == "json" {
			result := previewResult{
				Branch:        item.Branch,
				Path:          item.Worktree.Path,
				Reason:        item.DefaultBranch.Integration.Reason,
				Target:        upstream,
				BranchDeleted: !*noDeleteBranch,
			}
			if err := json.NewEncoder(stdout).Encode(result); err != nil {
				return fmt.Errorf("encode preview for %s: %w", item.Branch, err)
			}
			continue
		}
		action := "would remove"
		if *apply {
			action = "remove"
		}
		if *format == "text" {
			fmt.Fprintf(stdout, "%-12s %s (%s): %s in %s\n", action, item.Worktree.Path, item.Branch, item.DefaultBranch.Integration.Reason, upstream)
		}
		if !*apply {
			continue
		}
		removeArgs := []string{"remove", "--foreground", "--format", *format}
		if *noDeleteBranch {
			removeArgs = append(removeArgs, "--no-delete-branch")
		}
		if *reap {
			removeArgs = append(removeArgs, "--reap")
		}
		if *noHooks {
			removeArgs = append(removeArgs, "--no-hooks")
		}
		removeArgs = append(removeArgs, item.Branch)
		removeOutput, err := commands.run("wt", removeArgs...)
		if err != nil {
			return fmt.Errorf("remove %s: %w", item.Branch, err)
		}
		if *format == "json" {
			if _, err := stdout.Write(removeOutput); err != nil {
				return fmt.Errorf("write removal result for %s: %w", item.Branch, err)
			}
		}
		removed++
	}

	if *format == "text" {
		if *apply {
			fmt.Fprintf(stdout, "\nRemoved %d worktree(s).\n", removed)
		} else {
			fmt.Fprintf(stdout, "\n%d matching worktree(s). Re-run with --yes to remove them.\n", matched)
		}
	}
	return nil
}

func listWorktrees(commands runner) (listResult, error) {
	output, err := commands.run("wt", "list", "--format=json")
	if err != nil {
		return listResult{}, fmt.Errorf("list worktrees: %w", err)
	}
	var result listResult
	if err := json.Unmarshal(output, &result); err != nil {
		return listResult{}, fmt.Errorf("decode worktree list: %w", err)
	}
	return result, nil
}

func isPresent(value json.RawMessage) bool {
	return len(value) != 0 && string(value) != "null"
}

func (c changes) dirty() bool {
	return c.Staged || c.Modified || c.Untracked || c.Renamed || c.Deleted || c.Conflicted
}
