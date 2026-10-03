package search

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		for _, bad := range []string{"nats", "minio", "protobuf", "mongo", "compress/gzip"} {
			if strings.Contains(lower, bad) {
				t.Fatalf("forbidden dependency %q in %s", bad, line)
			}
		}
	}
}
