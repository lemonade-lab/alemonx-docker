package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerHelper(t *testing.T) {
	if os.Getenv("ALX_DOCKER_HELPER") != "1" {
		return
	}
	if os.Getenv("ALX_DOCKER_HELPER_FAIL_CLI") == "1" {
		fmt.Fprintln(os.Stderr, "docker: command not found")
		os.Exit(1)
	}
	args := os.Args
	for index, value := range args {
		if value == "--" && index+1 < len(args) {
			args = args[index+1:]
			break
		}
	}
	joined := strings.Join(args, " ")
	switch {
	case os.Getenv("ALX_DOCKER_HELPER_FAIL_COMPOSE") == "1" && joined == "compose version --short":
		fmt.Fprintln(os.Stderr, "docker compose: plugin not found")
		os.Exit(1)
	case os.Getenv("ALX_DOCKER_HELPER_FAIL_DAEMON") == "1" && strings.HasPrefix(joined, "info "):
		fmt.Fprintln(os.Stderr, "Cannot connect to the Docker daemon")
		os.Exit(1)
	case os.Getenv("ALX_DOCKER_HELPER_FAIL_NEXT") == "1" &&
		joined != "version --format {{.Client.Version}}" &&
		joined != "compose version --short" &&
		joined != "info --format {{.ServerVersion}}":
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(1)
	case os.Getenv("ALX_DOCKER_HELPER_IMAGES") == "1" && strings.Contains(joined, "image ls"):
		_, _ = os.Stdout.WriteString(`{"ID":"sha256:abc","Repository":"nginx","Tag":"latest","Size":"50MB","CreatedSince":"2 weeks ago"}` + "\n")
	case os.Getenv("ALX_DOCKER_HELPER_CONTAINERS") == "1" && strings.Contains(joined, "container ls"):
		_, _ = os.Stdout.WriteString(`{"ID":"abc123","Names":"web-1","Image":"nginx:latest","State":"running","Status":"Up 2 minutes","Ports":"0.0.0.0:8080->80/tcp, 443/tcp","Labels":"com.docker.compose.project=myapp,com.docker.compose.service=web"}` + "\n")
	case joined == "version --format {{.Client.Version}}":
		_, _ = os.Stdout.WriteString("27.0.0")
	case joined == "compose version --short":
		_, _ = os.Stdout.WriteString("v2.29.0")
	case joined == "info --format {{.ServerVersion}}":
		_, _ = os.Stdout.WriteString("27.0.0")
	default:
		if os.Getenv("ALX_DOCKER_HELPER_CWD") == "1" {
			cwd, _ := os.Getwd()
			_, _ = os.Stdout.WriteString("cwd=" + cwd + "\n")
		}
		_, _ = os.Stdout.WriteString(strings.Join(args, "|"))
	}
	os.Exit(0)
}

func fakeDocker(ctx context.Context, _ string, args ...string) *exec.Cmd {
	return fakeDockerWith(ctx, nil, args...)
}

func fakeDockerWith(ctx context.Context, env []string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=TestDockerHelper", "--")
	command.Args = append(command.Args, args...)
	command.Env = append(os.Environ(), "ALX_DOCKER_HELPER=1")
	command.Env = append(command.Env, env...)
	return command
}

func withDocker(t *testing.T, env ...string) {
	t.Helper()
	original := execCommand
	execCommand = func(ctx context.Context, _ string, args ...string) *exec.Cmd {
		return fakeDockerWith(ctx, env, args...)
	}
	t.Cleanup(func() { execCommand = original })
}

func TestDockerStatusHealthy(t *testing.T) {
	withDocker(t)
	status := dockerStatus()
	if !status.CLI.Available || !status.Compose.Available || !status.Daemon.Available {
		t.Fatalf("healthy status: %+v", status)
	}
	if status.CLI.Version != "27.0.0" || status.Compose.Version != "v2.29.0" || status.Daemon.Version != "27.0.0" {
		t.Fatalf("unexpected versions: %+v", status)
	}
}

