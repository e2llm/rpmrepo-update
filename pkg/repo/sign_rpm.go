package repo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// signRPM re-signs an RPM payload using gpg via rpmsign --resign. Expects rpm data as bytes.
func (r *Repo) signRPM(ctx context.Context, rpmData []byte, gpgKey string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "rpmrepo-sign-*.rpm")
	if err != nil {
		return nil, fmt.Errorf("mktemp failed: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(rpmData); err != nil {
		return nil, fmt.Errorf("write temp rpm: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp rpm: %w", err)
	}

	cmd := exec.CommandContext(ctx, "rpmsign", "--resign")
	if gpgKey != "" {
		cmd.Args = append(cmd.Args, "--define", fmt.Sprintf("_gpg_name %s", gpgKey))
	}
	cmd.Args = append(cmd.Args, tmpPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("rpmsign failed: %s", strings.TrimSpace(string(out)))
	}
	signed, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read signed rpm: %w", err)
	}
	return signed, nil
}
