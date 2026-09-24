package updater

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExecutablePathSurvivesRename(t *testing.T) {
	if os.Getenv("CYRENE_EXECUTABLE_HELPER") == "1" {
		original, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		original, err = filepath.EvalSymlinks(original)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(original, original+".old"); err != nil {
			t.Fatal(err)
		}
		got, err := ExecutablePath()
		if err != nil || got != original {
			t.Fatalf("target changed after live binary rename: got %q, want %q, error %v", got, original, err)
		}
		return
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	copyPath := filepath.Join(t.TempDir(), "updater-test.exe")
	out, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy test executable: %v, %v", copyErr, closeErr)
	}
	cmd := exec.Command(copyPath, "-test.run=^TestExecutablePathSurvivesRename$")
	cmd.Env = append(os.Environ(), "CYRENE_EXECUTABLE_HELPER=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("renamed executable helper failed: %v\n%s", err, output)
	}
}