func TestDockerStatusMissingCLI(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_CLI=1")
	status := dockerStatus()
	if status.CLI.Available || status.Compose.Available || status.Daemon.Available {
		t.Fatalf("CLI missing must fail every check: %+v", status)
	}
	if !strings.Contains(status.CLI.Detail, "not found") {
		t.Fatalf("detail should surface docker error: %+v", status.CLI)
	}
}

func TestDockerStatusDaemonDown(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_DAEMON=1")
	status := dockerStatus()
	if !status.CLI.Available || !status.Compose.Available || status.Daemon.Available {
		t.Fatalf("only daemon should be down: %+v", status)
	}
	if !strings.Contains(status.Daemon.Detail, "daemon") {
		t.Fatalf("daemon detail should explain the failure: %+v", status.Daemon)
	}
}

func TestDockerStatusComposeMissing(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_COMPOSE=1")
	status := dockerStatus()
	if !status.CLI.Available || status.Compose.Available || !status.Daemon.Available {
		t.Fatalf("only compose plugin should be missing: %+v", status)
	}
}

func TestComposeLifecycleUsesExactCommandsAndProjectDir(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	withDocker(t, "ALX_DOCKER_HELPER_CWD=1")
	dir, err := projectPath(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	for action, verb := range map[string]string{
		"compose-up":      "up|-d",
		"compose-stop":    "stop",
		"compose-restart": "restart",
		"compose-down":    "down",
	} {
		result, err := composeAction(action, project.ID)
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if !strings.Contains(result.Output, "compose|-f|"+composePath(dir)+"|"+verb) {
			t.Fatalf("%s used unexpected command: %q", action, result.Output)
		}
		if !strings.Contains(result.Output, "cwd="+realDir) {
			t.Fatalf("%s did not run in project dir %q: %q", action, realDir, result.Output)
		}
	}
}

func TestComposeDownUsesNoVolumeOrImageDeletionFlags(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	withDocker(t)
	result, err := composeAction("compose-down", project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "compose|-f|") || !strings.Contains(result.Output, "|down") {
		t.Fatalf("unexpected compose command: %q", result.Output)
	}
	if strings.Contains(result.Output, "|-v") || strings.Contains(result.Output, "--rmi") || strings.Contains(result.Output, "--volumes") {
		t.Fatalf("unsafe compose command: %q", result.Output)
	}
}

func TestComposeActionRequiresManagedProject(t *testing.T) {
	useTemporaryProjectRoot(t)
	withDocker(t)
	if _, err := composeAction("compose-up", "../escape"); err == nil {
		t.Fatal("invalid project ID must fail")
	}
	if _, err := composeAction("compose-up", "missing-project"); err == nil {
		t.Fatal("unknown project must fail")
	}
}

func TestComposeErrorMapsDockerOutput(t *testing.T) {
	useTemporaryProjectRoot(t)
	project, err := createProject("Error")
	if err != nil {
		t.Fatal(err)
	}
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_NEXT=1")
	result, err := composeAction("compose-up", project.ID)
	if err == nil || !strings.Contains(err.Error(), "Compose 操作失败") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should map docker failure: %v", err)
	}
	if !strings.Contains(result.Output, "boom") {
		t.Fatalf("raw docker output should be returned: %q", result.Output)
	}
}

func TestImageActionsUseExactCommands(t *testing.T) {
	withDocker(t)
	pulled, err := imagePull("nginx:latest")
	if err != nil || !strings.Contains(pulled.Output, "image|pull|nginx:latest") {
		t.Fatalf("pull = %+v, %v", pulled, err)
	}
	removed, err := imageRemove("nginx:1.27")
	if err != nil || !strings.Contains(removed.Output, "image|rm|nginx:1.27") {
		t.Fatalf("remove = %+v, %v", removed, err)
	}
}

