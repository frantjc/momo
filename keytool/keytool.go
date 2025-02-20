package keytool

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func SHA256CertFingerprints(ctx context.Context, name string) (string, error) {
	return Command("keytool").SHA256CertFingerprints(ctx, name)
}

// Command represents the path to an `keytool` executable.
type Command string

func (c Command) String() string {
	return string(c)
}

func (c Command) SHA256CertFingerprints(ctx context.Context, name string) (string, error) {
	var (
		buf = new(bytes.Buffer)
		//nolint:gosec
		cmd = exec.CommandContext(ctx, c.String(), "-printcert", "-jarfile", name)
	)

	cmd.Stdout = buf

	if err := cmd.Run(); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(buf)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "SHA256: ") {
			if fields := strings.Fields(line); len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}

	return "", fmt.Errorf("sha256 cert fingerprints not found")
}
