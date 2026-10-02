package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type call struct {
	name string
	args []string
}

type response struct {
	output string
	err    error
}

type fakeRunner struct {
	responses map[string][]response
	calls     []call
}

func (f *fakeRunner) run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, call{name: name, args: args})
	key := strings.Join(append([]string{name}, args...), " ")
	responses := f.responses[key]
	if len(responses) == 0 {
		return nil, fmt.Errorf("unexpected command: %s", key)
	}
	result := responses[0]
	f.responses[key] = responses[1:]
	return []byte(result.output), result.err
}

func TestRunFetchesAndPreviewsIntegratedWorktrees(t *testing.T) {
	commands := newFakeRunner()
	var stdout, stderr bytes.Buffer
	if err := run(nil, commands, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}

	output := stdout.String()
	for _, want := range []string{"feature/rebased", "patch_id_match in origin/main", "feature/squashed", "merge_adds_nothing in origin/main", "2 matching worktree(s)"} {
		if !strings.Contains(output, want) {
			t.Errorf("output %q does not contain %q", output, want)
		}
	}
	for _, invocation := range commands.calls {
		if invocation.name == "wt" && len(invocation.args) > 0 && invocation.args[0] == "remove" {
			t.Errorf("preview invoked wt remove: %#v", invocation)
		}
	}
}

func TestRunRemovesOnlySafeIntegratedWorktrees(t *testing.T) {
	commands := newFakeRunner()
	commands.responses["wt remove --foreground --format text feature/rebased"] = []response{{}}
	commands.responses["wt remove --foreground --format text feature/squashed"] = []response{{}}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--yes"}, commands, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}

	var removed []string
	for _, invocation := range commands.calls {
		if invocation.name == "wt" && len(invocation.args) > 0 && invocation.args[0] == "remove" {
			removed = append(removed, invocation.args[len(invocation.args)-1])
		}
	}
	want := []string{"feature/rebased", "feature/squashed"}
	if !reflect.DeepEqual(removed, want) {
		t.Fatalf("removed branches = %v, want %v", removed, want)
	}
}

func TestRunForwardsRemovalFlagsAndJSONOutput(t *testing.T) {
	commands := newFakeRunner()
	rebased := `{"branch":"feature/rebased","branch_outcome":"not_attempted"}` + "\n"
	squashed := `{"branch":"feature/squashed","branch_outcome":"not_attempted"}` + "\n"
	commands.responses["wt remove --foreground --format json --no-delete-branch --reap --no-hooks feature/rebased"] = []response{{output: rebased}}
	commands.responses["wt remove --foreground --format json --no-delete-branch --reap --no-hooks feature/squashed"] = []response{{output: squashed}}

	var stdout, stderr bytes.Buffer
	args := []string{"--yes", "--format=json", "--no-delete-branch", "--reap", "--no-hooks"}
	if err := run(args, commands, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if got, want := stdout.String(), rebased+squashed; got != want {
		t.Fatalf("JSON output = %q, want %q", got, want)
	}
}

func TestRunJSONPreviewHonorsNoDeleteBranch(t *testing.T) {
	commands := newFakeRunner()
	var stdout, stderr bytes.Buffer
	if err := run([]string{"--format=json", "--no-delete-branch"}, commands, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if !strings.Contains(line, `"branch_deleted":false`) {
			t.Errorf("preview line %q does not retain the branch", line)
		}
	}
}

func newFakeRunner() *fakeRunner {
	list := `{
  "repo": {"default_branch":"main","forge":{"remote":"origin"}},
  "items": [
    {"branch":"main","worktree":{"path":"/repo","main":true,"changes":null},"default_branch":{"integration":{"reason":"same_commit"}}},
    {"branch":"feature/current","worktree":{"path":"/repo.current","current":true,"changes":null},"default_branch":{"integration":{"reason":"ancestor"}}},
    {"branch":"feature/rebased","worktree":{"path":"/repo.rebased","changes":null},"default_branch":{"integration":{"reason":"patch_id_match"}}},
    {"branch":"feature/squashed","worktree":{"path":"/repo.squashed","changes":null},"default_branch":{"integration":{"reason":"merge_adds_nothing"}}},
    {"branch":"feature/dirty","worktree":{"path":"/repo.dirty","changes":{"modified":true}},"default_branch":{"integration":null}},
    {"branch":"feature/open","worktree":{"path":"/repo.open","changes":null},"default_branch":{}},
    {"branch":"feature/locked","worktree":{"path":"/repo.locked","locked":{"reason":"busy"},"changes":null},"default_branch":{"integration":{"reason":"ancestor"}}}
  ]
}`
	return &fakeRunner{responses: map[string][]response{
		"wt list --format=json":                    {{output: list}, {output: list}},
		"git fetch --quiet origin refs/heads/main": {{}},
	}}
}