func TestImageActionErrorMapsDockerOutput(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_FAIL_NEXT=1")
	result, err := imagePull("nginx:latest")
	if err == nil || !strings.Contains(err.Error(), "拉取镜像失败") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error should map docker failure: %v", err)
	}
	if !strings.Contains(result.Output, "boom") {
		t.Fatalf("raw docker output should be returned: %q", result.Output)
	}
}

func TestImageListParsesRows(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_IMAGES=1")
	items, err := dockerImages()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Repository != "nginx" || items[0].Tag != "latest" || items[0].Size != "50MB" {
		t.Fatalf("images = %+v", items)
	}
}

func TestContainerActionsUseExactCommands(t *testing.T) {
	withDocker(t)
	for action, verb := range map[string]string{
		"container-start":   "start",
		"container-stop":    "stop",
		"container-restart": "restart",
	} {
		result, err := containerAction(action, "abc123")
		if err != nil || !strings.Contains(result.Output, "container|"+verb+"|abc123") {
			t.Fatalf("%s = %+v, %v", action, result, err)
		}
	}
	logs, err := containerLogs("abc123")
	if err != nil || !strings.Contains(logs.Output, "container|logs|--tail|200|abc123") {
		t.Fatalf("logs = %+v, %v", logs, err)
	}
}

func TestContainerListExtractsComposeProject(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_CONTAINERS=1")
	items, err := dockerContainers()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "abc123" || items[0].Project != "myapp" || items[0].State != "running" {
		t.Fatalf("containers = %+v", items)
	}
	if len(items[0].Published) != 1 || items[0].Published[0].HostPort != 8080 || items[0].Published[0].ContainerPort != 80 || items[0].Published[0].Protocol != "tcp" {
		t.Fatalf("published ports = %+v", items[0].Published)
	}
}

func TestParsePublishedPorts(t *testing.T) {
	ports := parsePublishedPorts("0.0.0.0:8080->80/tcp, [::]:8080->80/tcp, 127.0.0.1:9000->9000/udp, 443/tcp, 0.0.0.0:3000-3005->3000-3005/tcp")
	if len(ports) != 3 {
		t.Fatalf("ports = %+v", ports)
	}
	if ports[0].HostPort != 8080 || ports[0].ContainerPort != 80 || ports[0].Protocol != "tcp" {
		t.Fatalf("first port = %+v", ports[0])
	}
	if ports[1].HostPort != 9000 || ports[1].Protocol != "udp" {
		t.Fatalf("udp port = %+v", ports[1])
	}
	if ports[2].HostPort != 3000 || ports[2].ContainerPort != 3000 {
		t.Fatalf("range port = %+v", ports[2])
	}
}

func TestDockerPortsFlattensContainers(t *testing.T) {
	withDocker(t, "ALX_DOCKER_HELPER_CONTAINERS=1")
	rows, err := dockerPorts()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].HostPort != 8080 || rows[0].ContainerID != "abc123" || rows[0].Project != "myapp" {
		t.Fatalf("port rows = %+v", rows)
	}
}

func TestDockerActionsRejectUnsafeReferences(t *testing.T) {
	for _, value := range []string{"", "nginx; rm -rf /", "../nginx", "image name", "--force nginx", "nginx --privileged"} {
		if _, err := imagePull(value); err == nil {
			t.Fatalf("image %q must fail", value)
		}
		if _, err := containerAction("container-start", value); err == nil {
			t.Fatalf("container %q must fail", value)
		}
		if _, err := containerLogs(value); err == nil {
			t.Fatalf("logs %q must fail", value)
		}
	}
}

func TestClipOutput(t *testing.T) {
	short := strings.Repeat("a", 100)
	if clipped := clipOutput(short); clipped != short {
		t.Fatalf("short output changed: %q", clipped)
	}
	long := strings.Repeat("b", maxActionOutput+50)
	clipped := clipOutput(long)
	if !strings.Contains(clipped, "已截断") || len([]rune(clipped)) >= len([]rune(long)) {
		t.Fatalf("long output not truncated: %d runes", len([]rune(clipped)))
	}
}
