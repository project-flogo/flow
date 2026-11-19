package support

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/project-flogo/core/engine"
	"github.com/stretchr/testify/assert"
)

func callEnvGettersConcurrently(t *testing.T, want [4]string) {
	t.Helper()
	const goroutines, calls = 8, 500
	start := make(chan struct{})
	var wg sync.WaitGroup
	got := make(chan [4]string, goroutines*calls)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < calls; i++ {
				got <- [4]string{GetUserName(), GetHostId(), GetAppName(), GetAppVerison()}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(got)
	for g := range got {
		if g != want {
			t.Fatalf("got %q, want %q", g, want)
		}
	}
}

// The getters are reached from every flow start and from state recording on concurrent goroutines. The caches live
// for the process, so the body runs in a fresh child process each time.
func TestEnvGettersConcurrentUse(t *testing.T) {
	if os.Getenv("FLOW_ENV_TEST_CHILD") != "1" {
		timeout := 10 * time.Minute
		if deadline, ok := t.Deadline(); ok {
			timeout = time.Until(deadline) * 9 / 10
		}
		if timeout < time.Second {
			timeout = time.Second
		}
		args := []string{"-test.run=^TestEnvGettersConcurrentUse$", "-test.count=1", "-test.v", "-test.timeout=" + timeout.String()}
		if testing.CoverMode() != "" {
			dir := os.Getenv("TEST_CHILD_COVERDIR")
			if dir == "" {
				t.Fatal("cover mode is on: set TEST_CHILD_COVERDIR to keep the child process's cover profile")
			}
			args = append(args, "-test.coverprofile="+filepath.Join(dir, fmt.Sprintf("child-env-%d.out", os.Getpid())))
		}
		cmd := exec.Command(os.Args[0], args...)
		cmd.Env = append(os.Environ(), "FLOW_ENV_TEST_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child process failed: %v\n%s", err, out)
		}
		return
	}

	host, _ := os.Hostname()

	t.Setenv(UserName, "")
	t.Setenv(HostName, "")
	t.Setenv(AppName, "")
	t.Setenv(AppVersion, "")
	callEnvGettersConcurrently(t, [4]string{"flogo", host, engine.GetAppName(), engine.GetAppVersion()})

	t.Setenv(UserName, "user-1")
	t.Setenv(HostName, "host-1")
	t.Setenv(AppName, "app-1")
	t.Setenv(AppVersion, "1.0.1")
	callEnvGettersConcurrently(t, [4]string{"user-1", "host-1", "app-1", "1.0.1"})

	// The first non-empty value is kept for the life of the process.
	t.Setenv(UserName, "user-2")
	t.Setenv(HostName, "host-2")
	t.Setenv(AppName, "app-2")
	t.Setenv(AppVersion, "2.0.0")
	assert.Equal(t, [4]string{"user-1", "host-1", "app-1", "1.0.1"},
		[4]string{GetUserName(), GetHostId(), GetAppName(), GetAppVerison()})
}
