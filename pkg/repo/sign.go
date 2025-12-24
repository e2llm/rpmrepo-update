package repo

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// signRepomd writes a detached ASCII-armored signature for repomd.xml as repodata/repomd.xml.asc.
func (r *Repo) signRepomd(ctx context.Context, repomd []byte, gpgKey string) error {
	cmd := exec.CommandContext(ctx, "gpg", "--detach-sign", "--armor", "--batch", "--yes")
	if gpgKey != "" {
		cmd.Args = append(cmd.Args, "--local-user", gpgKey)
	}
	cmd.Args = append(cmd.Args, "-o", "-")
	cmd.Stdin = bytes.NewReader(repomd)
	out, err := cmd.Output()
	if err != nil {
		// capture stderr if available
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("gpg sign failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return fmt.Errorf("gpg sign failed: %w", err)
	}
	return r.backend.WriteFile(ctx, "repodata/repomd.xml.asc", out)
}
